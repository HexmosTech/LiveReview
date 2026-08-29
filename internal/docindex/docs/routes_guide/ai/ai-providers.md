# AI Providers

**Route(s):** `/ai`, `/ai/:provider`, `/ai/:provider/:action`,
`/ai/:provider/:action/:connectorId`
**Component:** `ui/src/pages/AIProviders/AIProviders.tsx`

## Purpose

Manage AI connectors — the LLM backends (Gemini, OpenAI, Anthropic,
Ollama, DeepSeek, AWS Bedrock, etc.) LiveReview uses to run code reviews.
Lists supported providers (fetched from a backend catalog,
`getAIProviderCatalog`), lets the user configure connectors for each, and
holds org-wide review AI settings (`getReviewAISettings` /
`updateReviewAISettings`, e.g. which connector is the default).

## Who can access it

Any authenticated org member can view; adding/editing connectors is
typically an owner/admin action (enforced server-side).

## Key actions

- Browse supported providers and their connection requirements.
- Add/edit/delete an AI connector, including provider-specific forms (e.g.
  Ollama, Bedrock have dedicated forms for their unique config).
- Set org-wide review AI settings / adaptive review behavior
  (`AdaptiveReviewInfo`).
- View usage tips (`UsageTips`) for getting good review quality.

## Related pages

- [New Review](../reviews/new-review.md)
- [Settings](../settings/settings-overview.md)
