// Where to get a free Gemini API key (shown wherever we ask the user to add an AI provider).
export const GEMINI_API_KEYS_URL = 'https://aistudio.google.com/api-keys';

// The review pipeline answers with this text when the org has no AI provider and the Free plan needs one
// (see internal/aiselection/aiselection.go); it can arrive wrapped inside longer "failed to ..." messages.
export const isMissingAIProviderError = (message?: string | null): boolean =>
    !!message && /configure your own LLM API key|\(BYOK\)/i.test(message);
