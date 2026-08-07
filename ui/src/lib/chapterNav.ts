// Ported from git-lrc:internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js
// (buildChapterNavigation/buildProgressTrackItems/getActiveProgressTrackItemKey/
// getActiveProgressTrackMarkerKey/buildChapterExplorerCards/resolveSlideshowShortcut/
// clampSlideIndex, as of the git-lrc HEAD current when this port was written) — the
// chapter/subchapter progress-track data model behind the slideshow's top nav bar.
import { Slide } from './slideParser';

function normalizeLabel(text: string): string {
  return String(text || '').trim();
}

function buildFallbackChapterKey(text: string, index: number): string {
  const normalized = normalizeLabel(text)
    .toLowerCase()
    .replace(/[^a-z0-9\s]/g, ' ')
    .replace(/\s+/g, '-')
    .replace(/^-+|-+$/g, '');
  return normalized || `chapter-${index + 1}`;
}

export interface Subchapter {
  key: string;
  title: string;
  shortTitle: string;
  startIndex: number;
  endIndex: number;
  slideCount: number;
  offsetPct: number;
  widthPct: number;
  isSynthetic: boolean;
}

export interface Chapter {
  key: string;
  title: string;
  startIndex: number;
  endIndex: number;
  slideCount: number;
  widthPct: number;
  subchapters: Subchapter[];
}

export function buildChapterNavigation(slides: Slide[]): Chapter[] {
  if (!Array.isArray(slides) || slides.length === 0) return [];

  const chapters: Chapter[] = [];
  const firstExplicit = slides.find((s) => normalizeLabel(s.chapter?.topLevelTitle));
  let currentTopTitle = normalizeLabel(firstExplicit?.chapter?.topLevelTitle);
  let currentTopKey = normalizeLabel(firstExplicit?.chapter?.topLevelKey);
  let currentChapter: Chapter | null = null;
  let currentSubchapter: Subchapter | null = null;

  slides.forEach((slide, index) => {
    const explicitTopTitle = normalizeLabel(slide.chapter?.topLevelTitle);
    const explicitTopKey = normalizeLabel(slide.chapter?.topLevelKey);

    if (explicitTopTitle) {
      currentTopTitle = explicitTopTitle;
      currentTopKey = explicitTopKey || buildFallbackChapterKey(explicitTopTitle, index);
    }
    if (!currentTopTitle) {
      currentTopTitle = normalizeLabel(slide.title) || `Chapter ${chapters.length + 1}`;
      currentTopKey = buildFallbackChapterKey(currentTopTitle, index);
    }

    if (!currentChapter || currentChapter.key !== currentTopKey) {
      currentChapter = { key: currentTopKey, title: currentTopTitle, startIndex: index, endIndex: index, slideCount: 0, widthPct: 0, subchapters: [] };
      chapters.push(currentChapter);
      currentSubchapter = null;
    }

    currentChapter.endIndex = index;
    currentChapter.slideCount += 1;

    const nestedTitle = normalizeLabel(slide.chapter?.nestedTitle);
    const nestedKey = normalizeLabel(slide.chapter?.nestedKey);
    const chapterSlideOrdinal = index - currentChapter.startIndex + 1;

    if (!nestedTitle) {
      currentChapter.subchapters.push({
        key: `${currentTopKey}::slide-${chapterSlideOrdinal}`,
        title: `${currentChapter.title} ${chapterSlideOrdinal}`,
        shortTitle: `${chapterSlideOrdinal}`,
        startIndex: index,
        endIndex: index,
        slideCount: 1,
        offsetPct: 0,
        widthPct: 0,
        isSynthetic: true,
      });
      currentSubchapter = null;
      return;
    }

    if (!currentSubchapter || currentSubchapter.key !== nestedKey) {
      currentSubchapter = {
        key: nestedKey || `${currentTopKey}::section-${currentChapter.subchapters.length + 1}`,
        title: nestedTitle,
        shortTitle: nestedTitle,
        startIndex: index,
        endIndex: index,
        slideCount: 0,
        offsetPct: 0,
        widthPct: 0,
        isSynthetic: false,
      };
      currentChapter.subchapters.push(currentSubchapter);
    }
    currentSubchapter.endIndex = index;
    currentSubchapter.slideCount += 1;
  });

  chapters.forEach((chapter) => {
    chapter.widthPct = (chapter.slideCount / slides.length) * 100;
    chapter.subchapters.forEach((sub) => {
      sub.offsetPct = chapter.slideCount > 0 ? ((sub.startIndex - chapter.startIndex) / chapter.slideCount) * 100 : 0;
      sub.widthPct = chapter.slideCount > 0 ? (sub.slideCount / chapter.slideCount) * 100 : 0;
    });
  });

  return chapters;
}

const COMPLETE_TRACK_ITEM_KEY = 'complete';
const COMPLETE_TRACK_MARKER_KEY = 'complete::marker';
const COMPLETE_TRACK_TITLE = 'Complete';

export interface TrackSubchapter extends Subchapter {
  tooltipLabel: string;
  globalOffsetPct: number;
  markerVariant: 'default' | 'complete';
}

export interface TrackItem {
  key: string;
  kind: 'chapter' | 'complete';
  title: string;
  startIndex: number;
  endIndex: number;
  slideCount: number;
  unitCount: number;
  centerPct: number;
  subchapters: TrackSubchapter[];
}

