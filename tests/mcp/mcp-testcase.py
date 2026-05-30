import os
import json
import asyncio
import sys
import re
from pathlib import Path
from contextlib import AsyncExitStack

try:
    from dotenv import load_dotenv

    load_dotenv()
except ImportError:
    pass

from openai import AsyncOpenAI
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client


# ==========================================================
# CONFIG
# ==========================================================

# -----------------------------------------
# PATHS
# -----------------------------------------
BASE_DIR = Path(__file__).parent


# -----------------------------------------
# API KEYS
# -----------------------------------------
TOKENROUTER_API_KEY = os.getenv("TOKENROUTER_API_KEY")
LIVEREVIEW_API_KEY = os.getenv("LIVEREVIEW_API_KEY_TR")

if not TOKENROUTER_API_KEY:
    print(
        "Error: TOKENROUTER_API_KEY environment variable not set",
        file=sys.stderr,
    )
    sys.exit(1)


# -----------------------------------------
# MODEL CONFIG
# -----------------------------------------

# Free model
# MODEL = "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free"

# Paid model
# qwen/qwen3.5-9b
# Input: $0.10 / 1M tokens
# Output: $0.15 / 1M tokens
MODEL = "qwen/qwen3.5-9b"

MAX_TOOL_ITERATIONS = 10


# -----------------------------------------
# TOKENROUTER OPENAI CLIENT
# -----------------------------------------
client = AsyncOpenAI(
    base_url="https://api.tokenrouter.com/v1",
    api_key=TOKENROUTER_API_KEY,
)


# -----------------------------------------
# LIVEREVIEW MCP SERVER
# -----------------------------------------
MCP_SERVER_CONFIG = {
    "command": "npx",
    "args": [
        "-y",
        "mcp-remote",
        "http://localhost:8888/api/mcp",
        "--header",
        f"X-API-KEY: {LIVEREVIEW_API_KEY}",
    ],
    "env": {"LIVEREVIEW_API_KEY": (LIVEREVIEW_API_KEY or "")},
}


# -----------------------------------------
# FILESYSTEM MCP SERVER
# -----------------------------------------
FILESYSTEM_MCP_CONFIG = {
    "command": "npx",
    "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        str(BASE_DIR),
    ],
}
# =========================================================

# =========================================================
# RUNTIME CONTEXT
# Shared runtime state across testcase execution
# =========================================================


class RuntimeContext:
    def __init__(
        self,
        client,
        model,
        tools,
        session,
        filesystem_session,
        livereview_tool_names,
        hello_mode,
        master_instructions,
    ):
        self.client = client
        self.model = model
        self.tools = tools
        self.session = session
        self.filesystem_session = filesystem_session
        self.livereview_tool_names = livereview_tool_names
        self.hello_mode = hello_mode
        self.master_instructions = master_instructions


# =========================================================
# MCP / TOOL HELPERS
# Functions for MCP setup, tool conversion, and tool output
# =========================================================


def convert_mcp_to_openai_tool(mcp_tool):
    return {
        "type": "function",
        "function": {
            "name": mcp_tool.name,
            "description": mcp_tool.description or "",
            "parameters": mcp_tool.inputSchema,
        },
    }


def extract_tool_output(mcp_result):
    output_parts = []

    for content in mcp_result.content:
        if hasattr(content, "text"):
            output_parts.append(content.text)
        else:
            output_parts.append(str(content))

    return "\n".join(output_parts)


async def initialize_mcp_sessions(stack):
    # Start MCP Server
    server_params = StdioServerParameters(**MCP_SERVER_CONFIG)

    stdio_transport = await (stack.enter_async_context(stdio_client(server_params)))

    stdio, write = stdio_transport

    session = await stack.enter_async_context(ClientSession(stdio, write))

    await session.initialize()

    # Start Filesystem MCP
    filesystem_server_params = StdioServerParameters(**FILESYSTEM_MCP_CONFIG)

    filesystem_stdio_transport = await stack.enter_async_context(
        stdio_client(filesystem_server_params)
    )

    filesystem_stdio, filesystem_write = filesystem_stdio_transport

    filesystem_session = await stack.enter_async_context(
        ClientSession(filesystem_stdio, filesystem_write)
    )

    await filesystem_session.initialize()
    return (session, filesystem_session)


