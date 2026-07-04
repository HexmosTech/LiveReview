package prompts

import "context"

type conciseModeContextKey struct{}

// WithConciseMode returns a context flagging that comment content should be
// written as terse, telegraphic fragments rather than full grammatical
// sentences. Set this when a helper model will expand the leader's output
// afterward (see internal/review/helper_transform.go), so the leader isn't
// paying to write prose that gets rewritten anyway.
func WithConciseMode(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, conciseModeContextKey{}, enabled)
}

// ConciseModeFromContext reports whether concise mode was requested via ctx.
func ConciseModeFromContext(ctx context.Context) bool {
	enabled, _ := ctx.Value(conciseModeContextKey{}).(bool)
	return enabled
}

// BuildConciseModeSection returns instructions telling the model to write
// terse comment content when concise mode is enabled in ctx, or "" otherwise.
func BuildConciseModeSection(ctx context.Context) string {
	if !ConciseModeFromContext(ctx) {
		return ""
	}
	return `# Concise Draft Mode (overrides earlier style guidance for comment "content" WORDING ONLY)

A cheaper model will expand each comment's "content" into full, grammatical, user-facing prose afterward. You are writing an internal shorthand draft of "content", not the final comment. Ignore the earlier instruction to write clear, complete sentences for "content" — write a compressed note instead.

IMPORTANT — this changes "content" wording only, nothing else:
- Every selectivity rule above still applies at full strength: rare info comments, zero comments for trivial/behavior-preserving refactors, no comments for renames/constant-extraction/doc-nits/debug-log toggles, no near-duplicate comments about the same underlying issue. Making "content" cheap to write is not a reason to flag more things. If you would not have included a comment in full-sentence mode, do not include it here either.
- Judge severity, worthiness, and count exactly as if you were about to write full prose — decide what's worth saying first, then compress only the wording of what you decided to say.
- "fileSummaries[].summary" and "keyChanges" feed a separate synthesis step, not the expansion model — keep those normal, full sentences, unabbreviated.

Write "content" as a telegraphic note: keep only the specific noun/identifier/value and the verdict, drop subjects, articles, helper verbs, and connective words.
- BAD (too much like a finished sentence): "This function does not acquire a lock before mutating the shared cache, which can cause a race condition under concurrent requests."
- GOOD (telegraphic draft): "no lock before mutating shared cache; race under concurrent requests"
- BAD: "Consider extracting this repeated validation logic into a shared helper function to avoid duplication."
- GOOD: "dup validation logic x3; extract to shared helper"

Rules:
- Aim for well under 15 words per "content" value; drop anything the expansion model can infer.
- Do not drop the specific technical detail (identifiers, values, file/line-level facts) needed to reconstruct an accurate comment — compress grammar, not information.
- Severity, confidence, type, category, subcategory, and other structured fields must still be filled in normally; only "content" gets the compressed treatment.

`
}
