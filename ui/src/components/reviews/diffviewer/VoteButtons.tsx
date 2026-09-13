// Ported from git-lrc:internal/staticserve/static/components/FeedbackPopup.js (as of HEAD)
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FeedbackSourceType, retractFeedback, submitFeedback } from '../../../api/feedback';

const DOWN_TAGS = ['False positive', 'Wrong severity', 'Missed something', 'Hard to act on'];

interface VoteButtonsProps {
  reviewId: number;
  sourceType: FeedbackSourceType;
  aiCommentId?: number;
  commentContent?: string;
  codeExcerpt?: string;
  filePath?: string;
  severity?: string;
  size?: 'sm' | 'md';
}

type VoteState = 'up' | 'down' | null;
type PopupMode = 'hover' | 'click' | 'submitted' | null;

const VoteButtons: React.FC<VoteButtonsProps> = ({
  reviewId,
  sourceType,
  aiCommentId,
  commentContent,
  codeExcerpt,
  filePath,
  severity,
  size = 'sm',
}) => {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  const [vote, setVote] = useState<VoteState>(null);
  const [feedbackId, setFeedbackId] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);

  const [popupVisible, setPopupVisible] = useState(false);
  const [popupMode, setPopupMode] = useState<PopupMode>(null);
  const [popupPos, setPopupPos] = useState({ top: 0, left: 0 });
  const [popupAnim, setPopupAnim] = useState({ opacity: 0, shift: -6 });

  const [feedbackText, setFeedbackText] = useState('');
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState(false);

  const autoTimer = useRef<number | null>(null);
  const hoverTimer = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (autoTimer.current) { clearTimeout(autoTimer.current); autoTimer.current = null; }
    if (hoverTimer.current) { clearTimeout(hoverTimer.current); hoverTimer.current = null; }
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  const retract = useCallback(() => {
    if (feedbackId !== null) {
      retractFeedback(feedbackId).catch(() => {});
      setFeedbackId(null);
    }
  }, [feedbackId]);

  const calculatePos = (wrapperEl: HTMLElement, popupEl?: HTMLElement | null) => {
    const r = wrapperEl.getBoundingClientRect();
    const w = 400;
    const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
    const viewportHeight = document.documentElement.clientHeight || window.innerHeight;

    let targetLeft = r.right - w;
    if (targetLeft + w > viewportWidth - 16) {
      targetLeft = viewportWidth - w - 16;
    }
    if (targetLeft < 16) {
      targetLeft = 16;
    }

    const h = popupEl?.offsetHeight || 260;
    let top = r.bottom + 4;
    if (r.top >= viewportHeight / 2) {
      top = Math.max(16, r.top - 4 - h);
    }

    return { top, left: targetLeft };
  };

  const show = useCallback((mode: PopupMode) => {
    if (wrapperRef.current) {
      setPopupPos(calculatePos(wrapperRef.current, popupRef.current));
    }
    setPopupVisible(true);
    setPopupMode(mode);
    setPopupAnim({ opacity: 0, shift: -6 });
  }, []);

  const hide = useCallback(() => {
    setPopupAnim({ opacity: 0, shift: -4 });
    window.setTimeout(() => {
      setPopupVisible(false);
      setPopupMode(null);
    }, 280);
  }, []);

  const startAuto = useCallback((ms = 6000) => {
    if (autoTimer.current) clearTimeout(autoTimer.current);
    autoTimer.current = window.setTimeout(hide, ms);
  }, [hide]);

  useEffect(() => {
    if (!popupVisible || !wrapperRef.current) return;
    setPopupPos(calculatePos(wrapperRef.current, popupRef.current));
    if (popupAnim.opacity === 0) {
      requestAnimationFrame(() => requestAnimationFrame(() => {
        setPopupAnim({ opacity: 1, shift: 0 });
      }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [popupVisible, popupMode]);

  // Close popup on click outside
  useEffect(() => {
    if (!popupVisible) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (
        popupRef.current && !popupRef.current.contains(e.target as Node) &&
        wrapperRef.current && !wrapperRef.current.contains(e.target as Node)
      ) {
        hide();
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [popupVisible, hide]);

  const cast = async (next: VoteState) => {
    if (busy) return;
    setBusy(true);
    setDenied(false);
    try {
      retract();
      if (next === vote) {
        setVote(null);
        if (popupVisible) hide();
        return;
      }
      const res = await submitFeedback({
        review_id: reviewId,
        ai_comment_id: aiCommentId,
        vote_type: next as 'up' | 'down',
        source_type: sourceType,
        comment_content: commentContent,
        code_excerpt: codeExcerpt,
        file_path: filePath,
        severity,
      });
      setFeedbackId(res.id);
      setVote(next);
      if (next === 'up') {
        if (popupVisible) hide();
      } else {
        show('click');
        startAuto(10000);
      }
    } catch (err) {
      if ((err as any)?.status === 403) {
        setDenied(true);
        if (popupVisible) hide();
      }
    } finally {
      setBusy(false);
    }
  };

  const handleDislikeMouseEnter = () => {
    if (hoverTimer.current) { clearTimeout(hoverTimer.current); hoverTimer.current = null; }
    if (popupMode === 'click' || popupMode === 'submitted') return;
    if (vote === 'down' && (!popupVisible || popupMode !== 'hover')) {
      show('hover');
    }
  };

  const handleDislikeMouseLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') return;
    hoverTimer.current = window.setTimeout(() => {
      if (popupMode === 'hover') hide();
    }, 300);
  };

  const onPopupEnter = () => {
    if (hoverTimer.current) { clearTimeout(hoverTimer.current); hoverTimer.current = null; }
  };

  const onPopupLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') return;
    hoverTimer.current = window.setTimeout(() => {
      if (popupMode === 'hover') hide();
    }, 200);
  };

  const handleSubmit = async (e: React.MouseEvent) => {
    e.stopPropagation();
    clearTimers();
    setSubmitError(false);
    setSubmitting(true);
    try {
      await submitFeedback({
        review_id: reviewId,
        ai_comment_id: aiCommentId,
        vote_type: vote || 'down',
        source_type: sourceType,
        tags: [...selectedTags],
        feedback_text: feedbackText,
        comment_content: commentContent,
        file_path: filePath,
        severity,
        code_excerpt: codeExcerpt,
      });
    } catch {
      setSubmitting(false);
      setSubmitError(true);
      startAuto(6000);
      return;
    }
    setSubmitting(false);
    setPopupMode('submitted');
    startAuto(3000);
  };

  const dimClass = size === 'sm' ? 'h-7 w-7' : 'h-8 w-8';

  if (denied) {
    return <span className="text-[11px] text-slate-600" title="Only the review's creator can leave feedback">Feedback unavailable</span>;
  }

  return (
    <div className="relative inline-flex items-center gap-1.5 font-sans" ref={wrapperRef}>
      {/* Upvote button (NO hover popup) */}
      <button
        type="button"
        disabled={busy}
        onClick={() => cast('up')}
        title="Helpful"
        className={`inline-flex ${dimClass} items-center justify-center rounded border transition-all ${
          vote === 'up'
            ? 'border-emerald-500 bg-emerald-500/15 text-emerald-400'
            : 'border-slate-700 bg-slate-800/40 text-slate-400 hover:border-emerald-500/40 hover:bg-emerald-500/10 hover:text-emerald-300'
        }`}
      >
        <svg width={size === 'sm' ? 13 : 15} height={size === 'sm' ? 13 : 15} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
          <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z" />
          <path d="M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3" />
        </svg>
      </button>

      {/* Downvote button (HAS hover popup) */}
      <button
        type="button"
        disabled={busy}
        onClick={() => cast('down')}
        onMouseEnter={handleDislikeMouseEnter}
        onMouseLeave={handleDislikeMouseLeave}
        title="Not helpful"
        className={`inline-flex ${dimClass} items-center justify-center rounded border transition-all ${
          vote === 'down'
            ? 'border-red-500 bg-red-500/15 text-red-400'
            : 'border-slate-700 bg-slate-800/40 text-slate-400 hover:border-red-500/40 hover:bg-red-500/10 hover:text-red-300'
        }`}
      >
        <svg width={size === 'sm' ? 13 : 15} height={size === 'sm' ? 13 : 15} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
          <path d="M10 15v4a3 3 0 0 0 3 3l4-9V2H5.72a2 2 0 0 0-2 1.7l-1.38 9a2 2 0 0 0 2 2.3H10z" />
          <path d="M17 2h2.67A2.31 2.31 0 0 1 22 4v7a2.31 2.31 0 0 1-2.33 2H17" />
        </svg>
      </button>

      {/* Dislike Feedback Popup Box */}
      {popupVisible &&
        createPortal(
          <div
            ref={popupRef}
            className="fixed z-[20000] w-[400px] rounded-xl border border-slate-700/80 bg-[#151f2e] p-4 text-slate-200 shadow-2xl font-sans"
            style={{
              top: popupPos.top,
              left: popupPos.left,
              opacity: popupAnim.opacity,
              transform: `translateY(${popupAnim.shift}px)`,
              transition: 'opacity 0.28s ease, transform 0.28s ease',
            }}
            onMouseEnter={onPopupEnter}
            onMouseLeave={onPopupLeave}
            onClick={(e) => e.stopPropagation()}
          >
            {popupMode === 'submitted' ? (
              <div className="flex items-center gap-2 py-1">
                <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="text-emerald-400">
                  <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                  <polyline points="9 11 12 14 22 4" />
                </svg>
                <span className="text-sm font-semibold text-slate-100">
                  Thanks. We'll work on making it better.
                </span>
              </div>
            ) : (
              <div>
                <div className="mb-3 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="text-red-400">
                      <path d="M10 15v4a3 3 0 0 0 3 3l4-9V2H5.72a2 2 0 0 0-2 1.7l-1.38 9a2 2 0 0 0 2 2.3H10z" />
                      <path d="M17 2h2.67A2.31 2.31 0 0 1 22 4v7a2.31 2.31 0 0 1-2.33 2H17" />
                    </svg>
                    <span className="text-sm font-semibold text-slate-100">
                      We're sorry it didn't meet your expectations!
                    </span>
                  </div>
                  <button
                    type="button"
                    onClick={hide}
                    className="rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
                    title="Close"
                  >
                    <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                      <line x1="18" y1="6" x2="6" y2="18" />
                      <line x1="6" y1="6" x2="18" y2="18" />
                    </svg>
                  </button>
                </div>

                <div className="mb-3">
                  <div className="mb-2 text-[11px] font-bold tracking-wider text-slate-400 uppercase">
                    What went wrong?
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {DOWN_TAGS.map((tag) => {
                      const active = selectedTags.has(tag);
                      return (
                        <button
                          key={tag}
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            setSelectedTags((prev) => {
                              const next = new Set(prev);
                              if (next.has(tag)) next.delete(tag);
                              else next.add(tag);
                              return next;
                            });
                          }}
                          className={`rounded-full border px-3 py-1 text-xs transition-all ${
                            active
                              ? 'border-red-500/80 bg-red-500/20 text-red-300 font-medium'
                              : 'border-slate-700 bg-slate-800/60 text-slate-300 hover:border-slate-600 hover:bg-slate-800'
                          }`}
                        >
                          {tag}
                        </button>
                      );
                    })}
                  </div>
                </div>

                <textarea
                  value={feedbackText}
                  onChange={(e) => setFeedbackText(e.target.value)}
                  onClick={(e) => e.stopPropagation()}
                  placeholder="Tell us more... (optional)"
                  rows={2}
                  className="min-h-[64px] w-full resize-y rounded-md border border-slate-700 bg-slate-900/90 p-2.5 text-xs text-slate-200 outline-none placeholder:text-slate-500 focus:border-slate-500 focus:ring-1 focus:ring-slate-500"
                />

                <div className="mt-2 text-[11px] leading-relaxed text-slate-400">
                  This comment and the code block will be sent to Hexmos. A human will review it to understand and ship a fix.
                </div>

                {submitError && (
                  <div className="mt-1.5 text-[11px] text-red-400">
                    Failed to send — please try again.
                  </div>
                )}

                <button
                  type="button"
                  onClick={handleSubmit}
                  disabled={submitting}
                  className="mt-3 rounded-md bg-[#2d5be3] px-4 py-1.5 text-xs font-semibold text-white shadow-sm hover:bg-blue-600 active:bg-blue-700 disabled:opacity-50 transition-colors"
                >
                  {submitting ? 'Sending…' : 'Submit More'}
                </button>
              </div>
            )}
          </div>,
          document.body
        )}
    </div>
  );
};

export default VoteButtons;