async def load_available_tools(session, filesystem_session):
    livereview_tools_response = await session.list_tools()

    filesystem_tools_response = await filesystem_session.list_tools()

    livereview_tools = [
        convert_mcp_to_openai_tool(tool) for tool in (livereview_tools_response.tools)
    ]

    filesystem_tools = [
        convert_mcp_to_openai_tool(tool) for tool in (filesystem_tools_response.tools)
    ]

    tools = livereview_tools + filesystem_tools

    print(
        f"Connected to "
        f"{len(livereview_tools)} "
        f"LiveReview tools + "
        f"{len(filesystem_tools)} "
        f"filesystem tools",
        file=sys.stderr,
    )
    return (livereview_tools, filesystem_tools, tools)


async def execute_tool_call(
    tool_call,
    session,
    filesystem_session,
    livereview_tool_names,
):
    tool_name = tool_call.function.name

    try:
        tool_args = json.loads(tool_call.function.arguments or "{}")
    except json.JSONDecodeError:
        tool_args = {}

    target_session = (
        session if tool_name in livereview_tool_names else filesystem_session
    )

    try:
        mcp_result = await asyncio.wait_for(
            target_session.call_tool(tool_name, arguments=tool_args), timeout=30
        )

        tool_output = extract_tool_output(mcp_result)

    except Exception as e:
        tool_output = f"Tool execution failed: " f"{str(e)}"

    return {
        "tool_name": tool_name,
        "tool_output": tool_output,
        "tool_message": {
            "role": "tool",
            "tool_call_id": tool_call.id,
            "name": tool_name,
            "content": compress_tool_output(tool_name, tool_output),
        }
    }


# =========================================================
# EXECUTION MODE / TEST DISCOVERY
# Determine execution mode and load testcase metadata
# =========================================================


def get_execution_mode_and_test_files():
    # -----------------------------------------
    # HELLO MODE
    # -----------------------------------------
    hello_mode = "--hello" in sys.argv

    # Remove flag from argv
    if hello_mode:
        sys.argv.remove("--hello")

    if hello_mode:
        test_files = [None]
    else:
        test_files = [
            "1.basic_review_test.md",
            "2.quota_test.md",
            "3.billing_status_test.md",
            "4.billing_upgrade_preview_test.md",
            "5.git_connector_listing_test.md",
            "6.billing_usage_by_members_test.md",
            "7.recent_billable_reviews_test.md",
            "8.review_listing_test.md",
            "9.list_ai_providers_test.md",
            "10.learnings_mcp_test.md",
            "11.review_insights_mcp_test.md",
            "12.prompt_rules_mcp_test.md",
        ]
        test_files = [str(BASE_DIR / f) for f in test_files]

    return hello_mode, test_files


def load_master_instructions(hello_mode):
    master_instructions = ""
    if not hello_mode:
        master_instructions_path = BASE_DIR / "master-test-executor-json.md"
        if master_instructions_path.exists():
            master_instructions = master_instructions_path.read_text(encoding="utf-8")
    return master_instructions


def load_testcase_content(file_path):
    if not file_path:
        return ""

    print(f"\n\n=== RUNNING TESTCASE: " f"{file_path} ===", file=sys.stderr)

    md_content = Path(file_path).read_text(encoding="utf-8")

    # Replace {{ENV_VARS}}
    # from OS environment variables
    def env_replacer(match):
        var_name = match.group(1)

        val = os.environ.get(var_name)

        if val is None:
            print(
                f"Warning: " f"Environment variable " f"'{var_name}' " f"not found",
                file=sys.stderr,
            )

            # Keep unchanged if missing
            return match.group(0)

        return val

    md_content = re.sub(r"\{\{([^}]+)\}\}", env_replacer, md_content)

    return md_content


# =========================================================
# PROMPT / MESSAGE BUILDERS
# Construct model input messages
# =========================================================


