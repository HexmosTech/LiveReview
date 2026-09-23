// Ported from git-lrc:internal/staticserve/static/components/FeedbackPopup.js (as of HEAD)
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FeedbackSourceType, retractFeedback, submitFeedback, getImpactStats, ImpactStat } from '../../../api/feedback';

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

const buildLinkedinText = (stats: ImpactStat[] | null) => {
  const get = (label: string) => {
    const s = (stats || []).find((x) => x.label === label);
    return s != null ? s.value : '—';
  };
  return `🚀 Shipping with confidence — here's my code review impact since Jan 2025:

✅ ${get('Total Reviews')} reviews completed
🐛 ${get('Bugs Caught Pre-Prod')} bugs caught before production
🔍 ${get('Issues Found')} total issues found
🔴 ${get('Critical')} critical issues found
🟠 ${get('Errors')} errors caught
🟡 ${get('Warnings')} warnings flagged

Using LiveReview to AI-review every commit before it lands.

⭐ Star it if you find it useful: https://github.com/HexmosTech/LiveReview

#CodeReview #DevOps #SoftwareEngineering #AI`;
};

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
  const [popupSource, setPopupSource] = useState<'up' | 'down' | null>(null);
  const [popupPos, setPopupPos] = useState({ top: 0, left: 0 });
  const [popupAnim, setPopupAnim] = useState({ opacity: 0, shift: -6 });

  const [feedbackText, setFeedbackText] = useState('');
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState(false);

  const [impactStats, setImpactStats] = useState<ImpactStat[] | null>(null);
  const [statsExpanded, setStatsExpanded] = useState(false);
  const [linkedinOpen, setLinkedinOpen] = useState(false);
  const [linkedinOpacity, setLinkedinOpacity] = useState(0);
  const [linkedinText, setLinkedinText] = useState('');
  const [snackbar, setSnackbar] = useState(false);

  const autoTimer = useRef<number | null>(null);
  const hoverTimer = useRef<number | null>(null);
  const snackTimer = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (autoTimer.current) { window.clearTimeout(autoTimer.current); autoTimer.current = null; }
    if (hoverTimer.current) { window.clearTimeout(hoverTimer.current); hoverTimer.current = null; }
    if (snackTimer.current) { window.clearTimeout(snackTimer.current); snackTimer.current = null; }
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  useEffect(() => {
    if (!linkedinOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeLinkedin();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [linkedinOpen]);

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
    if (targetLeft + w > viewportWidth - 16) targetLeft = viewportWidth - w - 16;
    if (targetLeft < 16) targetLeft = 16;

    const h = popupEl?.offsetHeight || 260;
    let top = r.bottom + 4;
    if (r.top >= viewportHeight / 2) {
      top = Math.max(16, r.top - 4 - h);
    }

    return { top, left: targetLeft };
  };

  const show = useCallback((mode: PopupMode, source?: 'up' | 'down') => {
    if (wrapperRef.current) {
      setPopupPos(calculatePos(wrapperRef.current, popupRef.current));
    }
    setPopupVisible(true);
    setPopupMode(mode);
    if (source) setPopupSource(source);
    setPopupAnim({ opacity: 0, shift: -6 });
  }, []);

  const hide = useCallback(() => {
    setPopupAnim({ opacity: 0, shift: -4 });
    window.setTimeout(() => {
      setPopupVisible(false);
      setPopupMode(null);
      setPopupSource(null);
      setStatsExpanded(false);
    }, 280);
  }, []);

  const startAuto = useCallback((ms = 6000) => {
    if (autoTimer.current) window.clearTimeout(autoTimer.current);
    autoTimer.current = window.setTimeout(hide, ms);
  }, [hide]);

  useEffect(() => {
    if (!popupVisible || !wrapperRef.current || !popupRef.current) return;
    setPopupPos(calculatePos(wrapperRef.current, popupRef.current));
    if (popupAnim.opacity === 0) {
      requestAnimationFrame(() => requestAnimationFrame(() => {
        setPopupAnim({ opacity: 1, shift: 0 });
      }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [popupVisible, popupMode, statsExpanded]);

  // Close popup on click outside
  useEffect(() => {
    if (!popupVisible) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (
        popupRef.current && !popupRef.current.contains(e.target as Node) &&
        wrapperRef.current && !wrapperRef.current.contains(e.target as Node) &&
        !linkedinOpen
      ) {
        hide();
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [popupVisible, hide, linkedinOpen]);

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
      if (next === 'up') {
        getImpactStats((stats) => setImpactStats(stats));
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
        show('click', 'up');
        startAuto(10000);
      } else {
        show('click', 'down');
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

  const handleLikeMouseEnter = () => {
    if (hoverTimer.current) { window.clearTimeout(hoverTimer.current); hoverTimer.current = null; }
    if (popupMode === 'click' || popupMode === 'submitted') return;
    if (!popupVisible || popupMode !== 'hover' || popupSource !== 'up') {
      getImpactStats((stats) => setImpactStats(stats));
      show('hover', 'up');
    }
  };

  const handleDislikeMouseEnter = () => {
    if (hoverTimer.current) { window.clearTimeout(hoverTimer.current); hoverTimer.current = null; }
    if (popupMode === 'click' || popupMode === 'submitted') return;
    if (vote === 'down' && (!popupVisible || popupMode !== 'hover' || popupSource !== 'down')) {
      show('hover', 'down');
    }
  };

  const handleMouseLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') return;
    hoverTimer.current = window.setTimeout(() => {
      if (popupMode === 'hover') hide();
    }, 300);
  };

  const onPopupEnter = () => {
    if (hoverTimer.current) { window.clearTimeout(hoverTimer.current); hoverTimer.current = null; }
  };

  const onPopupLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') return;
    hoverTimer.current = window.setTimeout(() => {
      if (popupMode === 'hover') hide();
    }, 200);
  };

  const handleSubmit = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (autoTimer.current) { window.clearTimeout(autoTimer.current); autoTimer.current = null; }
    setSubmitError(false);
    setSubmitting(true);
    try {
      await submitFeedback({
        review_id: reviewId,
        ai_comment_id: aiCommentId,
        vote_type: (popupSource || vote || 'down') as 'up' | 'down',
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

  const openLinkedin = () => {
    setLinkedinText(buildLinkedinText(impactStats));
    setLinkedinOpen(true);
    setLinkedinOpacity(0);
    requestAnimationFrame(() => requestAnimationFrame(() => setLinkedinOpacity(1)));
  };

  const closeLinkedin = () => {
    setLinkedinOpacity(0);
    setTimeout(() => setLinkedinOpen(false), 200);
  };

  const handleCopyLinkedin = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(linkedinText);
      setSnackbar(true);
      if (snackTimer.current) window.clearTimeout(snackTimer.current);
      snackTimer.current = window.setTimeout(() => setSnackbar(false), 2200);
    } catch {}
  };

  const dimClass = size === 'sm' ? 'h-7 w-7' : 'h-8 w-8';

  if (denied) {
    return <span className="text-[11px] text-slate-600" title="Only the review's creator can leave feedback">Feedback unavailable</span>;
  }

  const ImpactLink = () =>
    statsExpanded ? (
      <div className="flex items-center gap-1.5 py-1 text-xs text-slate-400 select-none">
        <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="text-blue-400">
          <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
        </svg>
        Want to see your impact stats?
      </div>
    ) : (
      <div
        className="flex items-center gap-1.5 py-1 text-xs font-medium text-blue-400 select-none cursor-pointer hover:text-blue-300 transition-colors"
        onMouseEnter={() => setStatsExpanded(true)}
      >
        <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
        </svg>
        Want to see your impact stats?
        <svg width={11} height={11} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <polyline points="9 18 15 12 9 6" />
        </svg>
      </div>
    );

  const StatsGrid = () => {
    if (!impactStats) return <div className="py-2 text-xs text-slate-400">Loading stats…</div>;
    return (
      <div className="mt-2.5">
        <div className="grid grid-cols-4 gap-1.5 mb-2.5">
          {impactStats.map((s) => (
            <div
              key={s.label}
              title={s.tooltip}
              className="rounded-lg border border-slate-700 bg-slate-800/40 px-1.5 py-2 text-center cursor-default"
            >
              <div className="text-[17px] font-bold leading-tight text-blue-400">{s.value}</div>
              <div className="mt-1 text-[10px] leading-[1.3] text-slate-400">{s.label}</div>
            </div>
          ))}
        </div>
        <div className="border-t border-slate-700/60 pt-2">
          <div
            className="flex items-center gap-1.5 text-xs font-semibold text-blue-400 cursor-pointer hover:text-blue-300 transition-colors"
            onClick={openLinkedin}
          >
            <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <path d="M16 8a6 6 0 0 1 6 6v7h-4v-7a2 2 0 0 0-2-2 2 2 0 0 0-2 2v7h-4v-7a6 6 0 0 1 6-6z" />
              <rect x="2" y="9" width="4" height="12" />
              <circle cx="4" cy="4" r="2" />
            </svg>
            <span>Stand out by showing your impact stats to your peers</span>
            <svg width={12} height={12} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </div>
        </div>
      </div>
    );
  };

  return (
    <div className="relative inline-flex items-center gap-1.5 font-sans" ref={wrapperRef}>
      {/* Upvote button */}
      <button
        type="button"
        disabled={busy}
        onClick={() => cast('up')}
        onMouseEnter={handleLikeMouseEnter}
        onMouseLeave={handleMouseLeave}
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

      {/* Downvote button */}
      <button
        type="button"
        disabled={busy}
        onClick={() => cast('down')}
        onMouseEnter={handleDislikeMouseEnter}
        onMouseLeave={handleMouseLeave}
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

      {/* Feedback Popup Box */}
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
            {popupMode === 'hover' && popupSource === 'up' && (
              <div>
                <ImpactLink />
                {statsExpanded && <StatsGrid />}
              </div>
            )}
            
            {popupMode === 'submitted' && (
              <div className="flex items-center gap-2 py-1">
                <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="text-emerald-400">
                  <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                  <polyline points="9 11 12 14 22 4" />
                </svg>
                <span className="text-sm font-semibold text-slate-100">
                  {popupSource === 'up' ? 'Thanks for your detailed feedback!' : "Thanks. We'll work on making it better."}
                </span>
              </div>
            )}

            {popupMode === 'click' && (
              <div>
                <div className="mb-3 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    {popupSource === 'up' ? (
                      <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="text-emerald-400">
                        <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z" />
                        <path d="M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3" />
                      </svg>
                    ) : (
                      <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="text-red-400">
                        <path d="M10 15v4a3 3 0 0 0 3 3l4-9V2H5.72a2 2 0 0 0-2 1.7l-1.38 9a2 2 0 0 0 2 2.3H10z" />
                        <path d="M17 2h2.67A2.31 2.31 0 0 1 22 4v7a2.31 2.31 0 0 1-2.33 2H17" />
                      </svg>
                    )}
                    <span className="text-sm font-semibold text-slate-100">
                      {popupSource === 'up' ? 'Thanks for your feedback!' : "We're sorry it didn't meet your expectations!"}
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
                    {popupSource === 'up' ? 'What did you like about this review comment?' : 'What went wrong?'}
                  </div>
                  {popupSource === 'down' && (
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
                  )}
                </div>

                <textarea
                  value={feedbackText}
                  onChange={(e) => setFeedbackText(e.target.value)}
                  onClick={(e) => e.stopPropagation()}
                  placeholder={popupSource === 'up' ? 'Share your thoughts...' : 'Tell us more... (optional)'}
                  rows={2}
                  className="min-h-[64px] w-full resize-y rounded-md border border-slate-700 bg-slate-900/90 p-2.5 text-xs text-slate-200 outline-none placeholder:text-slate-500 focus:border-slate-500 focus:ring-1 focus:ring-slate-500"
                />

                <div className="mt-2 text-[11px] leading-relaxed text-slate-400">
                  This comment and the code block will be sent to Hexmos to continue improving quality.
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

                {popupSource === 'up' && (
                  <div className="mt-3 border-t border-slate-700/60 pt-3">
                    <ImpactLink />
                    {statsExpanded && <StatsGrid />}
                  </div>
                )}
              </div>
            )}
          </div>,
          document.body
        )}
        
      {/* LinkedIn Modal */}
      {linkedinOpen &&
        createPortal(
          <div
            className="fixed inset-0 z-[30000] flex items-center justify-center bg-black/70 backdrop-blur-sm transition-opacity duration-200"
            style={{ opacity: linkedinOpacity }}
            onClick={(e) => {
              if (e.target === e.currentTarget) closeLinkedin();
            }}
          >
            <div
              className="relative max-h-[calc(100vh-80px)] w-full max-w-[600px] overflow-y-auto rounded-2xl border border-slate-700/80 bg-[#151f2e] p-8 shadow-2xl"
              onClick={(e) => e.stopPropagation()}
            >
              <button
                type="button"
                onClick={closeLinkedin}
                title="Close (Esc)"
                className="absolute right-4 top-4 rounded-md border border-slate-700/60 bg-slate-800/40 p-1 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
              >
                <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                  <line x1="18" y1="6" x2="6" y2="18" />
                  <line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </button>
              
              <div className="mb-1 text-lg font-bold text-slate-100">
                Share your impact with your peers
              </div>
              <div className="mb-4 text-xs text-slate-400">
                Edit and post on LinkedIn to showcase your engineering impact
              </div>
              
              <textarea
                value={linkedinText}
                onChange={(e) => setLinkedinText(e.target.value)}
                onClick={(e) => e.stopPropagation()}
                className="min-h-[300px] w-full resize-y rounded-lg border border-slate-700/60 bg-slate-800/30 p-4 text-sm leading-relaxed text-slate-200 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              />
              
              <button
                type="button"
                onClick={handleCopyLinkedin}
                className={`mt-4 flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-semibold text-white transition-colors ${
                  snackbar ? 'bg-emerald-500 hover:bg-emerald-600' : 'bg-[#2d5be3] hover:bg-blue-600'
                }`}
              >
                {snackbar ? (
                  <>
                    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                      <path d="M20 6L9 17l-5-5" />
                    </svg>
                    Copied!
                  </>
                ) : (
                  <>
                    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                    </svg>
                    Copy to clipboard
                  </>
                )}
              </button>
            </div>
          </div>,
          document.body
        )}
        
      {/* Toast Notification */}
      {snackbar && !linkedinOpen &&
        createPortal(
          <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-[40000] flex animate-[fadeInUp_0.3s_ease] items-center gap-2 rounded-lg border border-emerald-500 bg-[#1e3a2f] px-5 py-2 text-sm font-medium text-emerald-400 shadow-lg pointer-events-none">
            <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <path d="M20 6L9 17l-5-5" />
            </svg>
            Copied to clipboard!
          </div>,
          document.body
        )}
    </div>
  );
};

export default VoteButtons;
