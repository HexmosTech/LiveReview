# Gemini 3.8 Flash Migration & Parameter Deprecation Fixes

**Date**: October 10, 2026

## Overview
Google AI Studio deprecated the `gemini-3.5-flash` model, automatically routing to `3.6-flash`, while announcing the general availability of `gemini-3.8-flash`. Furthermore, the Gemini 3.x models officially deprecated all sampling parameters (`temperature`, `top_p`, `top_k`) and the `thinking_budget` variable. Sending these parameters now strictly returns a `400 INVALID_ARGUMENT` error.

This document serves as the permanent record of the codebase updates applied to ensure LiveReview remains 100% compliant with the Gemini 3.8 API rules without crashing.

## Changes Implemented

### 1. `langchaingo` Integration Sanitization
**File**: `internal/ai/langchain/provider.go`
- **Issue**: The underlying `github.com/tmc/langchaingo/llms/googleai` SDK sets a default temperature of `0.5`, and LiveReview was actively passing an `effectiveTemperature` payload downstream. Additionally, a linter flagged a long string of boolean `OR` comparisons for provider checking as a maintainability risk.
- **Fix**: Created a localized `buildCallOptions()` helper function mimicking the behavior of `internal/aiconnectors/connector.go`. This dynamically intercepts calls to `GenerateFromSinglePrompt` and `AggregateAndCombineOutputs`. If the active provider is `gemini`, `googleai`, `gemini-enterprise`, or `vertex` (now cleanly verified using a `switch` statement), it entirely skips attaching the `llms.WithTemperature()` option to the request, protecting the payload.

### 2. PreLivi Python Script Fix
**File**: `scripts/prelivi/interpretation.py`
- **Issue**: The direct REST call to `generativelanguage.googleapis.com` had a hardcoded `"temperature": 0.2` injected into the `generationConfig` block.
- **Fix**: Deleted the `temperature` key entirely. The request now solely depends on `maxOutputTokens`.

### 3. Native Gemini Connector
**File**: `internal/ai/gemini/gemini.go`
- **Issue**: Historical references to `temperature` still existed in the struct logging (`p.Temperature`) and initialization fields, which caused build compilation errors when purged.
- **Fix**: Dropped the temperature assignment and `fmt.Fprintf(f, "Temperature: %f\n", p.Temperature)` logging. The `internal/ai/gemini` package compiles and runs without expecting sampling configs.

### 4. Configuration Hygiene
**Files**: `config/livereview.toml.example`, `internal/config/config.go`
- **Issue**: Example TOML configs and the automatic config generator still pointed to `gemini-2.0-flash` and generated sample configurations with `temperature = 0.2`.
- **Fix**: Refactored the sample generator and TOML files to use `gemini-3.8-flash` as the standard, actively scrubbing out the temperature variables so new deployments don't try to submit invalid configs.

### 5. Cleanup
- `internal/ai/gemini/gemini.go.bak` and `internal/ai/gemini/.gemini.go.broken` were removed as they were dead-tracked code still reflecting the old temperature logic.

## Note on Database Migrations
We deliberately opted **not** to force a database-level SQL migration over the `ai_connectors` table for existing users configured to `3.5-flash` or `3.6-flash`. Because Google handles routing on their backend seamlessly, active user settings have been preserved as-is.
