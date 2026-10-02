# MCP LLM Error Handling

When interacting with multiple AI Providers (Google, OpenAI, Anthropic) via `langchaingo`, LLM calls can fail in messy, unpredictable ways. Providers may return HTTP 503s for overload, 429s for rate limits, or 401s for revoked keys. 

If we return these raw errors directly to the MCP client (the chat UI), the turn crashes and the user sees ugly stack traces (e.g., `googleapi: Error 503`).

To solve this, we use a **centralized UI fallback mechanism** that traps raw SDK errors and gracefully degrades the UI without disrupting the core ReAct loop.

## The Architecture

The error handling logic lives in `internal/mcpagent/provider.go` and consists of two functions:

### 1. `CategorizeLLMError(err error) LLMErrorCategory`
This function intercepts an error, inspects it for `langchaingo` normalizations, unwraps leaked provider-specific errors (like `*googleapi.Error`), and falls back to string matching. 

It maps the messy error into one of four clean categories:
* `ErrCategoryAuth` (401, 403, 404, invalid keys, revoked models)
* `ErrCategoryOverload` (429, 503, rate limits, high demand)
* `ErrCategoryTimeout` (504, context deadline exceeded)
* `ErrCategoryUnknown` (Prompt parsing errors, network disconnects)

### 2. `GetLLMFallbackMessage(cat LLMErrorCategory) string`
This function takes a category and returns a beautifully formatted, non-emoji Markdown string to show the user. 
* If it returns a non-empty string, **the turn is considered saved**. We show the user this message.
* If it returns `""` (for `ErrCategoryUnknown`), the error is fatal and should be bubbled up.

## How to use it

We deliberately **do not** wrap the original `a.provider.Complete(...)` function. We keep the core LLM signatures untouched. Instead, you just "pick up" the error immediately after the LLM call fails.

### Example (from `agent.go`)

```go
response, usage, err := a.provider.Complete(ctx, history, tools, completeOpts...)
if err != nil {
    // 1. Pick up the error and categorize it
    errCategory := CategorizeLLMError(err)
    
    // 2. Ask for a clean UI message
    msg := GetLLMFallbackMessage(errCategory)
    
    // 3. If there is no clean fallback, bubble up the fatal error
    if msg == "" {
        return "", history, nil, nil, fmt.Errorf("llm completion failed: %w", err)
    }

    // 4. Otherwise, inject the fallback message as an AI response and return a nil error!
    // This gracefully completes the turn and renders the message in the UI.
    history = append(history, HistoryEntry{
        "role":    "assistant",
        "content": msg,
        "text":    msg,
    })
    return msg, history, nil, nil, nil
}
```

## Adding new Fallback Scenarios

If you need to handle a new failure mode (e.g. `ContextTooLarge`):
1. Add `ErrCategoryContextSize` to the const block in `provider.go`.
2. Add the detection logic to `CategorizeLLMError`.
3. Add the exact Markdown message you want the user to see to the `switch` block in `GetLLMFallbackMessage`. 

This guarantees every MCP LLM call across the app degrades consistently.
