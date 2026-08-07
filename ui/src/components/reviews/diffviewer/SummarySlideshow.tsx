// Ported from git-lrc:internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js
// (as of the git-lrc HEAD current when this port was written): per-slide-kind
// typography sizing, the chapter/subchapter progress track, autoplay paced by
// read-time, keyboard shortcuts, and file-point/label-point slide layouts. Rendered
// inline within the Summary tab rather than as a full-viewport modal overlay (git-lrc
// supports both 'modal' and inline modes; LiveReview's page shell doesn't have a
// natural place for a takeover modal, so this uses the inline layout throughout) —
// markdown content itself renders via lib/markdown.tsx's block renderer instead of
// git-lrc's marked+DOMParser+sanitizeNode pipeline (no dangerouslySetInnerHTML
// anywhere in this port, so there's no sanitization step to replicate).
import React, { useEffect, useMemo, useRef, useState } from 'react';
import classNames from 'classnames';
import { Button } from '../../UIPrimitives';
import { OpenFileFromText, renderInline } from '../../../lib/markdown';
import VoteButtons from './VoteButtons';
import {
  calculateTotalReadTime,
  evaluateSummarySlidesEligibility,
  formatRemainingTime,
  formatTime,
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
        : { fontSize: 'clamp(38px, 4.6vw, 54px)', lineHeight: '1.18', maxWidth: 'min(100%, 640px)' };
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
}

