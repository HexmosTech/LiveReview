// Ported from git-lrc:internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js
// (as of HEAD): per-slide-kind typography sizing, embedded stage with floating navigation
// arrows (< and >), integrated bottom controls bar with autoplay countdown, counter, feedback,
// copy with location, help, and segmented chapter progress scrubber with notches and percentage readout.
import React, { useEffect, useMemo, useRef, useState } from 'react';
import classNames from 'classnames';
import { Button } from '../../UIPrimitives';
import { OpenFileFromText, renderInline } from '../../../lib/markdown';
import VoteButtons from './VoteButtons';
import {
  calculateTotalReadTime,
  evaluateSummarySlidesEligibility,
  formatRemainingTime,
  parseMarkdownToSlides,
  Slide,
} from '../../../lib/slideParser';
import {
  buildChapterExplorerCards,
  buildChapterNavigation,
  buildProgressTrackItems,
  clampSlideIndex,
  getActiveProgressTrackItemKey,
  getActiveProgressTrackMarkerKey,
  resolveSlideshowShortcut,
} from '../../../lib/chapterNav';

interface SlideTypography {
  fontSize: string;
  lineHeight: string;
  maxWidth: string;
}

function resolveSlideTypography(slide: Slide, isNarrow: boolean): SlideTypography {
  switch (slide.kind) {
    case 'intro':
      return isNarrow
        ? { fontSize: 'clamp(24px, 8vw, 34px)', lineHeight: '1.22', maxWidth: '100%' }
        : { fontSize: 'clamp(38px, 4.6vw, 54px)', lineHeight: '1.18', maxWidth: 'min(820px, 100%)' };
    case 'sentence':
    case 'file-point':
    case 'label-point':
      return isNarrow
        ? { fontSize: 'clamp(20px, 6.5vw, 28px)', lineHeight: '1.32', maxWidth: '100%' }
        : { fontSize: 'clamp(31px, 3.5vw, 46px)', lineHeight: '1.28', maxWidth: 'min(100%, 800px)' };
    case 'list':
      return isNarrow
        ? { fontSize: 'clamp(19px, 6vw, 26px)', lineHeight: '1.36', maxWidth: '100%' }
        : { fontSize: 'clamp(28px, 3.1vw, 40px)', lineHeight: '1.34', maxWidth: '100%' };
    case 'code':
      return isNarrow
        ? { fontSize: 'clamp(13px, 3.6vw, 17px)', lineHeight: '1.48', maxWidth: '100%' }
        : { fontSize: 'clamp(18px, 1.9vw, 24px)', lineHeight: '1.52', maxWidth: '100%' };
    default:
      return isNarrow
        ? { fontSize: 'clamp(16px, 5vw, 21px)', lineHeight: '1.4', maxWidth: '100%' }
        : { fontSize: 'clamp(22px, 2.3vw, 30px)', lineHeight: '1.46', maxWidth: '100%' };
  }
}

interface SummarySlideshowProps {
  reviewId: number;
  summary: string;
  hasQuiz: boolean;
  onTakeQuiz: () => void;
  onOpenFile?: OpenFileFromText;
  isModal?: boolean;
  onClose?: () => void;
  isShortcutActive?: boolean;
}

