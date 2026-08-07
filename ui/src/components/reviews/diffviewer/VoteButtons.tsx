// Ported from git-lrc:internal/staticserve/static/components/FeedbackPopup.js (as of
// the git-lrc HEAD current when this port was written) — full popup UX: impact stats,
// downvote reason tags, free-text feedback, LinkedIn share overlay. The vote itself
// calls LiveReview's real feedback API (internal/api/feedback_handler.go).
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { FeedbackSourceType, getImpactStats, ImpactStat, retractFeedback, submitFeedback } from '../../../api/feedback';

const DOWN_TAGS = ['False positive', 'Wrong severity', 'Missed something', 'Hard to act on'];

function buildLinkedinText(stats: ImpactStat[] | null): string {
  const v = (label: string) => { const s = (stats || []).find((x) => x.label === label); return s ? s.value : '-'; };
  return `Shipping with confidence — here's my code review impact since Jan 2025:

${v('Total Reviews')} reviews completed
${v('Bugs Caught Pre-Prod')} bugs caught before production
${v('Issues Found')} total issues found
${v('Critical')} critical issues found
${v('Errors')} errors caught
${v('Warnings')} warnings flagged

Using LiveReview to AI-review every commit before it lands.

#CodeReview #DevOps #SoftwareEngineering #AI`;
}

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
  reviewId, sourceType, aiCommentId, commentContent, codeExcerpt, filePath, severity, size = 'sm',
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
  const [statsExpanded, setStatsExpanded] = useState(false);
  const [impactStats, setImpactStats] = useState<ImpactStat[] | null>(null);
  const [linkedinOpen, setLinkedinOpen] = useState(false);
  const [snackbar, setSnackbar] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState(false);

  const autoTimer = useRef<number | null>(null);
  const hoverTimer = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (autoTimer.current) { clearTimeout(autoTimer.current); autoTimer.current = null; }
    if (hoverTimer.current) { clearTimeout(hoverTimer.current); hoverTimer.current = null; }
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  const isActive = vote === 'up' || vote === 'down';

  const postFeedback = useCallback((extra: Record<string, unknown> = {}) => {
    try {
      const body: Record<string, unknown> = {
        review_id: reviewId,
        vote_type: vote!,
        source_type: sourceType,
        tags: [...selectedTags],
        ...(commentContent && { comment_content: commentContent }),
        ...(filePath && { file_path: filePath }),
        ...(severity && { severity }),
        ...(codeExcerpt && { code_excerpt: codeExcerpt }),
        ...extra,
      };
      submitFeedback(body as any).then((res) => {
        if (res?.id) setFeedbackId(res.id);
      }).catch(() => {});
    } catch {}
  }, [reviewId, vote, sourceType, selectedTags, commentContent, filePath, severity, codeExcerpt]);

  const retract = useCallback(() => {
    if (feedbackId !== null) {
      retractFeedback(feedbackId).catch(() => {});
      setFeedbackId(null);
    }
  }, [feedbackId]);

  const show = useCallback((mode: PopupMode) => {
    setPopupVisible(true);
    setPopupMode(mode);
    setPopupAnim({ opacity: 0, shift: -6 });
  }, []);

  const hide = useCallback(() => {
    setPopupAnim({ opacity: 0, shift: -4 });
    window.setTimeout(() => {
      setPopupVisible(false);
      setPopupMode(null);
      setStatsExpanded(false);
    }, 280);
  }, []);

  const startAuto = useCallback((ms = 5000) => {
    if (autoTimer.current) clearTimeout(autoTimer.current);
    autoTimer.current = window.setTimeout(hide, ms);
  }, [hide]);

  useEffect(() => {
    if (!popupVisible || !popupRef.current || !wrapperRef.current) return;
    const r = wrapperRef.current.getBoundingClientRect();
    const w = 420;
    const left = Math.max(8, Math.min(r.right - w, window.innerWidth - w - 8));
    const belowTop = r.bottom + 8;
    const h = popupRef.current.offsetHeight;
    const top = h > 0 && belowTop + h > window.innerHeight ? Math.max(8, r.top - h - 8) : belowTop;
    setPopupPos({ top, left });
    if (popupAnim.opacity === 0) {
      requestAnimationFrame(() => requestAnimationFrame(() => {
        setPopupAnim({ opacity: 1, shift: 0 });
      }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [popupVisible, popupMode]);

  const cast = async (next: VoteState) => {
    if (busy) return;
    setBusy(true);
    setDenied(false);
    try {
      retract();
      if (next === vote) { setVote(null); if (popupVisible) hide(); return; }
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
        getImpactStats((stats) => setImpactStats(stats));
        show('click');
        startAuto();
      } else {
        show('click');
        startAuto();
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

  const handleMouseEnter = () => {
    if (hoverTimer.current) { clearTimeout(hoverTimer.current); hoverTimer.current = null; }
    if (popupMode === 'click' || popupMode === 'submitted') return;
    if (!popupVisible) {
      getImpactStats((stats) => setImpactStats(stats));
      show('hover');
    }
  };

  const handleMouseLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') return;
    hoverTimer.current = window.setTimeout(() => {
      if (popupMode === 'hover') hide();
    }, 80);
  };

  const onPopupEnter = () => {
    clearTimers();
  };

  const onPopupLeave = () => {
    if (popupMode === 'click' || popupMode === 'submitted') hide();
    else {
      hoverTimer.current = window.setTimeout(() => { if (popupMode === 'hover') hide(); }, 80);
    }
  };

  const handleSubmit = async (e: React.MouseEvent) => {
    e.stopPropagation();
    clearTimers();
    setSubmitError(false);
    setSubmitting(true);
    try {
      await submitFeedback({
        review_id: reviewId,
        vote_type: vote!,
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
  };

  const btnSize = size === 'sm' ? 'text-xs px-1.5 py-0.5' : 'text-sm px-2 py-1';

  if (denied) {
    return <span className="text-[11px] text-slate-600" title="Only the review's creator can leave feedback">Feedback unavailable</span>;
  }

  const popupWidth = 420;

  return (
    <div className="relative inline-flex items-center gap-1" ref={wrapperRef}>
      <button
        type="button" disabled={busy}
        onClick={() => cast('up')}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        title="This was helpful"
        className={`rounded border ${btnSize} ${
          vote === 'up'
            ? 'border-emerald-600 bg-emerald-900/40 text-emerald-300'
            : 'border-slate-700 text-slate-500 hover:text-slate-300'
        }`}
      >▲</button>
      <button
        type="button" disabled={busy}
        onClick={() => cast('down')}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        title="This wasn't helpful"
        className={`rounded border ${btnSize} ${
          vote === 'down'
            ? 'border-red-600 bg-red-900/40 text-red-300'
            : 'border-slate-700 text-slate-500 hover:text-slate-300'
        }`}
      >▼</button>

      {popupVisible && (
        <div
          ref={popupRef}
          className="fixed z-50 rounded-lg border border-slate-600 bg-slate-800 shadow-2xl"
          style={{
            top: popupPos.top, left: popupPos.left, width: popupWidth,
            opacity: popupAnim.opacity, transform: `translateY(${popupAnim.shift}px)`,
            transition: 'opacity 0.22s ease-out, transform 0.22s ease-out',
          }}
          onMouseEnter={onPopupEnter}
          onMouseLeave={onPopupLeave}
        >
          {vote === 'up' ? (
            // ── Upvote popup: impact stats + "like" text ──
            <div className="p-4">
              <p className="mb-3 text-sm font-medium text-slate-200">
                {popupMode === 'submitted' ? 'Thanks for your feedback!' : 'This was helpful?'}
              </p>
              {impactStats && (
                <div className="mb-3 rounded-md bg-slate-900/60 p-3">
                  <button
                    type="button"
                    onClick={() => setStatsExpanded((v) => !v)}
                    className="flex w-full items-center justify-between text-xs font-medium text-slate-400"
                  >
                    <span>Your Impact</span>
                    <span className="text-slate-500">{statsExpanded ? '▴' : '▾'}</span>
                  </button>
                  {statsExpanded && (
                    <div className="mt-2 grid grid-cols-2 gap-2">
                      {impactStats.map((s) => (
                        <div key={s.label} className="rounded bg-slate-800 px-2 py-1.5" title={s.tooltip}>
                          <div className="text-lg font-semibold text-white">{s.value ?? '-'}</div>
                          <div className="text-[10px] text-slate-500">{s.label}</div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
              {popupMode !== 'submitted' && (
                <div className="space-y-3">
                  <textarea
                    value={feedbackText}
                    onChange={(e) => setFeedbackText(e.target.value)}
                    placeholder="What did you like? (optional)"
                    rows={2}
                    className="w-full rounded-md border border-slate-600 bg-slate-900 px-3 py-2 text-xs text-slate-200 placeholder:text-slate-600"
                  />
                  <div className="flex items-center justify-between gap-2">
                    <button
                      type="button" onClick={() => setLinkedinOpen(true)}
                      className="rounded border border-sky-700 px-2 py-1 text-[11px] text-sky-400 hover:bg-sky-900/30"
                    >
                      Share on LinkedIn
                    </button>
                    <button
                      type="button" onClick={handleSubmit}
                      disabled={submitting}
                      className="rounded bg-emerald-700 px-3 py-1 text-xs font-medium text-white hover:bg-emerald-600 disabled:opacity-50"
                    >
                      {submitting ? 'Sending...' : 'Submit feedback'}
                    </button>
                  </div>
                  {submitError && <p className="text-[11px] text-red-400">Failed to submit. Try again.</p>}
                </div>
              )}
              {linkedinOpen && (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60" onClick={() => setLinkedinOpen(false)}>
                  <div className="max-h-[80vh] w-[520px] overflow-y-auto rounded-lg border border-slate-600 bg-slate-800 p-5 shadow-2xl" onClick={(e) => e.stopPropagation()}>
                    <h4 className="mb-3 text-sm font-medium text-white">Share on LinkedIn</h4>
                    <textarea
                      readOnly
                      value={buildLinkedinText(impactStats)}
                      className="mb-3 h-56 w-full resize-none rounded-md border border-slate-600 bg-slate-900 p-3 text-xs text-slate-200"
                    />
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => {
                          navigator.clipboard.writeText(buildLinkedinText(impactStats)).then(() => {
                            setSnackbar(true);
                            window.setTimeout(() => setSnackbar(false), 2000);
                          });
                        }}
                        className="rounded bg-slate-700 px-3 py-1.5 text-xs text-white hover:bg-slate-600"
                      >
                        {snackbar ? 'Copied!' : 'Copy to clipboard'}
                      </button>
                      <button type="button" onClick={() => setLinkedinOpen(false)} className="rounded border border-slate-600 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700">Close</button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          ) : (
            // ── Downvote popup: tag chips + feedback text ──
            <div className="p-4">
              <p className="mb-1 text-sm font-medium text-slate-200">
                {popupMode === 'submitted' ? 'Thanks for your feedback!' : 'We\'re sorry — what went wrong?'}
              </p>
              {popupMode !== 'submitted' && (
                <div className="space-y-3">
                  <div className="flex flex-wrap gap-1.5">
                    {DOWN_TAGS.map((tag) => {
                      const active = selectedTags.has(tag);
                      return (
                        <button
                          key={tag}
                          type="button"
                          onClick={() => setSelectedTags((prev) => {
                            const next = new Set(prev);
                            if (next.has(tag)) next.delete(tag);
                            else next.add(tag);
                            return next;
                          })}
                          className={`rounded-full border px-2.5 py-1 text-[11px] ${
                            active ? 'border-red-600 bg-red-900/40 text-red-300' : 'border-slate-600 text-slate-400 hover:bg-slate-700'
                          }`}
                        >
                          {tag}
                        </button>
                      );
                    })}
                  </div>
                  <textarea
                    value={feedbackText}
                    onChange={(e) => setFeedbackText(e.target.value)}
                    placeholder="Anything else? (optional)"
                    rows={2}
                    className="w-full rounded-md border border-slate-600 bg-slate-900 px-3 py-2 text-xs text-slate-200 placeholder:text-slate-600"
                  />
                  <div className="flex justify-end">
                    <button
                      type="button" onClick={handleSubmit}
                      disabled={submitting}
                      className="rounded bg-slate-700 px-3 py-1 text-xs font-medium text-white hover:bg-slate-600 disabled:opacity-50"
                    >
                      {submitting ? 'Sending...' : 'Submit feedback'}
                    </button>
                  </div>
                  {submitError && <p className="text-[11px] text-red-400">Failed to submit. Try again.</p>}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {snackbar && (
        <div className="fixed bottom-6 left-1/2 z-[70] -translate-x-1/2 rounded-full bg-slate-700 px-4 py-2 text-xs text-white shadow-lg">
          Copied to clipboard!
        </div>
      )}
    </div>
  );
};

export default VoteButtons;
