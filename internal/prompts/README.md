# Prompts Package

This package centralizes all AI prompt templates used throughout the LiveReview application. It provides a single source of truth for prompt management and makes it easy to:

- View all prompts in one place
- Update prompt templates without hunting through multiple files
- Maintain consistency across different AI providers
- Test and version control prompt changes

## Structure

### Files

- `builder.go` - Main prompt building functions with logic
- `templates.go` - Constants and template strings  
- `README.md` - This documentation

### Key Components

#### PromptBuilder
The main service that constructs prompts by combining templates with dynamic data.

**Primary Methods:**
- `BuildCodeReviewPrompt(diffs)` - Creates prompts for the main code review functionality (used by trigger-review endpoint)
- `BuildSummaryPrompt(technicalSummaries)` - Creates prompts for generating high-level summaries from structured per-file data

#### Templates
Constants containing all prompt text templates, organized by purpose:

- **System Roles**: `CodeReviewerRole`, `SummaryWriterRole`
- **Instructions**: `CodeReviewInstructions`, `ReviewGuidelines`, `CommentRequirements`
- **Output Formats**: `JSONStructureExample`, `SummaryStructure`
- **Guidelines**: `CommentClassification`, `LineNumberInstructions`

## Usage

### In AI Providers

Replace direct prompt construction with the centralized builder:

```go
// OLD (scattered across files):
func createReviewPrompt(diffs []*models.CodeDiff) string {
    var prompt strings.Builder
    prompt.WriteString("You are an expert code reviewer...")
    // ... lots of prompt building logic
    return prompt.String()
}

// NEW (centralized):
import "github.com/livereview/internal/prompts"

func createReviewPrompt(diffs []*models.CodeDiff) string {
    builder := prompts.NewPromptBuilder()
    return builder.BuildCodeReviewPrompt(diffs)
}
```

### In Batch Processing

```go
import "github.com/livereview/internal/prompts"

func synthesizeGeneralSummary(ctx context.Context, llm llms.Model, entries []prompts.TechnicalSummary) string {
   builder := prompts.NewPromptBuilder()
   promptText := builder.BuildSummaryPrompt(entries)
    
    summary, err := llms.GenerateFromSinglePrompt(ctx, llm, promptText)
    if err != nil {
        return "Error generating summary: " + err.Error()
    }
    return summary
}
```

## Migration Plan

### Files to Update

1. **`internal/ai/gemini/gemini.go`**
   - Replace `createReviewPrompt()` function
   - Import and use `prompts.NewPromptBuilder().BuildCodeReviewPrompt()`

2. **`internal/ai/langchain/provider.go`**  
   - Replace `createReviewPrompt()` function
   - Import and use `prompts.NewPromptBuilder().BuildCodeReviewPrompt()`

3. **`internal/batch/batch.go`**
   - Replace `synthesizeGeneralSummary()` prompt building
   - Import and use `prompts.NewPromptBuilder().BuildSummaryPrompt()`

4. **`internal/ai/aiconnectors_adapter.go`**
   - Implement the placeholder `createReviewPrompt()` function
   - Use the centralized prompt builder

5. **Test files**
   - Update `internal/ai/gemini/test_helpers.go`  
   - Update test cases to use centralized prompts

### Benefits of Migration

- **Single Source of Truth**: All prompts in one place
- **Consistency**: Same prompt logic across all AI providers
- **Maintainability**: Easy to update and improve prompts
- **Testing**: Easier to test prompt generation logic
- **Version Control**: Clear history of prompt changes
- **Debugging**: Centralized logging and debugging of prompts

## Future Enhancements

### Possible Extensions

1. **Template Versioning**: Support different prompt versions for A/B testing
2. **Context-Aware Prompts**: Adjust prompts based on programming language, file type, etc.
3. **Prompt Metrics**: Track which prompts perform better
4. **Dynamic Templates**: Load prompts from configuration files
5. **Multi-Language Support**: Different prompts for different natural languages

### Configuration Options

Consider adding configuration for:
- Temperature/creativity settings per prompt type
- Maximum token limits per prompt section
- Provider-specific adaptations
- User preference overrides

## Testing

The prompts package should include:
- Unit tests for prompt generation
- Integration tests with actual AI providers  
- Regression tests to ensure prompt consistency
- Performance tests for large diff sets

## Maintenance

When updating prompts:
1. Test with various code change scenarios
2. Validate JSON output structure 
3. Check line number accuracy
4. Verify comment classification logic
5. Update documentation if behavior changes