def build_initial_messages(hello_mode, master_instructions, md_content):
    if hello_mode:
        messages = [
            {
                "role": "system",
                "content": "You are testing MCP connectivity. "
                "If tools are available, you may use one. "
                "Keep response short.",
            },
            {"role": "user", "content": "Hello world. " "Can you see MCP tools?"},
        ]
        return messages
    else:
        messages = [
            {
                "role": "system",
                "content": (
                    f"You are an automated QA execution agent. Follow instructions exactly.\n"
                    f"=== MASTER INSTRUCTIONS ===\n{master_instructions}\n===========================\n\n"
                    "IMPORTANT: You are evaluating a SINGLE test case right now in isolation.\n"
                    "Each testcase must run in a fresh execution context.\n"
                    "Do not reuse messages, tool outputs, or intermediate state from previous testcases.\n"
                    "If a file is referenced, use filesystem MCP to read it.\n"
                    "Never assume file contents. Execute tests using available tools.\n"
                    "Never truncate tool output. Never summarize tool output.\n"
                    "Always return raw tool output exactly as received.\n"
                    "If output is large: still include full string. do NOT cut or compress.\n"
                    "When you finish executing the testcase, you MUST return a SINGLE JSON object representing the result of THIS testcase ONLY.\n"
                    "Do NOT return the suite_summary, just the testcase object containing: id, file_name, test_name, passed, error_message, detailed_execution_report."
                ),
            },
            {
                "role": "user",
                "content": f"Here is the specification file for the current test case:\n\n{md_content}\n\nExecute the test case and return ONLY the JSON result object for this specific testcase.",
            },
        ]
        return messages


# =========================================================
# MODEL EXECUTION
# OpenAI model calls and retry handling
# =========================================================


async def call_model_with_retry(
    client,
    model,
    messages,
    tools,
    max_retries=3,
    timeout=120,
):
    response = None
    aborted = False

    for model_attempt in range(1, max_retries + 1):
        try:
            response = await asyncio.wait_for(
                client.chat.completions.create(
                    model=model,
                    messages=messages,
                    tools=tools if tools else None,
                    temperature=0.0,
                ),
                timeout=timeout,
            )
            break

        except asyncio.TimeoutError:
            print(
                f"❌ MODEL CALL TIMED OUT "
                f"({timeout}s) - attempt "
                f"{model_attempt}/"
                f"{max_retries}",
                file=sys.stderr,
            )

            if model_attempt == max_retries:
                aborted = True
                break

    return {
        "response": response,
        "aborted": aborted,
    }


async def execute_agent_loop(
    client,
    model,
    messages,
    tools,
    session,
    filesystem_session,
    livereview_tool_names,
    hello_mode,
):
    print("\n=== EXECUTING TESTCASE (Agent Loop) ===", file=sys.stderr)

    tool_outputs_log = []
    aborted = False

    for iteration in range(MAX_TOOL_ITERATIONS):
        print(f"\n--- Iteration {iteration+1} ---", file=sys.stderr)

        print(f"Messages count: {len(messages)}", file=sys.stderr)

        model_result = await call_model_with_retry(
            client=client,
            model=model,
            messages=messages,
            tools=tools,
        )

        aborted = model_result["aborted"]

        if aborted:
            break

        response_message = model_result["response"].choices[0].message

        print("Model responded", file=sys.stderr)

        print(
            f"Tool calls: " f"{len(response_message.tool_calls or [])}", file=sys.stderr
        )

        if not response_message.tool_calls:
            final_text = response_message.content or ""

            if hello_mode:
                print(final_text)

            break

        messages.append({
            "role": "assistant",
            "content": response_message.content,
            "tool_calls": response_message.tool_calls
        })

        for tool_call in response_message.tool_calls:
            tool_result = await execute_tool_call(
                tool_call=tool_call,
                session=session,
                filesystem_session=filesystem_session,
                livereview_tool_names=livereview_tool_names,
            )

            tool_outputs_log.append(
                f"=== TOOL: {tool_result['tool_name']} ===\n{tool_result['tool_output']}\n"
            )

            messages.append(tool_result["tool_message"])




    return {
        "aborted": aborted,
        "tool_outputs_log": tool_outputs_log,
    }


# =========================================================
# TESTCASE EXECUTION
# Execute a single testcase and build result
# =========================================================


def build_test_result(
    file_path,
    tool_outputs_log,
):
    has_error = any(
        "tool execution failed:" in out.lower() or "tool_timeout" in out.lower()
        for out in tool_outputs_log
    )

    return {
        "id": (
            int(Path(file_path).name.split(".")[0])
            if Path(file_path).name.split(".")[0].isdigit()
            else 0
        ),
        "file_name": Path(file_path).name,
        "test_name": (Path(file_path).name.replace("_", " ").replace(".md", "")),
        "passed": not has_error,
        "error_message": ("Tools reported errors or failures" if has_error else None),
        "detailed_execution_report": "\n".join(tool_outputs_log),
    }