const SummarySlideshow: React.FC<SummarySlideshowProps> = ({ reviewId, summary, hasQuiz, onTakeQuiz, onOpenFile }) => {
  const slides = useMemo(() => parseMarkdownToSlides(summary), [summary]);
  const eligibility = useMemo(() => evaluateSummarySlidesEligibility(summary), [summary]);
  const chapters = useMemo(() => buildChapterNavigation(slides), [slides]);
  const trackItems = useMemo(() => buildProgressTrackItems(chapters, slides.length), [chapters, slides.length]);

  const [currentSlide, setCurrentSlide] = useState(0);
  const [isAutoPlay, setIsAutoPlay] = useState(false);
  const [showHelp, setShowHelp] = useState(false);
  const [copied, setCopied] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const [startTime] = useState(() => Date.now());
  const autoplayTimerRef = useRef<number | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [isNarrow, setIsNarrow] = useState(typeof window !== 'undefined' && window.innerWidth <= 640);

  useEffect(() => setCurrentSlide(0), [summary]);

  useEffect(() => {
    const onResize = () => setIsNarrow(window.innerWidth <= 640);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  // git-lrc appends one virtual "complete" slide past the last real content
  // slide (currentSlide can reach slides.length, not just slides.length-1 —
  // SummarySlideshow.js:747-751's clampSlideIndex is called with
  // parsedSlides.length, not length-1) — it's what renders the celebration
  // screen, and it's the only thing that makes the "Complete" chapter-track
  // segment (buildProgressTrackItems' COMPLETE_TRACK_ITEM_KEY, already
  // ported in chapterNav.ts) ever actually reachable/fillable. Without it,
  // the progress readout could hit 100% on the last *content* slide while
  // the track's final segment stayed permanently unfilled, since its
  // startIndex (slides.length) was a position nothing could ever reach.
  const isCompleteSlide = currentSlide >= slides.length;
  const totalSlidesWithComplete = slides.length + 1;

  const goTo = (idx: number) => setCurrentSlide(clampSlideIndex(idx, slides.length));
  const goNext = () => { if (!isCompleteSlide) goTo(currentSlide + 1); else setIsAutoPlay(false); };
  const goPrev = () => goTo(currentSlide - 1);

  const copyCurrentSlide = () => {
    const slide = slides[currentSlide];
    if (!slide) return;
    const text = [slide.title, slide.content].filter(Boolean).join('\n\n');
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  };

  // Autoplay: advance to the next slide after its own read-time elapses;
  // stops on its own once it reaches the complete slide.
  useEffect(() => {
    if (!isAutoPlay || slides.length === 0) return;
    if (isCompleteSlide) { setIsAutoPlay(false); return; }
    const slide = slides[currentSlide];
    const timer = window.setTimeout(() => goNext(), (slide?.readTime || 5) * 1000);
    autoplayTimerRef.current = timer;
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAutoPlay, currentSlide, slides.length]);

  useEffect(() => {
    if (!isAutoPlay) return;
    const interval = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(interval);
  }, [isAutoPlay]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea') return;
      const shortcut = resolveSlideshowShortcut(e.key);
      if (!shortcut) return;
      e.preventDefault();
      switch (shortcut.type) {
        case 'next': goNext(); break;
        case 'prev': goPrev(); break;
        case 'jump': goTo(shortcut.slideIndex); break;
        case 'autoplay': setIsAutoPlay((v) => !v); break;
        case 'copy': copyCurrentSlide(); break;
        case 'help': setShowHelp((v) => !v); break;
        // 'close' (Escape/Q) only dismisses the help overlay here — git-lrc's
        // modal slideshow closes the whole takeover on this key, but this
        // component only ever renders inline (see the file header comment),
        // so there's no modal to close.
        case 'close': setShowHelp(false); break;
        default: break;
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentSlide, slides.length, isCompleteSlide]);

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
  const elapsedActual = Math.max(1, Math.round((now - startTime) / 1000));
  // git-lrc's formatActualElapsed/formatElapsed (SummarySlideshow.js:101-122)
  // — "actual" time spent vs. the "Planned" estimate from calculateTotalReadTime.
  const formatDuration = (totalSeconds: number): string => {
    if (totalSeconds < 60) return `${totalSeconds}s`;
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
  };
  const actualElapsed = formatDuration(Math.max(1, Math.round((Date.now() - startTime) / 1000)));
  const plannedElapsed = formatDuration(totalReadTime);
  // Ported 1:1 from git-lrc's progressValue (SummarySlideshow.js:750-752) —
  // reaches 100% only on the true complete slide, not one slide early on
  // the last *content* slide, which is what made the readout and the
  // "Complete" track segment's fill disagree.
  const progressValue = totalSlidesWithComplete
    ? ((Math.min(currentSlide, totalSlidesWithComplete - 1) + 1) / totalSlidesWithComplete) * 100
    : 0;

  return (
    <div ref={containerRef} className="relative">
      {!eligibility.eligible && (
        <p className="mb-2 text-[11px] text-slate-600">
          Auto-split by heading (this summary doesn't have the Overview / Technical Highlights / Impact structure git-lrc requires for its richer slide eligibility check).
        </p>
      )}

      {/* Slide content — sized to feel like an actual presentation slide
          (git-lrc's is a near-viewport-height card), not a compact info box. */}
      <div
        className={classNames(
          'flex min-h-[55vh] max-h-[640px] flex-col justify-center rounded-lg border p-10',
          isCompleteSlide && 'items-center text-center'
        )}
        style={isCompleteSlide ? { background: '#1f2430', borderColor: '#38455e' } : { background: slide!.color.surface, borderColor: slide!.color.accent + '80' }}
      >
        {isCompleteSlide ? (
          // Ported 1:1 from git-lrc's .summary-slideshow-complete block
          // (SummarySlideshow.js:963-1007) — the celebration screen shown
          // once every real content slide has been passed, not just a bare
          // "Take the Quiz" button tacked onto the last content slide.
          <div className="mx-auto max-w-[520px]">
            <svg viewBox="0 0 240 84" width={220} height={76} className="mx-auto" aria-hidden="true">
              <circle cx="32" cy="24" r="5" fill="#4f8cff" />
              <circle cx="58" cy="14" r="4" fill="#38b28a" />
              <circle cx="86" cy="28" r="4" fill="#f5a524" />
              <circle cx="152" cy="18" r="5" fill="#9a7bff" />
              <circle cx="188" cy="30" r="4" fill="#ff6b94" />
              <circle cx="212" cy="16" r="5" fill="#4f8cff" />
              <rect x="106" y="14" width="28" height="28" rx="14" fill="#233046" stroke="#7fb3ff" strokeWidth={2} />
              <path d="M112 28l6 6 10-12" fill="none" stroke="#9ed8ff" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            <div className="mb-2.5 mt-2 text-3xl font-bold tracking-tight text-slate-100">Review complete</div>
            <p className="mb-4 text-base text-slate-300">You finished all {slides.length} slides.</p>
            <p className="mb-4 text-sm text-slate-500">Your commitment to higher engineering standards made this review possible.</p>
            <div className="mb-1">
              <span className="text-2xl font-bold tracking-tight text-slate-100">{actualElapsed}</span>
              <span className="ml-2 text-sm text-slate-500">actual</span>
            </div>
            <p className="mb-5 text-xs text-slate-500">Planned: {plannedElapsed}</p>
            {hasQuiz && (
              <Button variant="primary" onClick={onTakeQuiz} title="Take the comprehension quiz for this review">Take the Quiz →</Button>
            )}
          </div>
        ) : slide!.kind === 'intro' ? (
          <h2 style={{ fontSize: typography!.fontSize, lineHeight: typography!.lineHeight, maxWidth: typography!.maxWidth, color: slide!.color.title }} className="font-bold">
            {slide!.title}
          </h2>
        ) : (
          <div className="w-full text-left">
            {slide!.title && <p className="mb-2 text-xs font-medium uppercase tracking-wide" style={{ color: slide!.color.accent }}>{slide!.title}</p>}
            {slide!.kind === 'file-point' && slide!.meta?.kind === 'file-point' && (() => {
              const fileMeta = slide!.meta;
              return onOpenFile ? (
                <button
                  type="button"
                  onClick={() => onOpenFile(fileMeta.filePath, fileMeta.line ?? undefined)}
                  title={`Open in diff: ${fileMeta.pathShort}`}
                  className="mb-2 inline-block rounded bg-black/30 px-2 py-1 font-mono text-xs underline decoration-dotted hover:brightness-125"
                  style={{ color: slide!.color.accent }}
                >
                  {fileMeta.pathShort}
                </button>
              ) : (
                <code className="mb-2 inline-block rounded bg-black/30 px-2 py-1 font-mono text-xs" style={{ color: slide!.color.accent }}>
                  {fileMeta.pathShort}
                </code>
              );
            })()}
            {slide!.kind === 'label-point' && slide!.meta?.kind === 'label-point' && (
              <p className="mb-1 text-xs font-semibold uppercase tracking-wide" style={{ color: slide!.color.accent }}>{slide!.meta.label}</p>
            )}
            {slide!.kind === 'code' ? (
              <pre className="overflow-x-auto rounded-md bg-black/30 p-3 text-sm" style={{ color: slide!.color.text }}><code>{slide!.content}</code></pre>
            ) : (
              <p style={{ fontSize: typography!.fontSize, lineHeight: typography!.lineHeight, maxWidth: typography!.maxWidth, color: slide!.color.text }} className="font-medium">
                {renderInline(slide!.content, `slide-${currentSlide}`, onOpenFile)}
              </p>
            )}
          </div>
        )}
      </div>

      {/* Progress track — directly below the slide, above the controls
          (standard scrubber placement, like a video player). Hovering/
          focusing it reveals the chapter explorer card grid (git-lrc's
          openChapterExplorer/.summary-chapter-explorer): a per-chapter card
          with its own progress fill, slide count, "Starts at slide N"
          caption, and clickable subchapters — not just a sizing hint for
          the thin bar.
          git-lrc's explorer is NOT an absolutely-positioned overlay
          (styles.css:1448-1459: no `position: absolute` anywhere on
          .summary-chapter-explorer, and its ancestors are explicitly
          `overflow: visible`) — it's a normal grid row that expands its own
          `max-height` on open, pushing whatever comes after it (the controls
          row) down. An absolute overlay here gets clipped by whatever
          container happens to sit above/behind it; growing in normal flow
          can't be clipped by anything. */}
      <div className="group/track relative mt-3">
        <div className="flex h-2 gap-0.5 overflow-hidden rounded-full bg-slate-800 pr-8">
          {explorerCards.map((card) => (
            <button
              key={card.key}
              type="button"
              title={card.title}
              onClick={() => goTo(card.startIndex)}
              style={{ width: `${Math.max(2, (card.slideCount / totalSlidesWithComplete) * 100)}%` }}
              className="group relative h-full overflow-hidden bg-slate-700"
            >
              <span
                className={classNames('absolute inset-y-0 left-0 block', card.isActive ? 'bg-blue-500' : 'bg-slate-500 group-hover:bg-slate-400')}
                style={{ width: `${card.progressPercent}%` }}
              />
            </button>
          ))}
        </div>
        <span className="absolute right-0 top-0 text-[10px] tabular-nums text-slate-500">
          {Math.round(progressValue)}%
        </span>

        <div
          className={classNames(
            'mt-3 overflow-hidden rounded-lg border border-slate-700 bg-slate-900 shadow-xl',
            'transition-all duration-300 ease-in-out max-h-0 opacity-0 -translate-y-2 delay-500',
            'group-hover/track:max-h-[320px] group-hover/track:opacity-100 group-hover/track:translate-y-0 group-hover/track:delay-300',
            'group-focus-within/track:max-h-[320px] group-focus-within/track:opacity-100 group-focus-within/track:translate-y-0 group-focus-within/track:delay-300'
          )}
        >
          <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-3 md:grid-cols-4">
            {explorerCards.map((card) => (
              <div
                key={card.key}
                className={classNames('rounded-md border p-2', card.isActive ? 'border-blue-600 bg-blue-950/30' : 'border-slate-700 bg-slate-800/60')}
              >
                <button type="button" onClick={() => goTo(card.startIndex)} className="block w-full text-left">
                  <div className="truncate text-xs font-medium text-slate-200">{card.title}</div>
                  <div className="text-[10px] text-slate-500">{card.slideCount} slide{card.slideCount !== 1 ? 's' : ''}</div>
                  <div className="mt-1 h-1 overflow-hidden rounded-full bg-slate-700">
                    <span className="block h-full bg-blue-500" style={{ width: `${card.progressPercent}%` }} />
                  </div>
                  <div className="mt-1 text-[10px] text-slate-600">
                    {card.kind === 'complete' ? 'Final slide' : `Starts at slide ${card.startIndex + 1}`}
                  </div>
                </button>
                {card.subchapters.length > 0 && (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {card.subchapters.map((sub) => (
                      <button
                        key={sub.key}
                        type="button"
                        title={sub.tooltipLabel}
                        onClick={() => goTo(sub.startIndex)}
                        className={classNames('rounded px-1.5 py-0.5 text-[10px]', sub.isActive ? 'bg-blue-600 text-white' : 'bg-slate-700 text-slate-300 hover:bg-slate-600')}
                      >
                        {sub.title}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Controls — three-column row (nav | position | actions), sitting
          below the progress track like a video player's button row. */}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <button type="button" onClick={goPrev} disabled={currentSlide === 0} className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-30">‹ Prev</button>
          <button type="button" onClick={goNext} disabled={isCompleteSlide} className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-30">Next ›</button>
          <button
            type="button"
            onClick={() => setIsAutoPlay((v) => !v)}
            className={classNames('rounded-md border px-3 py-1.5 text-sm', isAutoPlay ? 'border-blue-600 bg-blue-900/30 text-blue-300' : 'border-slate-700 text-slate-300 hover:bg-slate-800')}
          >
            {isAutoPlay ? `Playing · ${elapsedActual}s` : 'Auto-play'}
          </button>
        </div>
        <div className="flex items-center gap-3 text-xs text-slate-500">
          <span>
            {isCompleteSlide
              ? `${totalSlidesWithComplete} / ${totalSlidesWithComplete} · complete`
              : `${currentSlide + 1} / ${totalSlidesWithComplete} · ${remaining} left`}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <VoteButtons reviewId={reviewId} sourceType="slideshow" size="sm" />
          <button
            type="button"
            onClick={copyCurrentSlide}
            title="Copy current slide (C)"
            className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800"
          >
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button
            type="button"
            onClick={() => setShowHelp((v) => !v)}
            title="Keyboard shortcuts (?)"
            className={classNames('rounded-md border px-3 py-1.5 text-sm', showHelp ? 'border-blue-600 bg-blue-900/30 text-blue-300' : 'border-slate-700 text-slate-300 hover:bg-slate-800')}
          >
            ? Help
          </button>
        </div>
      </div>
      <p className="mt-1 text-[11px] text-slate-600">
        Shortcuts: ←/→/H/J/K/L/Space to navigate, 1–9 to jump, A to autoplay, C to copy, ? for help
      </p>

      {showHelp && (
        <div className="absolute bottom-14 left-0 z-10 w-72 rounded-lg border border-slate-700 bg-slate-900 p-4 shadow-xl">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Keyboard shortcuts</p>
          <dl className="space-y-1 text-sm text-slate-300">
            <div className="flex justify-between"><dt>Previous</dt><dd className="text-slate-500">← / H / K</dd></div>
            <div className="flex justify-between"><dt>Next</dt><dd className="text-slate-500">→ / L / J / Space</dd></div>
            <div className="flex justify-between"><dt>Jump</dt><dd className="text-slate-500">1-9</dd></div>
            <div className="flex justify-between"><dt>Auto-play</dt><dd className="text-slate-500">A</dd></div>
            <div className="flex justify-between"><dt>Copy</dt><dd className="text-slate-500">C</dd></div>
            <div className="flex justify-between"><dt>Hide help</dt><dd className="text-slate-500">Esc</dd></div>
          </dl>
        </div>
      )}
    </div>
  );
};

export default SummarySlideshow;