const SummarySlideshow: React.FC<SummarySlideshowProps> = ({
  reviewId,
  summary,
  hasQuiz,
  onTakeQuiz,
  onOpenFile,
  isModal = false,
  onClose,
  isShortcutActive = true,
}) => {
  const slides = useMemo(() => parseMarkdownToSlides(summary), [summary]);
  const eligibility = useMemo(() => evaluateSummarySlidesEligibility(summary), [summary]);
  const chapters = useMemo(() => buildChapterNavigation(slides), [slides]);
  const trackItems = useMemo(() => buildProgressTrackItems(chapters, slides.length), [chapters, slides.length]);

  const [currentSlide, setCurrentSlide] = useState(0);
  const [isAutoPlay, setIsAutoPlay] = useState(false);
  const [autoPlayRemainingMs, setAutoPlayRemainingMs] = useState(0);
  const [showHelp, setShowHelp] = useState(false);
  const [copied, setCopied] = useState(false);
  const [hoveredCardKey, setHoveredCardKey] = useState<string | null>(null);
  const [startTime] = useState(() => Date.now());

  const autoplayTimerRef = useRef<number | null>(null);
  const autoplayTickRef = useRef<number | null>(null);
  const copyTimerRef = useRef<number | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const [isNarrow, setIsNarrow] = useState(typeof window !== 'undefined' && window.innerWidth <= 640);

  useEffect(() => setCurrentSlide(0), [summary]);

  useEffect(() => {
    const onResize = () => setIsNarrow(window.innerWidth <= 640);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  // Reset vertical scroll position on slide change (matching git-lrc)
  useEffect(() => {
    if (bodyRef.current) {
      bodyRef.current.scrollTop = 0;
    }
  }, [currentSlide]);

  const isCompleteSlide = currentSlide >= slides.length;
  const totalSlidesWithComplete = slides.length + 1;

  const clearAutoPlayTimers = () => {
    if (autoplayTimerRef.current) {
      window.clearTimeout(autoplayTimerRef.current);
      autoplayTimerRef.current = null;
    }
    if (autoplayTickRef.current) {
      window.clearInterval(autoplayTickRef.current);
      autoplayTickRef.current = null;
    }
    setAutoPlayRemainingMs(0);
  };

  const goTo = (idx: number) => {
    clearAutoPlayTimers();
    setCurrentSlide(clampSlideIndex(idx, slides.length));
  };
  const goNext = () => {
    if (!isCompleteSlide) {
      goTo(currentSlide + 1);
    } else {
      clearAutoPlayTimers();
      setIsAutoPlay(false);
    }
  };
  const goPrev = () => {
    if (isCompleteSlide) {
      goTo(slides.length - 1);
    } else {
      goTo(Math.max(0, currentSlide - 1));
    }
  };

  const copyCurrentSlide = () => {
    const current = slides[currentSlide];
    if (!current) return;
    const copyParts: string[] = [];
    if (current.title) copyParts.push(current.title);
    if (current.meta?.kind === 'file-point') {
      const location = current.meta.line ? `${current.meta.filePath}:${current.meta.line}` : current.meta.filePath;
      copyParts.push(location);
    }
    if (current.content) copyParts.push(current.content);

    navigator.clipboard?.writeText(copyParts.join('\n\n')).then(() => {
      setCopied(true);
      if (copyTimerRef.current) {
        window.clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
      copyTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
    }).catch((err) => {
      console.error('Failed to copy slide text to clipboard:', err);
    });
  };

  // Autoplay timer advancement & countdown (matching git-lrc buildAutoplayLabel)
  useEffect(() => {
    clearAutoPlayTimers();
    if (!isAutoPlay || slides.length === 0) return;
    if (isCompleteSlide) {
      setIsAutoPlay(false);
      return;
    }

    const current = slides[currentSlide];
    const durationMs = (current?.readTime || 5) * 1000;
    const deadline = Date.now() + durationMs;
    setAutoPlayRemainingMs(durationMs);

    autoplayTickRef.current = window.setInterval(() => {
      setAutoPlayRemainingMs(Math.max(0, deadline - Date.now()));
    }, 200);

    autoplayTimerRef.current = window.setTimeout(() => {
      goNext();
    }, durationMs);

    return () => clearAutoPlayTimers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAutoPlay, currentSlide, slides.length, isCompleteSlide]);

  useEffect(() => {
    return () => {
      clearAutoPlayTimers();
      if (copyTimerRef.current) {
        window.clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  // Keyboard shortcut handler with hierarchical dismissal & element containment
  useEffect(() => {
    if (!isShortcutActive) return;
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable) return;
      const shortcut = resolveSlideshowShortcut(e.key);
      if (!shortcut) return;
      e.preventDefault();
      e.stopPropagation();
      switch (shortcut.type) {
        case 'next': goNext(); break;
        case 'prev': goPrev(); break;
        case 'jump': goTo(Math.max(0, Math.min(shortcut.slideIndex, Math.max(0, slides.length - 1)))); break;
        case 'autoplay': setIsAutoPlay((v) => !v); break;
        case 'copy': copyCurrentSlide(); break;
        case 'help': setShowHelp((v) => !v); break;
        case 'close':
          if (showHelp) {
            setShowHelp(false);
          } else if (isModal && onClose) {
            onClose();
          }
          break;
        default: break;
      }
    };
    document.addEventListener('keydown', handler, true);
    return () => document.removeEventListener('keydown', handler, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentSlide, slides.length, isCompleteSlide, showHelp, isModal, isShortcutActive, onClose]);

  if (slides.length === 0) {
    return <p className="text-sm text-slate-500">No summary was generated for this review.</p>;
  }

  const slide = !isCompleteSlide ? slides[currentSlide] : undefined;
  const typography = slide ? resolveSlideTypography(slide, isNarrow) : null;
  const activeTrackItemKey = getActiveProgressTrackItemKey(trackItems, currentSlide);
  const activeTrackMarkerKey = getActiveProgressTrackMarkerKey(trackItems, currentSlide);
  const explorerCards = buildChapterExplorerCards(trackItems, currentSlide, activeTrackItemKey, activeTrackMarkerKey);
  const totalReadTime = calculateTotalReadTime(slides);
  const remaining = formatRemainingTime(slides, currentSlide);

  const formatDuration = (totalSeconds: number): string => {
    if (totalSeconds < 60) return `${totalSeconds}s`;
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
  };
  const actualElapsed = formatDuration(Math.max(1, Math.round((Date.now() - startTime) / 1000)));
  const plannedElapsed = formatDuration(totalReadTime);
  const progressValue = totalSlidesWithComplete
    ? ((Math.min(currentSlide, totalSlidesWithComplete - 1) + 1) / totalSlidesWithComplete) * 100
    : 0;

  const progressAccent = slide ? slide.color.accent : (slides[slides.length - 1]?.color?.accent || '#3b82f6');
  const panelBg = isCompleteSlide ? '#1f2430' : (slide ? slide.color.surface : '#1f2430');
  const isIntro = !isCompleteSlide && slide?.kind === 'intro';

  // Exact git-lrc height clamping & aspect ratio
  const panelHeight = isCompleteSlide
    ? 'auto'
    : (isModal ? 'clamp(540px, 78vh, 760px)' : 'clamp(440px, 62vh, 620px)');
  const panelMaxHeight = isModal ? 'calc(100vh - 48px)' : (isCompleteSlide ? 'none' : '620px');

  const autoPlaySeconds = Math.max(1, Math.ceil(autoPlayRemainingMs / 1000));
  const autoPlayLabel = isAutoPlay ? `Playing · ${autoPlaySeconds}s` : 'Auto-play';

  return (
    <div
      ref={containerRef}
      className={classNames(
        'summary-slideshow-surface flex flex-col overflow-hidden relative border border-white/10 transition-colors duration-[220ms] ease-out',
        isModal
          ? 'w-full rounded-b-[14px] border-t-0 shadow-[0_28px_72px_rgba(0,0,0,0.4)]'
          : 'w-full rounded-[14px] shadow-[0_16px_40px_rgba(0,0,0,0.28),inset_0_1px_0_rgba(255,255,255,0.03)]'
      )}
      style={{
        background: panelBg,
        height: panelHeight,
        maxHeight: panelMaxHeight,
        aspectRatio: isModal ? '16 / 10' : undefined,
      }}
    >
      {!eligibility.eligible && (
        <p className="px-4 pt-2 text-[11px] text-slate-500">
          Auto-split by heading (this summary doesn't have the Overview / Technical Highlights / Impact structure git-lrc requires for its richer slide eligibility check).
        </p>
      )}

      {/* Slide stage: flex: 1, min-height: 0 with NO fixed min-height floor so it compresses cleanly upward when drawer opens */}
      <div className="summary-slideshow-stage relative flex flex-1 min-h-0 w-full group/stage">
        {/* Floating Previous slide overlay button */}
        <button
          type="button"
          onClick={goPrev}
          disabled={currentSlide === 0 && !isCompleteSlide}
          title="Previous slide (H / K / Left Arrow)"
          aria-label="Previous slide"
          tabIndex={-1}
          className="summary-slideshow-nav-overlay absolute left-4 top-1/2 -translate-y-1/2 z-10 w-11 h-11 flex items-center justify-center p-0 rounded-xl border border-slate-400/[0.22] bg-slate-900/[0.72] text-[#dce7fb] backdrop-blur-[8px] opacity-0 pointer-events-none group-hover/stage:opacity-[0.92] group-hover/stage:pointer-events-auto hover:!opacity-100 hover:!bg-[#1c2434]/[0.92] hover:!border-slate-400/[0.36] hover:!text-white transition-all disabled:!opacity-0 disabled:!pointer-events-none"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>

        {/* Slide Body */}
        <div
          ref={bodyRef}
          className={classNames(
            'summary-slideshow-body flex-1 min-h-0 flex flex-col justify-center px-6 py-7 md:px-8 overflow-y-auto',
            (isIntro || isCompleteSlide) ? 'items-center text-center' : 'items-start text-left'
          )}
        >
          {isCompleteSlide ? (
            <div className="summary-slideshow-complete mx-auto max-w-[520px] w-full text-center p-4 md:p-8">
              <div className="summary-slideshow-celebration mb-2.5 opacity-90 inline-flex items-center justify-center" aria-hidden="true">
                <svg viewBox="0 0 240 84" width={isNarrow ? 160 : 220} height={isNarrow ? 55 : 76}>
                  <circle cx="32" cy="24" r="5" fill="#4f8cff" />
                  <circle cx="58" cy="14" r="4" fill="#38b28a" />
                  <circle cx="86" cy="28" r="4" fill="#f5a524" />
                  <circle cx="152" cy="18" r="5" fill="#9a7bff" />
                  <circle cx="188" cy="30" r="4" fill="#ff6b94" />
                  <circle cx="212" cy="16" r="5" fill="#4f8cff" />
                  <rect x="106" y="14" width="28" height="28" rx="14" fill="#233046" stroke="#7fb3ff" strokeWidth={2} />
                  <path d="M112 28l6 6 10-12" fill="none" stroke="#9ed8ff" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </div>
              <div className="mb-2 text-2xl md:text-3xl font-bold tracking-tight text-[#cccccc]">Review complete</div>
              <p className="mb-3 text-[15px] md:text-[17px] text-[#d4d4d4] leading-normal">You finished all {slides.length} slides.</p>
              <p className="mb-4 text-xs md:text-sm text-[#858585] leading-normal">Your commitment to higher engineering standards made this review possible.</p>
              <div className="mb-1">
                <span className="text-xl md:text-2xl font-bold tracking-tight text-[#cccccc]">{actualElapsed}</span>
                <span className="ml-2 text-sm text-[#858585]">actual</span>
              </div>
              <p className="mb-5 text-xs text-[#858585]">Planned: {plannedElapsed}</p>
              <div className="flex flex-wrap items-center justify-center gap-2.5">
                {hasQuiz && (
                  <Button variant="primary" onClick={onTakeQuiz} title="Take the comprehension quiz for this review" className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="12" r="10" />
                      <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
                      <line x1="12" y1="17" x2="12.01" y2="17" />
                    </svg>
                    Take the Quiz
                  </Button>
                )}
                {isModal && onClose && (
                  <button
                    type="button"
                    onClick={onClose}
                    className="action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border border-[rgba(255,255,255,0.15)] bg-white/5 px-3.5 py-2 text-xs font-medium text-slate-200 hover:bg-white/10 hover:text-white transition-colors"
                  >
                    Back to Review
                  </button>
                )}
              </div>
            </div>
          ) : isIntro ? (
            /* Intro Slide 0: No artificial maxWidth constraint, spans full 820px parent centered */
            <div className="summary-slideshow-intro text-center max-w-[820px] w-full mx-auto break-words">
              <h1
                className="m-0 font-[760] text-center tracking-[-0.034em]"
                style={{
                  color: slide!.color.title,
                  fontSize: typography?.fontSize,
                  lineHeight: typography?.lineHeight,
                  textWrap: 'balance' as any,
                  textShadow: '0 1px 0 rgba(0, 0, 0, 0.25)',
                }}
              >
                {slide!.title}
              </h1>
            </div>
          ) : (
            <div className="w-full text-left" style={{ maxWidth: typography?.maxWidth }}>
              {slide!.title && (
                <div className="mb-1 text-sm font-bold tracking-[0.01em]" style={{ color: slide!.color.accent }}>
                  {slide!.title}
                </div>
              )}
              {slide!.kind === 'file-point' && slide!.meta?.kind === 'file-point' && (() => {
                const fileMeta = slide!.meta;
                const pathLabel = `${fileMeta.pathShort}${fileMeta.line ? `:${fileMeta.line}` : ''}`;
                return onOpenFile ? (
                  <button
                    type="button"
                    onClick={() => onOpenFile(fileMeta.filePath, fileMeta.line ?? undefined)}
                    title={`Open in diff: ${fileMeta.filePath}${fileMeta.line ? `:${fileMeta.line}` : ''}`}
                    className="mb-3 inline-flex items-center gap-1.5 rounded-full border border-[rgba(255,255,255,0.16)] bg-white/[0.035] px-3 py-[5px] text-[13px] font-semibold text-[#d8e6ff] hover:bg-[#4f8cff]/[0.11] hover:border-[#7fb3ff]/[0.42] hover:text-[#e4eeff] transition-all"
                  >
                    {pathLabel}
                  </button>
                ) : (
                  <code className="mb-3 inline-flex items-center gap-1.5 rounded-full border border-[rgba(255,255,255,0.16)] bg-white/[0.035] px-3 py-[5px] text-[13px] font-semibold text-[#d8e6ff]">
                    {pathLabel}
                  </code>
                );
              })()}
              {slide!.kind === 'label-point' && slide!.meta?.kind === 'label-point' && (
                <div className="mb-2 inline-block rounded-full border border-[rgba(255,255,255,0.16)] bg-white/10 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-slate-200">
                  {slide!.meta.label}
                </div>
              )}
              {slide!.kind === 'code' ? (
                <pre
                  className="overflow-x-auto rounded-lg bg-black/40 p-4 font-mono border border-white/10"
                  style={{ fontSize: typography?.fontSize, lineHeight: typography?.lineHeight, color: slide!.color.text }}
                >
                  <code>{slide!.content}</code>
                </pre>
              ) : (
                <div
                  style={{ fontSize: typography?.fontSize, lineHeight: typography?.lineHeight, color: slide!.color.text }}
                  className="font-medium tracking-[-0.01em]"
                >
                  {renderInline(slide!.content, `slide-${currentSlide}`, onOpenFile)}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Floating Next slide overlay button */}
        <button
          type="button"
          onClick={goNext}
          disabled={isCompleteSlide}
          title="Next slide (J / L / Right Arrow / Space)"
          aria-label="Next slide"
          tabIndex={-1}
          className="summary-slideshow-nav-overlay absolute right-4 top-1/2 -translate-y-1/2 z-10 w-11 h-11 flex items-center justify-center p-0 rounded-xl border border-slate-400/[0.22] bg-slate-900/[0.72] text-[#dce7fb] backdrop-blur-[8px] opacity-0 pointer-events-none group-hover/stage:opacity-[0.92] group-hover/stage:pointer-events-auto hover:!opacity-100 hover:!bg-[#1c2434]/[0.92] hover:!border-slate-400/[0.36] hover:!text-white transition-all disabled:!opacity-0 disabled:!pointer-events-none"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="9 18 15 12 9 6" />
          </svg>
        </button>
      </div>

      {/* Integrated bottom controls bar */}
      <div
        className="summary-slideshow-controls flex-shrink-0 px-4 pt-2.5 pb-3"
        style={{
          borderTop: '1px solid rgba(255, 255, 255, 0.12)',
          background: 'linear-gradient(180deg, rgba(17, 22, 29, 0.97), rgba(17, 22, 29, 0.94))',
        }}
      >
        <div className="summary-slideshow-controls-row relative flex items-center justify-between gap-3 mb-2">
          {/* Left button group: Prev, Next, Auto-play */}
          <div className="flex items-center gap-1.5 z-10">
            <button
              type="button"
              onClick={goPrev}
              disabled={currentSlide === 0 && !isCompleteSlide}
              title="Previous slide (H / K / Left Arrow)"
              aria-label="Previous slide"
              className="action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border border-[rgba(255,255,255,0.15)] bg-white/5 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-white/10 hover:text-white disabled:opacity-30 disabled:pointer-events-none transition-colors"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="15 18 9 12 15 6" />
              </svg>
              <span className={classNames('slide-btn-label', isNarrow && 'hidden')}>Prev</span>
            </button>
            <button
              type="button"
              onClick={goNext}
              disabled={isCompleteSlide}
              title="Next slide (J / L / Right Arrow / Space)"
              aria-label="Next slide"
              className="action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border border-[rgba(255,255,255,0.15)] bg-white/5 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-white/10 hover:text-white disabled:opacity-30 disabled:pointer-events-none transition-colors"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="9 18 15 12 9 6" />
              </svg>
              <span className={classNames('slide-btn-label', isNarrow && 'hidden')}>Next</span>
            </button>
            <button
              type="button"
              onClick={() => setIsAutoPlay((v) => !v)}
              title="Toggle auto-play (A)"
              aria-label="Toggle auto-play"
              className={classNames(
                'action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors',
                isAutoPlay
                  ? 'border-[rgba(79,195,247,0.5)] bg-[rgba(0,120,212,0.22)] text-[#eaf6ff]'
                  : 'border-[rgba(255,255,255,0.15)] bg-white/5 text-slate-200 hover:bg-white/10 hover:text-white'
              )}
            >
              {isAutoPlay ? (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="6" y="4" width="4" height="16" /><rect x="14" y="4" width="4" height="16" />
                </svg>
              ) : (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <polygon points="5 3 19 12 5 21 5 3" />
                </svg>
              )}
              <span className="slide-btn-label">{autoPlayLabel}</span>
            </button>
          </div>

          {/* Center counter mathematically locked at true 50% midpoint */}
          <div className="summary-slideshow-counter absolute left-1/2 -translate-x-1/2 pointer-events-none whitespace-nowrap text-[13px] font-medium text-[#aebad0] tracking-[0.01em]">
            {isCompleteSlide
              ? `${totalSlidesWithComplete}/${totalSlidesWithComplete} · complete`
              : `${currentSlide + 1}/${totalSlidesWithComplete} · ${remaining} left`}
          </div>

          {/* Right button group: Vote, Copy, Help */}
          <div className="flex items-center gap-1.5 z-10">
            <VoteButtons reviewId={reviewId} sourceType="slideshow" size="sm" />
            <button
              type="button"
              onClick={copyCurrentSlide}
              title="Copy current slide (C)"
              aria-label="Copy current slide"
              className={classNames(
                'action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-all',
                copied
                  ? 'border-[rgba(137,209,133,0.58)] bg-gradient-to-b from-[rgba(76,175,80,0.28)] to-[rgba(76,175,80,0.18)] text-[#efffec] shadow-[0_0_0_1px_rgba(137,209,133,0.12),0_0_18px_rgba(76,175,80,0.18)]'
                  : 'border-[rgba(255,255,255,0.15)] bg-white/5 text-slate-200 hover:bg-white/10 hover:text-white'
              )}
            >
              {copied ? (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              ) : (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
              )}
              <span className={classNames('slide-btn-label', isNarrow && 'hidden')}>Copied!</span>
            </button>
            <button
              type="button"
              onClick={() => setShowHelp((v) => !v)}
              title="Show keyboard shortcuts (?)"
              aria-label="Show keyboard shortcuts"
              className={classNames(
                'action-btn summary-slide-btn inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors',
                showHelp
                  ? 'border-blue-500/60 bg-blue-600/25 text-blue-200'
                  : 'border-[rgba(255,255,255,0.15)] bg-white/5 text-slate-200 hover:bg-white/10 hover:text-white'
              )}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
              <span className={classNames('slide-btn-label', isNarrow && 'hidden')}>Help</span>
            </button>
          </div>
        </div>

        {/* Chapter progress track zone */}
        <div className="summary-chapter-progress-wrap group/track relative mt-1">
          <div className="summary-chapter-progress-zone grid grid-cols-[1fr_auto] items-center gap-2.5 pt-1 pb-1">
            <div className="summary-chapter-progress-shell min-w-0">
              <div className="summary-chapter-progress flex items-center gap-[3px] relative pt-1.5 overflow-visible group/progress">
                {trackItems.map((trackItem) => {
                  const card = explorerCards.find((c) => c.key === trackItem.key);
                  const fillPercent = card ? card.progressPercent : 0;
                  const isActive = trackItem.key === activeTrackItemKey;
                  const trackItemLabel = `${trackItem.title} · ${trackItem.slideCount} ${trackItem.slideCount === 1 ? 'slide' : 'slides'}`;

                  return (
                    <div
                      key={trackItem.key}
                      className="summary-chapter-segment-wrap relative min-w-[18px] flex items-center"
                      style={{ flex: `${Math.max(1, trackItem.unitCount || 1)} 1 0` }}
                    >
                      <button
                        type="button"
                        onClick={() => goTo(trackItem.startIndex)}
                        title={trackItemLabel}
                        aria-label={`Jump to ${trackItemLabel}`}
                        className={classNames(
                          'summary-chapter-segment relative block w-full h-1 group-hover/progress:h-[7px] hover:!h-[9px] focus-visible:!h-[9px] rounded-full bg-slate-400/20 hover:!bg-slate-400/30 overflow-hidden cursor-pointer transition-all duration-200',
                          isActive && 'shadow-[0_0_0_1px_rgba(255,255,255,0.08)]'
                        )}
                      >
                        <span
                          className="summary-chapter-segment-fill absolute inset-y-0 left-0 rounded-full transition-all duration-280"
                          style={{
                            width: `${fillPercent}%`,
                            background: progressAccent,
                          }}
                        />
                      </button>

                      {/* Subchapter notches */}
                      {trackItem.subchapters.map((sub) => {
                        const isMarkerActive = sub.key === activeTrackMarkerKey;
                        const isCompleteMarker = sub.markerVariant === 'complete';
                        return (
                          <button
                            key={sub.key}
                            type="button"
                            onClick={() => goTo(sub.startIndex)}
                            title={sub.tooltipLabel}
                            aria-label={`Jump to ${sub.tooltipLabel}`}
                            className="summary-chapter-subsection-hit absolute top-1/2 -translate-y-1/2 -translate-x-1/2 z-[2] p-0 border-0 bg-transparent cursor-pointer w-3.5 h-5 flex items-center justify-center group/marker"
                            style={{ left: `${sub.offsetPct}%` }}
                          >
                            <span
                              className={classNames(
                                'summary-chapter-subsection-marker block rounded-full transition-all duration-200',
                                isCompleteMarker
                                  ? 'w-[4px] h-[16px] bg-[#f8fafc] shadow-[0_0_0_1px_rgba(15,23,42,0.4),0_0_10px_rgba(248,250,252,0.3)] group-hover/marker:h-[18px] group-hover/marker:bg-white'
                                  : 'w-[3px] h-[12px] bg-white/[0.78] shadow-[0_0_0_1px_rgba(15,23,42,0.4)] group-hover/marker:h-[15px] group-hover/marker:bg-white group-hover/marker:shadow-[0_0_0_1px_rgba(127,179,255,0.3)]',
                                isMarkerActive && (isCompleteMarker ? 'h-[18px] bg-white' : 'h-[15px] bg-white shadow-[0_0_0_1px_rgba(127,179,255,0.3)]')
                              )}
                            />
                          </button>
                        );
                      })}
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Readout percentage on right (e.g. 8%) */}
            <div className="summary-chapter-progress-readout text-[11px] font-bold text-[#8ea3c1] tracking-[0.02em] select-none pl-1 self-start pt-1.5">
              {Math.round(progressValue)}%
            </div>
          </div>

          {/* Chapter explorer expandable grid - fluid auto-fit, subchapters gated to active/hovered */}
          <div
            className={classNames(
              'overflow-hidden rounded-xl border border-slate-700/80 bg-slate-900/95 shadow-2xl backdrop-blur',
              'transition-all duration-300 ease-in-out max-h-0 opacity-0 -translate-y-2 pointer-events-none',
              'group-hover/track:max-h-[260px] group-hover/track:opacity-100 group-hover/track:translate-y-0 group-hover/track:pointer-events-auto group-hover/track:mt-2.5',
              'group-focus-within/track:max-h-[260px] group-focus-within/track:opacity-100 group-focus-within/track:translate-y-0 group-focus-within/track:pointer-events-auto group-focus-within/track:mt-2.5'
            )}
          >
            <div
              className="p-2.5"
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(152px, 1fr))',
                gap: '8px',
              }}
            >
              {explorerCards.map((card) => {
                const isEmphasized = hoveredCardKey === card.key;
                const showSubchapters = isEmphasized || card.isActive;
                return (
                  <div
                    key={card.key}
                    onMouseEnter={() => setHoveredCardKey(card.key)}
                    onMouseLeave={() => setHoveredCardKey(null)}
                    className={classNames(
                      'rounded-xl border p-2.5 transition-all duration-200 flex flex-col gap-2',
                      card.isActive
                        ? 'bg-[#1c2434]/90 border-[rgba(148,163,184,0.3)] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.03)]'
                        : 'bg-[#0f172a]/[0.58] border-[rgba(148,163,184,0.16)]',
                      isEmphasized && '-translate-y-0.5 shadow-lg border-[rgba(148,163,184,0.4)]'
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => goTo(card.startIndex)}
                      className="block w-full text-left p-0 border-0 bg-transparent cursor-pointer"
                    >
                      <div className="flex items-baseline justify-between gap-2">
                        <div className="truncate text-xs font-bold text-[#e7efff]">{card.title}</div>
                        <div className="text-[11px] text-[#97abc8] flex-shrink-0">{card.slideCount} {card.slideCount === 1 ? 'slide' : 'slides'}</div>
                      </div>
                      <div className="mt-1.5 h-[5px] overflow-hidden rounded-full bg-[rgba(148,163,184,0.16)]">
                        <span
                          className="block h-full rounded-full transition-all duration-280"
                          style={{
                            width: `${card.progressPercent}%`,
                            background: progressAccent,
                          }}
                        />
                      </div>
                      <div className="mt-1 text-[11px] text-[#90a3c0]">
                        {card.kind === 'complete' ? 'Final slide' : `Starts at slide ${card.startIndex + 1}`}
                      </div>
                    </button>

                    {showSubchapters && card.subchapters.length > 0 && (
                      <div className="flex flex-wrap gap-1.5 pt-0.5">
                        {card.subchapters.map((sub) => (
                          <button
                            key={sub.key}
                            type="button"
                            title={sub.tooltipLabel}
                            onClick={() => goTo(sub.startIndex)}
                            className={classNames(
                              'inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-[11px] font-semibold transition-colors',
                              sub.isActive
                                ? 'border-[rgba(148,163,184,0.4)] bg-white/10 text-white'
                                : 'border-[rgba(148,163,184,0.16)] bg-white/[0.03] text-[#dce7fb] hover:border-[rgba(148,163,184,0.36)] hover:bg-white/[0.06] hover:text-[#f5f9ff]'
                            )}
                          >
                            <span className="truncate max-w-[130px]">{sub.title}</span>
                            <span className="text-[10px] text-[#9fb3cf] font-bold">{sub.slideCount}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>

      {/* Keyboard shortcuts popup */}
      {showHelp && (
        <div className="absolute bottom-20 right-4 z-20 w-72 rounded-xl border border-slate-700/80 bg-slate-900/95 p-4 shadow-2xl backdrop-blur">
          <div className="flex items-center justify-between mb-2">
            <p className="text-xs font-semibold uppercase tracking-wide text-slate-300">Keyboard shortcuts</p>
            <button
              type="button"
              onClick={() => setShowHelp(false)}
              className="text-slate-400 hover:text-white"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
          <dl className="space-y-1 text-xs text-slate-300">
            <div className="flex justify-between"><dt>Previous</dt><dd className="text-slate-500">← / H / K</dd></div>
            <div className="flex justify-between"><dt>Next</dt><dd className="text-slate-500">→ / L / J / Space</dd></div>
            <div className="flex justify-between"><dt>Jump</dt><dd className="text-slate-500">1-9</dd></div>
            <div className="flex justify-between"><dt>Auto-play</dt><dd className="text-slate-500">A</dd></div>
            <div className="flex justify-between"><dt>Copy</dt><dd className="text-slate-500">C</dd></div>
            <div className="flex justify-between"><dt>{isModal ? 'Close' : 'Hide help'}</dt><dd className="text-slate-500">{isModal ? 'Q / Esc' : 'Esc'}</dd></div>
          </dl>
        </div>
      )}
    </div>
  );
};

export default SummarySlideshow;
