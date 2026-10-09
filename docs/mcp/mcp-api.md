# MCP & AI Provider Error Handling

The LiveReview WebChat endpoint (`/api/v1/chat/send`) has multiple layers of resilience and error handling for the AI layer and the internal MCP server. 

To prevent user confusion when errors occur, the backend intercepts these failures and injects specific UI components into the response:
- **Blockquote Messages**: A stylized error message explaining what happened.
- **Action Cards**: A card with a button (e.g., "Configure AI Provider", "Try Again") directing the user on how to fix it.
- **Suggested Questions**: Helpful fallback questions (e.g., "Where do I get an AI API key?").
- **Debug Artifacts**: The raw underlying error (e.g., HTTP 502, JSON parsing errors) stored in the debug logs for developers.

## Error Handling Cases

To make it easier to debug, we categorize errors into two main areas: **Internal MCP Issues (Our End)** and **LLM Provider Issues (Model Side)**. 

### 1. Internal MCP Issues (Our End)
These occur rarely, usually due to infrastructure or internal server problems. When these happen, we display a clear message indicating the problem is on our end, and provide debug logs that the user can share with us for troubleshooting.

| Case | Trigger / Condition | Message Prefix | Action Card Title | Suggested Questions | Fallback Behavior |
|------|---------------------|----------------|-------------------|---------------------|-------------------|
| **MCP Server Offline** | Connection to the MCP server fails (e.g. `502 Bad Gateway`). | `> **Something went wrong on our end**` | **MCP Server Offline** (Button: Try Again) | None | Turn aborted. Debug log shows raw network error. |

### 2. LLM Provider Issues (Model Side)
These are the most common issues. They occur due to provider rate limits, model deprecations, missing API keys, or hallucinated/truncated formatting.

| Case | Trigger / Condition | Message Prefix | Action Card Title | Suggested Questions | Fallback Behavior |
|------|---------------------|----------------|-------------------|---------------------|-------------------|
| **Auth / Config Issue** | AI Provider API key is invalid or missing (`ErrCategoryAuth`). | `> **Action Required: AI Provider Issue**` | **Configuration Required** (Button: Configure AI Provider) | `DefaultAIErrorSuggestedQuestions` | Turn aborted. |
| **Provider Overloaded** | AI Provider returns 503 or 429 (`ErrCategoryOverload`). | `> **AI Provider is Busy**` | **High Traffic Detected** (Button: Configure AI Provider) | `DefaultAIErrorSuggestedQuestions` | **Graceful Degradation**: If this happens during `classify`, the backend degrades to `product_guidance` mode, attaches the Action Card, and successfully generates a helpful product answer. |
| **Model Deprecated** | The configured AI model was removed (`ErrCategoryDeprecated`). | `> **Model No Longer Available**` | **Model No Longer Available** (Button: Configure AI Provider) | `DefaultAIErrorSuggestedQuestions` | Turn aborted. |
| **Timeout Error** | AI Provider takes too long to respond (`ErrCategoryTimeout`). | `> **Analysis Took Too Long**` | **Timeout Error** (Button: Try Again) | `DefaultAIErrorSuggestedQuestions` | Turn aborted. |
| **Unexpected AI Error** | Any other unclassified LLM API error (`ErrCategoryUnknown`). | `> **Unexpected Error**` | **Unexpected Error** (Button: Try Again) | `DefaultAIErrorSuggestedQuestions` | Turn aborted. |
| **Model Output Error** | The LLM returns truncated or malformed JSON (e.g., `envelope unmarshal failed`). | `> **Model Output Error**` | **Formatting Error** (Button: Try Again) | `DefaultAIErrorSuggestedQuestions` | Turn aborted (often caused by `maxOutputTokens` limits). |
| **No Data Found** | An analytics query successfully completes but returns 0 rows. | (Context-specific text explaining no data) | None | `DefaultNoDataSuggestedQuestions` | Chart rendering is skipped, dynamic text explaining the empty result is returned. |

## Flow Overview

1. **Early Failures**: If the MCP connection fails before the agent loop starts, `webchat_handler.go` intercepts it and constructs a `HistoryEntry` containing the `ActionCard` and `DebugArtifacts`.
2. **During the Agent Loop**: If the LLM call fails, `agent.go` categorizes the error (using `aiconnectors.CategorizeLLMError`). 
    - For most errors, the loop aborts and returns an error template.
    - For `classify` overload errors, it intercepts the error, appends the Action Card, and forces the agent to act as a `product_guidance` agent for that turn.
3. **Output Parsing**: If the LLM succeeds but its JSON is malformed (e.g., due to token limits), `analytics.go` returns `Model Output Error` and attaches the Formatting Error action card.
4. **Persistence**: All of these states correctly persist the `action_card` and `debug_artifacts` to the `chat_messages` table via `persistReply`, so that they reload flawlessly if the user refreshes the page.

## API Response Structure

The `/api/v1/chat/send` endpoint returns a structured JSON payload (`WebChatResponse`). To support dynamic UI error rendering without hardcoded string matching on the frontend, the backend explicitly sets the `is_error` flag and provides the `action_card` payload.

```json
{
  "response": "> **Something went wrong on our end**\n> \n> I am having trouble connecting to my internal data tools right now. The technical details have been attached below for troubleshooting. Please try your request again in a few moments.",
  "conversationId": 123,
  "sessionId": "abc-123",
  "is_error": true,
  "action_card": {
    "title": "MCP Server Offline",
    "description": "The internal data server is unreachable (502 Bad Gateway).",
    "button_text": "Try Again",
    "action_url": "#retry"
  },
  "debug_artifacts": {
    "raw_llm_error": "MCP Connection Failed: 502 Bad Gateway"
  },
  "suggested_questions": [
    {
      "category": "Troubleshooting",
      "questions": ["Where do I configure my AI provider?"]
    }
  ]
}
```

By passing `is_error: true`, the React frontend (`ChatConversation.tsx`) automatically wraps the response in a red error blockquote and displays the provided `action_card`.


