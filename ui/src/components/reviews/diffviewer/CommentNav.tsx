// Ported from git-lrc:internal/staticserve/static/components/CommentNav.js (floating
// prev/next navigator + j/k shortcuts) as of the git-lrc HEAD current when this port
// was written — simplified: git-lrc reconciles position against a set of AI-hidden
// comments (a feature LiveReview's viewer doesn't have), so this is a plain wrapping
// index over the visible-comment list, no reconciliation needed.
import React, { useCallback, useEffect, useState } from 'react';

export interface NavComment {
  id: string;
}

interface CommentNavProps {
  comments: NavComment[];
  active: boolean;
}

const CommentNav: React.FC<CommentNavProps> = ({ comments, active }) => {
  const [currentIdx, setCurrentIdx] = useState(-1);

  useEffect(() => {
    setCurrentIdx(-1);
  }, [comments.length]);

  const goTo = useCallback((idx: number) => {
    if (comments.length === 0) return;
    const wrapped = ((idx % comments.length) + comments.length) % comments.length;
    setCurrentIdx(wrapped);
    const el = document.getElementById(comments[wrapped].id);
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }, [comments]);

  const goNext = useCallback(() => goTo(currentIdx + 1), [goTo, currentIdx]);
  const goPrev = useCallback(() => goTo(currentIdx - 1 < 0 && currentIdx === -1 ? -1 : currentIdx - 1), [goTo, currentIdx]);

  useEffect(() => {
    if (!active) return;
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
      if ((e.target as HTMLElement)?.isContentEditable) return;
      if (comments.length === 0) return;
      if (e.key === 'j' || e.key === 'J') {
        e.preventDefault();
        goNext();
      } else if (e.key === 'k' || e.key === 'K') {
        e.preventDefault();
        goPrev();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [active, comments.length, goNext, goPrev]);

  if (!active || comments.length === 0) return null;

  return (
    <div className="fixed bottom-6 right-6 z-40 flex items-center gap-2 rounded-full border border-slate-600 bg-slate-800 px-3 py-2 shadow-lg">
      <button type="button" onClick={goPrev} title="Previous comment (k)" className="px-1 text-slate-300 hover:text-white">
        ‹
      </button>
      <span className="font-mono text-xs text-slate-300">{currentIdx >= 0 ? `${currentIdx + 1} / ${comments.length}` : `— / ${comments.length}`}</span>
      <button type="button" onClick={goNext} title="Next comment (j)" className="px-1 text-slate-300 hover:text-white">
        ›
      </button>
    </div>
  );
};

export default CommentNav;
