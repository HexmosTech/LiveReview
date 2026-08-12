import React, { useEffect, useState } from 'react';
import { ThinkingOrb } from 'thinking-orbs';

// One of these four caption pairs is picked at random each time a wait
// starts. The first word shows briefly, then the second holds for the rest
// of the wait - see PHASE_DELAY_MS below.
const CAPTION_PAIRS: [string, string][] = [
  ['Solving', 'Working'],
  ['Solving', 'Agent weaving'],
  ['Agent weaving', 'Agent shaping'],
  ['Agent breathing', 'Agent shaping'],
];

// thinking-orbs' nine states, keyed by the caption text that selects them.
const ORB_STATE: Record<string, 'working' | 'solving' | 'weaving' | 'shaping' | 'breathing'> = {
  Thinking: 'breathing',
  Solving: 'solving',
  Working: 'working',
  'Agent weaving': 'weaving',
  'Agent shaping': 'shaping',
  'Agent breathing': 'breathing',
};

const THINKING_PHASE_MS = 3000;
const FIRST_CAPTION_PHASE_MS = 3000;

// Cycles a caption through "Thinking" -> the pair's first word -> the pair's
// second word (which then holds until the response arrives and this
// component unmounts). A fresh random pair is picked every time a new wait
// starts.
function formatElapsed(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}m ${s}s`;
}

// This component's lifetime spans exactly one wait (mounted when the request
// starts, unmounted when the response arrives), so a plain mount-time clock
// doubles as the request's elapsed-time counter.
export function ThinkingIndicator() {
  const [caption, setCaption] = useState('Thinking');
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const pair = CAPTION_PAIRS[Math.floor(Math.random() * CAPTION_PAIRS.length)];
    setCaption('Thinking');

    const toFirst = setTimeout(() => setCaption(pair[0]), THINKING_PHASE_MS);
    const toSecond = setTimeout(() => setCaption(pair[1]), THINKING_PHASE_MS + FIRST_CAPTION_PHASE_MS);

    return () => {
      clearTimeout(toFirst);
      clearTimeout(toSecond);
    };
  }, []);

  useEffect(() => {
    const startedAt = Date.now();
    const tick = () => setElapsed(Math.floor((Date.now() - startedAt) / 1000));
    tick();
    const interval = setInterval(tick, 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="flex items-center gap-2 py-1">
      {/* The app has no light-mode variant (see .agents/design.md's dark
          background hierarchy), so theme is pinned rather than left on
          "auto" - auto would otherwise follow the OS's color-scheme
          preference, which has no relationship to this always-dark UI. */}
      <ThinkingOrb state={ORB_STATE[caption]} size={20} theme="dark" aria-label={caption} />
      <span className="text-sm text-slate-400">
        {caption}… <span className="text-slate-500">{formatElapsed(elapsed)}</span>
      </span>
    </div>
  );
}
