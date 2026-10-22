# Settings → Managed AI

**Route:** `/settings#managed-ai`
**Component:** `ui/src/pages/Settings/ManagedAITab.tsx`
**Who sees it:** super admins, LiveReview Cloud only

## Purpose

Chooses the provider, model and key behind the **LiveReview AI Model**: the
AI that paid organizations, and Free organizations created through
`/gh-login?src=launch-trial`, review on without adding their own key. The
setting is shared by every Cloud organization.

There are two tiers:

- **Main model** (`default`): writes the review comments.
- **Helper model** (`default_lite`): rewrites comments when the helper stage is
  on.

Each tier can hold several configurations (for example the old Vertex AI one
and a new OpenRouter one). Exactly one is active; reviews always use the active
one, and a switch takes effect on the next review.

## Key actions

- **Add configuration** — pick a provider (OpenRouter, DeepSeek, Gemini
  Enterprise / Vertex AI, Gemini, OpenAI, Claude), a model (the same searchable model
  picker as AI Providers, for example `deepseek/deepseek-v4-flash` on OpenRouter;
  **Custom model…** takes any other model ID) and the key,
  and optionally a label. **Test and add** makes a test call first and only saves
  the configuration if the model answers.
  Gemini Enterprise takes a service-account JSON key and a location (`us`, `eu`
  or `global`; Gemini 3.x isn't served from single regions such as
  `us-central1`). New configurations start inactive.
- **Test** — makes one small call with the form's values, or with a saved
  configuration's stored key. Nothing is saved.
- **Activate** — switches the tier to that configuration after testing it again;
  if the test fails nothing changes. The previous configuration stays, inactive,
  so it's one click to switch back.
- **Edit** — change the model, key, location or label. Leave the key blank to
  keep the stored one. **Test and save** only saves the changes if a test call
  with them succeeds.

Keys are write-only: the page shows only whether a key is set. Changes need a
signed-in session; API keys are refused.

## Related pages

- [Settings overview](settings-overview.md)
- [AI Providers](../ai/ai-providers.md) (an organization's own keys)
- [Plan & Usage](plan-and-usage.md)