export function buildProgressTrackItems(chapters: Chapter[], slideCount: number): TrackItem[] {
  const safeSlideCount = Number.isFinite(slideCount) ? Math.max(0, slideCount) : 0;
  const totalUnitCount = safeSlideCount + 1;

  const trackItems: TrackItem[] = (chapters || []).map((chapter) => ({
    key: chapter.key,
    kind: 'chapter',
    title: chapter.title,
    startIndex: chapter.startIndex,
    endIndex: chapter.endIndex,
    slideCount: chapter.slideCount,
    unitCount: chapter.slideCount,
    centerPct: totalUnitCount > 0 ? ((chapter.startIndex + chapter.slideCount / 2) / totalUnitCount) * 100 : 0,
    subchapters: chapter.subchapters.map((sub) => ({
      ...sub,
      tooltipLabel: sub.isSynthetic ? sub.title : `${chapter.title} -> ${sub.title}`,
      globalOffsetPct: totalUnitCount > 0 ? ((sub.startIndex + 0.5) / totalUnitCount) * 100 : 0,
      markerVariant: 'default' as const,
    })),
  }));

  trackItems.push({
    key: COMPLETE_TRACK_ITEM_KEY,
    kind: 'complete',
    title: COMPLETE_TRACK_TITLE,
    startIndex: safeSlideCount,
    endIndex: safeSlideCount,
    slideCount: 1,
    unitCount: 1,
    centerPct: totalUnitCount > 0 ? ((safeSlideCount + 0.5) / totalUnitCount) * 100 : 100,
    subchapters: [{
      key: COMPLETE_TRACK_MARKER_KEY,
      title: COMPLETE_TRACK_TITLE,
      shortTitle: COMPLETE_TRACK_TITLE,
      startIndex: safeSlideCount,
      endIndex: safeSlideCount,
      slideCount: 1,
      offsetPct: 0,
      widthPct: 100,
      isSynthetic: false,
      tooltipLabel: COMPLETE_TRACK_TITLE,
      globalOffsetPct: totalUnitCount > 0 ? (safeSlideCount / totalUnitCount) * 100 : 100,
      markerVariant: 'complete',
    }],
  });

  return trackItems;
}

export function getActiveProgressTrackItemKey(trackItems: TrackItem[], currentSlide: number): string {
  if (!trackItems.length) return '';
  const active = trackItems.find((t) => currentSlide >= t.startIndex && currentSlide <= t.endIndex);
  return active ? active.key : trackItems[0].key;
}

export function getActiveProgressTrackMarkerKey(trackItems: TrackItem[], currentSlide: number): string {
  for (const item of trackItems) {
    const marker = item.subchapters.find((s) => currentSlide >= s.startIndex && currentSlide <= s.endIndex);
    if (marker) return marker.key;
  }
  return '';
}

function getProgressTrackFillPercent(trackItem: TrackItem, currentSlide: number): number {
  if (!trackItem || trackItem.unitCount <= 0) return 0;
  if (currentSlide > trackItem.endIndex) return 100;
  if (currentSlide < trackItem.startIndex) return 0;
  const playedUnits = Math.max(0, Math.min(trackItem.unitCount, currentSlide - trackItem.startIndex + 1));
  return Math.max(0, Math.min(100, (playedUnits / trackItem.unitCount) * 100));
}

export interface ChapterExplorerCard {
  key: string;
  kind: 'chapter' | 'complete';
  title: string;
  slideCount: number;
  startIndex: number;
  progressPercent: number;
  isActive: boolean;
  subchapters: {
    key: string;
    title: string;
    tooltipLabel: string;
    startIndex: number;
    slideCount: number;
    isSynthetic: boolean;
    isActive: boolean;
  }[];
}

export function buildChapterExplorerCards(trackItems: TrackItem[], currentSlide: number, activeTrackItemKey = '', activeTrackMarkerKey = ''): ChapterExplorerCard[] {
  return trackItems.map((item) => ({
    key: item.key,
    kind: item.kind,
    title: item.title,
    slideCount: item.slideCount,
    startIndex: item.startIndex,
    progressPercent: getProgressTrackFillPercent(item, currentSlide),
    isActive: item.key === activeTrackItemKey,
    subchapters: item.kind === 'complete' ? [] : item.subchapters.map((sub) => ({
      key: sub.key,
      title: sub.title,
      tooltipLabel: sub.tooltipLabel,
      startIndex: sub.startIndex,
      slideCount: sub.slideCount,
      isSynthetic: sub.isSynthetic,
      isActive: sub.key === activeTrackMarkerKey,
    })),
  }));
}

export function clampSlideIndex(value: number, length: number): number {
  if (!Number.isFinite(value)) return 0;
  const maxIndex = Math.max(0, length);
  return Math.max(0, Math.min(Math.floor(value), maxIndex));
}

export type SlideshowShortcut =
  | { type: 'jump'; slideIndex: number }
  | { type: 'prev' | 'next' | 'autoplay' | 'copy' | 'help' | 'close' };

export function resolveSlideshowShortcut(key: string): SlideshowShortcut | null {
  const k = String(key || '').toLowerCase();
  if (/^[1-9]$/.test(k)) return { type: 'jump', slideIndex: parseInt(k, 10) - 1 };
  switch (k) {
    case 'arrowleft':
    case 'arrowup':
    case 'h':
    case 'k':
      return { type: 'prev' };
    case 'arrowright':
    case 'arrowdown':
    case 'l':
    case 'j':
    case ' ':
      return { type: 'next' };
    case 'a':
      return { type: 'autoplay' };
    case 'c':
      return { type: 'copy' };
    case '?':
      return { type: 'help' };
    case 'escape':
    case 'q':
      return { type: 'close' };
    default:
      return null;
  }
}
