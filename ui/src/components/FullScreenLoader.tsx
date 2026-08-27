import React from 'react';

// Single shared full-screen loading visual - reused across every full-screen loading state
// (App shell's auth-state gate, the route-chunk Suspense fallback, and the cloud login/
// provisioning wait in Cloud.tsx) so consecutive loading states read as one continuous screen
// instead of a stack of differently-styled spinners swapping in sequence.
//
// Deliberately mirrors the static pre-React #lr-boot screen (public/index.html) pixel-for-pixel
// - same bg-slate-900 (#0f172a), same logo, same column layout, logo-then-spinner - so the
// handoff from that static screen into this React one (and every loading state after it) reads
// as one continuous screen with only the caption text changing, not a stack of different-looking
// loaders.
//
// The spinner sits in its OWN centered row, with the caption in a separate row below it - not
// side-by-side in one row. Side-by-side would center the (spinner+text) pair as a unit, which
// shifts the spinner a few pixels left of where it sits on the boot screen (which has no text
// at all, so its spinner is centered alone) - visible as the loader "jumping" between screens
// even though it's the same component. Keeping the spinner in its own row means its horizontal
// position never depends on whether a caption is present, so it never moves.
export const FullScreenLoader: React.FC<{ text: string }> = ({ text }) => (
    <div className="min-h-screen flex items-center justify-center bg-slate-900 text-slate-100">
        <div className="flex flex-col items-center gap-4">
            <img src="/assets/logo-horizontal.svg" alt="LiveReview" width={240} height={64} className="h-16 w-auto" />
            <div className="flex flex-col items-center gap-3">
                {/* border-blue-500 (#3b82f6) matches #lr-boot's .lr-spin color exactly - the
                    earlier border-indigo-500 was a visibly different, more purple blue. */}
                <div className="h-10 w-10 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" aria-hidden />
                <span>{text}</span>
            </div>
        </div>
    </div>
);
