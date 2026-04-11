## unified loc pricing spec (symbolic)

This document defines one canonical algorithm for all review paths.
The algorithm is symbolic and implementation-agnostic.

## 1. plan-level inputs

plan_price_usd = P
plan_effective_loc_limit = L

input_chars_per_loc = 120
output_chars_per_loc = 87
chars_per_token = 4

input_cost_per_million_tokens_usd = R_in
output_cost_per_million_tokens_usd = R_out

## 2. budget split (mandatory)

loc_budget_usd = P * (1/3)
context_budget_usd = P * (1/3)
ops_reserved_usd = P * (1/3)

// ops_reserved_usd is non-consumable in this algorithm.

## 3. derived constants

input_tokens_per_loc = input_chars_per_loc / chars_per_token
output_tokens_per_loc = output_chars_per_loc / chars_per_token

context_total_input_tokens_budget = context_budget_usd * 10^6 / R_in
context_tokens_allowance_per_loc = context_total_input_tokens_budget / L

// context_tokens_allowance_per_loc is the central control knob.
// input diff tokens are always deterministic from LOC, not provider-reported.

## 4. request classification

diff_input = actual changed code input only
context_input = all non-diff input text

// context_input includes prompts, instructions, policies, repo guidance,
// metadata, and any extra non-diff material.

## 5. per-batch algorithm

for each batch in review_operation:

    raw_loc_batch = get_line_count(diff_input_batch)
    diff_input_tokens_batch = raw_loc_batch * input_tokens_per_loc
    diff_input_cost_usd_batch = diff_input_tokens_batch * R_in / 10^6

    context_chars_batch = get_char_count(context_input_batch)
    context_tokens_batch = get_provider_or_estimated_tokens(context_input_batch)
    context_input_cost_usd_batch = context_tokens_batch * R_in / 10^6

    final_prompt_batch = build_prompt(diff_input_batch, context_input_batch)

    // provider-reported total input tokens are optional observability only.
    provider_total_input_tokens_batch = get_provider_input_tokens(final_prompt_batch)

    llm_output_batch = call_llm(final_prompt_batch)
    output_tokens_batch = get_provider_output_tokens(llm_output_batch)

    input_cost_usd_batch = diff_input_cost_usd_batch + context_input_cost_usd_batch
    output_cost_usd_batch = output_tokens_batch * R_out / 10^6
    total_cost_usd_batch = input_cost_usd_batch + output_cost_usd_batch

    allowed_context_tokens_batch = raw_loc_batch * context_tokens_allowance_per_loc
    extra_context_tokens_batch = max(0, context_tokens_batch - allowed_context_tokens_batch)

    // convert context overrun into extra effective LOC.
    extra_effective_loc_batch = ceil(extra_context_tokens_batch / context_tokens_allowance_per_loc)
    effective_loc_batch = raw_loc_batch + extra_effective_loc_batch

    // output overage conversion is intentionally ignored in v1.

    cumulative_raw_loc += raw_loc_batch
    cumulative_effective_loc += effective_loc_batch
    cumulative_diff_input_tokens += diff_input_tokens_batch
    cumulative_context_tokens += context_tokens_batch
    cumulative_provider_total_input_tokens += provider_total_input_tokens_batch
    cumulative_output_tokens += output_tokens_batch
    cumulative_input_cost_usd += input_cost_usd_batch
    cumulative_output_cost_usd += output_cost_usd_batch
    cumulative_total_cost_usd += total_cost_usd_batch

## 6. invariants

1) budget math invariant
    loc_budget_usd + context_budget_usd + ops_reserved_usd = P

2) ops reserve invariant
    ops_reserved_usd is never consumed by this accounting algorithm.

3) effective loc invariant
    effective_loc_batch >= raw_loc_batch

4) deterministic diff invariant
   diff_input_tokens_batch = raw_loc_batch * input_tokens_per_loc.
   this relation is independent of provider token reports.

5) context trigger invariant
    if context_tokens_batch <= allowed_context_tokens_batch,
    then extra_effective_loc_batch = 0.

6) deterministic invariant
    same inputs must produce exactly same effective_loc output.

## 7. deterministic rounding

1) token counts are integers.
2) effective LOC conversion uses ceil.
3) USD calculations use fixed precision (no nondeterministic float drift).

## 8. plan scaling rule

All plans scale linearly from base profile.

example:
P=32, L=100000
P=64, L=200000
P=128, L=400000
...
up to max tier L=3200000.

All formulas above remain unchanged across tiers.

## 9. explicit v1 exclusions

These are intentionally out of scope for this document:

1) retries and idempotency behavior
2) failure charging semantics
3) billing cycle reset semantics
4) storage/database shape

This file defines only the pricing and effective LOC algorithm.




