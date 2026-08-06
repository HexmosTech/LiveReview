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
  summary: string;
  hasQuiz: boolean;
  onTakeQuiz: () => void;
  onOpenFile?: OpenFileFromText;
}

const SummarySlideshow: React.FC<SummarySlideshowProps> = ({ summary, hasQuiz, onTakeQuiz, onOpenFile }) => {
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

  const isLast = currentSlide >= slides.length - 1;

  const goTo = (idx: number) => setCurrentSlide(clampSlideIndex(idx, slides.length - 1));
  const goNext = () => { if (!isLast) goTo(currentSlide + 1); else setIsAutoPlay(false); };
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

  // Autoplay: advance to the next slide after its own read-time elapses.
  useEffect(() => {
    if (!isAutoPlay || slides.length === 0) return;
    const slide = slides[currentSlide];
    const timer = window.setTimeout(() => {
      if (isLast) setIsAutoPlay(false);
      else goNext();
    }, (slide?.readTime || 5) * 1000);
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
  }, [currentSlide, slides.length, isLast]);

  if (slides.length === 0) {
    return <p className="text-sm text-slate-500">No summary was generated for this review.</p>;
  }

  const slide = slides[currentSlide];
  const typography = resolveSlideTypography(slide, isNarrow);
  const activeTrackItemKey = getActiveProgressTrackItemKey(trackItems, currentSlide);
  const activeTrackMarkerKey = getActiveProgressTrackMarkerKey(trackItems, currentSlide);
  const explorerCards = buildChapterExplorerCards(trackItems, currentSlide, activeTrackItemKey, activeTrackMarkerKey);
  const totalReadTime = calculateTotalReadTime(slides);
  const remaining = formatRemainingTime(slides, currentSlide);
  const elapsedActual = Math.max(1, Math.round((now - startTime) / 1000));

  return (
    <div ref={containerRef} className="relative">
      {!eligibility.eligible && (
        <p className="mb-2 text-[11px] text-slate-600">
          Auto-split by heading (this summary doesn't have the Overview / Technical Highlights / Impact structure git-lrc requires for its richer slide eligibility check).
        </p>
      )}

      {/* Chapter progress track — hovering/focusing it reveals the chapter
          explorer card grid below (git-lrc's openChapterExplorer/
          .summary-chapter-explorer): a per-chapter card with its own
          progress fill, slide count, "Starts at slide N" caption, and
          clickable subchapters, not just a sizing hint for the thin bar. */}
      <div className="group/track relative mb-4">
        <div className="flex h-2 gap-0.5 overflow-hidden rounded-full bg-slate-800 pr-8">
          {explorerCards.map((card) => (
            <button
              key={card.key}
              type="button"
              title={card.title}
              onClick={() => goTo(card.startIndex)}
              style={{ width: `${Math.max(2, (card.slideCount / slides.length) * 100)}%` }}
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
          {Math.round(((currentSlide + 1) / slides.length) * 100)}%
        </span>

        <div
          className={classNames(
            'absolute left-0 right-0 top-3 z-20 overflow-hidden rounded-lg border border-slate-700 bg-slate-900 shadow-xl',
            'transition-all duration-300 ease-in-out max-h-0 opacity-0 delay-500',
            'group-hover/track:max-h-[280px] group-hover/track:opacity-100 group-hover/track:delay-300',
            'group-focus-within/track:max-h-[280px] group-focus-within/track:opacity-100 group-focus-within/track:delay-300'
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
      <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-slate-500">
        {chapters.map((c) => (
          <button key={c.key} type="button" onClick={() => goTo(c.startIndex)} className={classNames('hover:text-slate-300', activeTrackItemKey === c.key && 'text-slate-200 font-medium')}>
            {c.title}
          </button>
        ))}
      </div>

      {/* Slide content — sized to feel like an actual presentation slide
          (git-lrc's is a near-viewport-height card), not a compact info box. */}
      <div
        className="flex min-h-[55vh] max-h-[640px] flex-col justify-center rounded-lg border p-10"
        style={{ background: slide.color.surface, borderColor: slide.color.accent + '80' }}
      >
        {slide.kind === 'intro' ? (
          <h2 style={{ fontSize: typography.fontSize, lineHeight: typography.lineHeight, maxWidth: typography.maxWidth, color: slide.color.title }} className="font-bold">
            {slide.title}
          </h2>
        ) : (
          <div>
            {slide.title && <p className="mb-2 text-xs font-medium uppercase tracking-wide" style={{ color: slide.color.accent }}>{slide.title}</p>}
            {slide.kind === 'file-point' && slide.meta?.kind === 'file-point' && (() => {
              const fileMeta = slide.meta;
              return onOpenFile ? (
                <button
                  type="button"
                  onClick={() => onOpenFile(fileMeta.filePath, fileMeta.line ?? undefined)}
                  title={`Open in diff: ${fileMeta.pathShort}`}
                  className="mb-2 inline-block rounded bg-black/30 px-2 py-1 font-mono text-xs underline decoration-dotted hover:brightness-125"
                  style={{ color: slide.color.accent }}
                >
                  {fileMeta.pathShort}
                </button>
              ) : (
                <code className="mb-2 inline-block rounded bg-black/30 px-2 py-1 font-mono text-xs" style={{ color: slide.color.accent }}>
                  {fileMeta.pathShort}
                </code>
              );
            })()}
            {slide.kind === 'label-point' && slide.meta?.kind === 'label-point' && (
              <p className="mb-1 text-xs font-semibold uppercase tracking-wide" style={{ color: slide.color.accent }}>{slide.meta.label}</p>
            )}
            {slide.kind === 'code' ? (
              <pre className="overflow-x-auto rounded-md bg-black/30 p-3 text-sm" style={{ color: slide.color.text }}><code>{slide.content}</code></pre>
            ) : (
              <p style={{ fontSize: typography.fontSize, lineHeight: typography.lineHeight, maxWidth: typography.maxWidth, color: slide.color.text }} className="font-medium">
                {renderInline(slide.content, `slide-${currentSlide}`, onOpenFile)}
              </p>
            )}
          </div>
        )}

        {isLast && hasQuiz && (
          <div className="mt-6 border-t border-white/10 pt-4">
            <Button variant="primary" onClick={onTakeQuiz}>Take the Quiz →</Button>
          </div>
        )}
      </div>

      {/* Controls */}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <button type="button" onClick={goPrev} disabled={currentSlide === 0} className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-30">‹ Prev</button>
          <button type="button" onClick={goNext} disabled={isLast} className="rounded-md border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-30">Next ›</button>
          <button
            type="button"
            onClick={() => setIsAutoPlay((v) => !v)}
            className={classNames('rounded-md border px-3 py-1.5 text-sm', isAutoPlay ? 'border-blue-600 bg-blue-900/30 text-blue-300' : 'border-slate-700 text-slate-300 hover:bg-slate-800')}
          >
            {isAutoPlay ? `Playing · ${elapsedActual}s` : 'Auto-play'}
          </button>
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
            ?
          </button>
        </div>
        <div className="flex items-center gap-3 text-xs text-slate-500">
          <span>{currentSlide + 1} / {slides.length}</span>
          <span title="Total estimated read time">{formatTime(totalReadTime)} total</span>
          <span title="Remaining estimated read time">{remaining} left</span>
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