async def run_single_testcase(file_path, context):
    hello_mode = context.hello_mode
    master_instructions = context.master_instructions
    client = context.client
    model = context.model
    tools = context.tools
    session = context.session
    filesystem_session = context.filesystem_session
    livereview_tool_names = context.livereview_tool_names

    md_content = load_testcase_content(file_path)

    messages = build_initial_messages(hello_mode, master_instructions, md_content)

    # -----------------------------------------
    # AGENT LOOP (Multi-turn)
    # -----------------------------------------
    print("\n=== EXECUTING TESTCASE (Agent Loop) ===", file=sys.stderr)

    agent_result = await execute_agent_loop(
        client=client,
        model=model,
        messages=messages,
        tools=tools,
        session=session,
        filesystem_session=filesystem_session,
        livereview_tool_names=livereview_tool_names,
        hello_mode=hello_mode,
    )

    aborted = agent_result["aborted"]

    tool_outputs_log = agent_result["tool_outputs_log"]

    if aborted:
        return None

    # -----------------------------------------
    # FINAL JSON BUILDER (Deterministic)
    # -----------------------------------------
    if not hello_mode and file_path:
        print(
            f"Building deterministic JSON output for " f"{Path(file_path).name}",
            file=sys.stderr,
        )

        return build_test_result(
            file_path=file_path,
            tool_outputs_log=tool_outputs_log,
        )
    else:
        return None

    return None


# =========================================================
# RESULT PROCESSING
# Build and save final suite results
# =========================================================


def save_test_results(all_test_results):
    passed_count = sum(1 for t in all_test_results if t.get("passed", False))

    failed_count = len(all_test_results) - passed_count

    final_output = {
        "suite_summary": {
            "total_tests": len(all_test_results),
            "passed_count": passed_count,
            "failed_count": failed_count,
            "overall_status": ("PASSED" if failed_count == 0 else "FAILED"),
        },
        "tests": all_test_results,
    }

    output_file = BASE_DIR / "test_results.json"

    output_file.write_text(
        json.dumps(
            final_output,
            indent=2,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    print(
        f"\n✅ Final test suite results "
        f"successfully written to: "
        f"{output_file.name}",
        file=sys.stderr,
    )

    print(
        json.dumps(
            final_output,
            indent=2,
            ensure_ascii=False,
        )
    )





def compress_tool_output(tool_name, tool_output):
    if len(tool_output) < 1500:
        return tool_output

    return (
        f"Tool '{tool_name}' succeeded.\n"
        f"Large response received ({len(tool_output)} chars).\n"
        f"Only first portion shown:\n\n"
        f"{tool_output[:1000]}"
    )

# =========================================================
# APPLICATION ENTRYPOINT
# =========================================================


async def main():
    # -----------------------------------------
    # LOAD EXECUTION MODE + TEST CONFIG
    # -----------------------------------------
    hello_mode, test_files = get_execution_mode_and_test_files()
    master_instructions = load_master_instructions(hello_mode)

    # -----------------------------------------
    # INITIALIZE MCP SESSIONS
    # -----------------------------------------
    async with AsyncExitStack() as stack:
        session, filesystem_session = await initialize_mcp_sessions(stack)

        # -----------------------------------------
        # LOAD AVAILABLE MCP TOOLS
        # -----------------------------------------
        (livereview_tools, filesystem_tools, tools,) = await load_available_tools(
            session,
            filesystem_session,
        )

        livereview_tool_names = {t["function"]["name"] for t in livereview_tools}

        # -----------------------------------------
        # BUILD SHARED RUNTIME CONTEXT
        # -----------------------------------------
        context = RuntimeContext(
            client=client,
            model=MODEL,
            tools=tools,
            session=session,
            filesystem_session=filesystem_session,
            livereview_tool_names=(livereview_tool_names),
            hello_mode=hello_mode,
            master_instructions=(master_instructions),
        )

        # -----------------------------------------
        # EXECUTE TEST SUITE
        # -----------------------------------------
        all_test_results = []

        for file_path in test_files:
            result = await run_single_testcase(
                file_path=file_path,
                context=context,
            )

            if result:
                all_test_results.append(result)

        # -----------------------------------------
        # SAVE FINAL TEST RESULTS
        # -----------------------------------------
        if not hello_mode:
            save_test_results(all_test_results)


if __name__ == "__main__":
    asyncio.run(main())
