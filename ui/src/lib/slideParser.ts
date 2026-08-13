// Ported from git-lrc:internal/staticserve/static/components/SummarySlideshow/slideshowParser.js
// (as of the git-lrc HEAD current when this port was written). git-lrc converts markdown to
// HTML via the `marked` library and walks the resulting DOM (DOMParser, Range, TreeWalker) to
// build slides; that round-trip is an implementation detail, not user-visible behavior, so this
// operates on parseBlocks()'s own Block[] AST instead — same slide-splitting rules (one
// intro slide from a leading H1, one slide per sentence within paragraphs, one slide per list
// item classified as file-point/label-point/plain, chapter tracking from H2/H3 headings, risk
// vs normal color cycling), without needing a markdown-to-HTML vendor dependency. Values
// (colors, read-time formula, required-section names) are copied verbatim.
import { Block, parseBlocks } from './markdown';

export const SLIDE_COLORS = [
  { surface: '#1f2733', accent: '#4f8cff', title: '#eaf1ff', text: '#d7e2ff', name: 'blue' },
  { surface: '#1f2c29', accent: '#38b28a', title: '#e8fff7', text: '#c8f5e8', name: 'mint' },
  { surface: '#26243a', accent: '#9a7bff', title: '#f0ecff', text: '#ddd4ff', name: 'violet' },
  { surface: '#33222c', accent: '#ff6b94', title: '#ffeaf2', text: '#ffd1e2', name: 'rose' },
  { surface: '#30271b', accent: '#f5a524', title: '#fff4de', text: '#ffe2b0', name: 'amber' },
];

export const RISK_SLIDE_COLORS = [
  { surface: '#331b24', accent: '#ff5d86', title: '#ffe9f0', text: '#ffd0df', name: 'risk-rose' },
  { surface: '#3a1d1d', accent: '#ff6b6b', title: '#ffeaea', text: '#ffd3d3', name: 'risk-red' },
  { surface: '#3b271c', accent: '#ff8f5a', title: '#fff0e7', text: '#ffd9c7', name: 'risk-amber-red' },
];

export type SlideColor = (typeof SLIDE_COLORS)[number];

const MIN_SLIDE_SECONDS = 5;
const MAX_SLIDE_SECONDS = 12;

const REQUIRED_SUMMARY_SECTIONS = ['overview', 'technical highlights', 'impact'];
const REQUIRED_SUMMARY_SECTION_ALIASES: Record<string, Set<string>> = {
  overview: new Set(['overview', 'summary']),
  'technical highlights': new Set(['technical highlights', 'highlights']),
  impact: new Set(['impact', 'risk', 'risks']),
};

function countWords(text: string): number {
  const trimmed = (text || '').trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
}

function estimateReadTimeSeconds(text: string, title?: string): number {
  const words = countWords(`${title || ''} ${text || ''}`);
  if (!words) return MIN_SLIDE_SECONDS;
  const estimated = Math.round(3.5 + words / 3.2);
  return Math.max(MIN_SLIDE_SECONDS, Math.min(MAX_SLIDE_SECONDS, estimated));
}

export function formatTime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return secs === 0 ? `${minutes}m` : `${minutes}m ${secs}s`;
}

function normalizeHeading(text: string): string {
  return (text || '').toLowerCase().replace(/[^a-z0-9\s]/g, ' ').replace(/\s+/g, ' ').trim();
}

function buildChapterKey(text: string): string {
  return normalizeHeading(text).replace(/\s+/g, '-');
}

export interface ChapterMeta {
  topLevelTitle: string;
  topLevelKey: string;
  nestedTitle: string;
  nestedKey: string;
  activeTitle: string;
}

interface ChapterContext {
  topLevelTitle: string;
  nestedTitle: string;
  activeTitle: string;
}

function createChapterMeta(context: ChapterContext): ChapterMeta | null {
  const { topLevelTitle, nestedTitle, activeTitle } = context;
  if (!topLevelTitle && !nestedTitle && !activeTitle) return null;
  const topLevelKey = buildChapterKey(topLevelTitle || activeTitle);
  const nestedKey = nestedTitle ? `${topLevelKey || 'chapter'}::${buildChapterKey(nestedTitle) || 'section'}` : '';
  return {
    topLevelTitle: topLevelTitle || activeTitle,
    topLevelKey,
    nestedTitle,
    nestedKey,
    activeTitle: activeTitle || topLevelTitle || nestedTitle,
  };
}

// Sentence segmentation using the same browser API git-lrc reaches for
// (Intl.Segmenter) — falls back to a punctuation-based split when
// unavailable (older browsers), mirroring slideshowParser.js's own fallback.
function splitIntoSentences(text: string): string[] {
  const trimmed = (text || '').trim();
  if (!trimmed) return [];
  if (typeof Intl !== 'undefined' && typeof (Intl as any).Segmenter === 'function') {
    const segmenter = new (Intl as any).Segmenter(undefined, { granularity: 'sentence' });
    const parts = Array.from(segmenter.segment(trimmed) as Iterable<{ segment: string }>)
      .map((s) => s.segment.trim())
      .filter(Boolean);
    return parts.length ? parts : [trimmed];
  }
  const parts = trimmed
    .split(/(?<=[.!?])\s+(?=(?:["'(\[])?[A-Z0-9])/)
    .map((p) => p.trim())
    .filter(Boolean);
  return parts.length ? parts : [trimmed];
}

export type SlideKind = 'intro' | 'sentence' | 'list' | 'file-point' | 'label-point' | 'code' | 'block';

export interface FilePointMeta {
  kind: 'file-point';
  filePath: string;
  line: number | null;
  pathShort: string;
  pathDir: string;
  description: string;
}

export interface LabelPointMeta {
  kind: 'label-point';
  label: string;
  body: string;
}

export interface Slide {
  title: string;
  content: string;
  kind: SlideKind;
  readTime: number;
  readTimeFormatted: string;
  color: SlideColor;
  meta: FilePointMeta | LabelPointMeta | null;
  chapter: ChapterMeta | null;
  slideNumber: number;
  totalSlides: number;
  totalReadTime: number;
}

function parsePathToken(pathToken: string): { filePath: string; line: number | null; pathShort: string; pathDir: string } | null {
  const trimmed = (pathToken || '').trim();
  const match = trimmed.match(/^(.*?)(?::(\d+))?$/);
  if (!match) return null;
  const filePath = (match[1] || '').trim();
  if (!filePath || !/\.[A-Za-z0-9]+$/.test(filePath)) return null;
  const line = match[2] ? Number(match[2]) : null;
  const baseName = filePath.split('/').pop() || filePath;
  const parentPath = filePath.includes('/') ? filePath.slice(0, filePath.lastIndexOf('/')) : '';
  return { filePath, line, pathShort: line ? `${baseName}:${line}` : baseName, pathDir: parentPath };
}

/** Classifies one list item's text as a "path/to/file.go:42 - description"
 * file point, a "Risk: ..." / "Impact: ..." labeled point, or plain text. */
function classifyListItem(text: string): FilePointMeta | LabelPointMeta | null {
  const normalized = text.replace(/^\s*[•*-]\s+/, '');

  const fileMatch = normalized.match(/^([A-Za-z0-9._/-]+(?:\.[A-Za-z0-9]+)?(?::\d+)?)\s*[:\-–]\s*(.+)$/);
  if (fileMatch) {
    const parsed = parsePathToken(fileMatch[1]);
    if (parsed) {
      return { kind: 'file-point', ...parsed, description: fileMatch[2].trim() };
    }
  }

  const labelMatch = normalized.match(/^(Functionality|Risk|Impact|Recommendation|Action)\s*:\s*(.+)$/i);
  if (labelMatch) {
    return { kind: 'label-point', label: labelMatch[1], body: labelMatch[2].trim() };
  }

  return null;
}

export function evaluateSummarySlidesEligibility(markdown: string): { eligible: boolean; reason: string; details?: string[] } {
  const raw = (markdown || '').trim();
  if (!raw) return { eligible: false, reason: 'empty-summary' };
  if (countWords(raw) < 20) return { eligible: false, reason: 'too-short' };

  const blocks = parseBlocks(raw);
  const sectionBodies = new Map<string, string>(REQUIRED_SUMMARY_SECTIONS.map((s) => [s, '']));
  const seen = new Set<string>();
  let active: string | null = null;

  function resolveRequiredSection(heading: string): string | null {
    const normalized = normalizeHeading(heading);
    if (!normalized) return null;
    for (const section of REQUIRED_SUMMARY_SECTIONS) {
      const aliases = REQUIRED_SUMMARY_SECTION_ALIASES[section] || new Set([section]);
      if (aliases.has(normalized)) return section;
    }
    return null;
  }

  blocks.forEach((block) => {
    if (block.type === 'h1' || block.type === 'h2' || block.type === 'h3' || block.type === 'h4') {
      active = resolveRequiredSection(block.content);
      if (active) seen.add(active);
      return;
    }
    if (!active) return;
    const text = block.items ? block.items.join(' ') : block.content;
    if (text) sectionBodies.set(active, `${sectionBodies.get(active)} ${text}`.trim());
  });

  const missing = REQUIRED_SUMMARY_SECTIONS.filter((s) => !seen.has(s));
  if (missing.length > 0) return { eligible: false, reason: 'missing-required-sections', details: missing };

  const empty = REQUIRED_SUMMARY_SECTIONS.filter((s) => countWords(sectionBodies.get(s) || '') < 3);
  if (empty.length > 0) return { eligible: false, reason: 'empty-required-sections', details: empty };

  return { eligible: true, reason: 'ok' };
}

/** Builds the slide deck from markdown — one intro slide from a leading H1,
 * chapters from H2/H3 headings, one slide per sentence in paragraphs, one
 * slide per classified list item. Mirrors parseMarkdownToSlides in
 * slideshowParser.js (see this file's header comment for what's adapted). */
export function parseMarkdownToSlides(markdown: string): Slide[] {
  if (!markdown || !markdown.trim()) return [];
  const blocks = parseBlocks(markdown);

  const slides: Omit<Slide, 'slideNumber' | 'totalSlides' | 'totalReadTime'>[] = [];
  let colorIndex = 0;
  let riskColorIndex = 0;
  let sectionTitle = '';
  let chapterContext: ChapterContext = { topLevelTitle: '', nestedTitle: '', activeTitle: '' };

  const nextColor = () => SLIDE_COLORS[colorIndex++ % SLIDE_COLORS.length];
  const nextRiskColor = () => RISK_SLIDE_COLORS[riskColorIndex++ % RISK_SLIDE_COLORS.length];

  const push = (content: string, color: SlideColor, kind: SlideKind, meta: FilePointMeta | LabelPointMeta | null = null) => {
    slides.push({
      title: sectionTitle,
      content,
      kind,
      readTime: estimateReadTimeSeconds(content, sectionTitle),
      readTimeFormatted: formatTime(estimateReadTimeSeconds(content, sectionTitle)),
      color,
      meta,
      chapter: createChapterMeta(chapterContext),
    });
  };

  blocks.forEach((block: Block, blockIndex: number) => {
    if (block.type === 'h1' && slides.length === 0 && blockIndex === 0) {
      slides.push({
        title: block.content,
        content: '',
        kind: 'intro',
        readTime: estimateReadTimeSeconds('', block.content),
        readTimeFormatted: formatTime(estimateReadTimeSeconds('', block.content)),
        color: nextColor(),
        meta: null,
        chapter: null,
      });
      sectionTitle = '';
      chapterContext = { topLevelTitle: '', nestedTitle: '', activeTitle: '' };
      return;
    }

    if (block.type === 'h1' || block.type === 'h2' || block.type === 'h3' || block.type === 'h4') {
      if (block.type === 'h2') {
        chapterContext = { topLevelTitle: block.content, nestedTitle: '', activeTitle: block.content };
      } else {
        chapterContext = { topLevelTitle: chapterContext.topLevelTitle || block.content, nestedTitle: block.content, activeTitle: block.content };
      }
      sectionTitle = block.content;
      return;
    }

    if (block.type === 'p') {
      splitIntoSentences(block.content).forEach((sentence) => push(sentence, nextColor(), 'sentence'));
      return;
    }

    if (block.type === 'ul' || block.type === 'ol') {
      (block.items || []).forEach((item) => {
        const structured = classifyListItem(item);
        if (!structured) {
          push(item, nextColor(), 'list');
          return;
        }
        if (structured.kind === 'file-point') {
          const color = nextColor();
          splitIntoSentences(structured.description).forEach((fragment) => push(fragment, color, 'file-point', structured));
          return;
        }
        const isRisk = structured.label.toLowerCase() === 'risk';
        const color = isRisk ? nextRiskColor() : nextColor();
        splitIntoSentences(structured.body).forEach((fragment) => push(fragment, color, 'label-point', structured));
      });
      return;
    }

    if (block.type === 'code') {
      push(block.content, nextColor(), 'code');
      return;
    }

    if (block.type === 'hr') {
      push('', nextColor(), 'block');
    }
  });

  const totalReadTime = slides.reduce((sum, s) => sum + s.readTime, 0);
  return slides.map((slide, index) => ({ ...slide, slideNumber: index + 1, totalSlides: slides.length, totalReadTime }));
}

export function calculateTotalReadTime(slides: Slide[]): number {
  return slides.reduce((sum, s) => sum + s.readTime, 0);
}

export function formatTotalReadTime(slides: Slide[]): string {
  return formatTime(calculateTotalReadTime(slides));
}

export function getRemainingReadTime(slides: Slide[], currentSlideIndex: number): number {
  if (!slides || currentSlideIndex >= slides.length) return 0;
  return slides.slice(currentSlideIndex).reduce((sum, s) => sum + s.readTime, 0);
}

export function formatRemainingTime(slides: Slide[], currentSlideIndex: number): string {
  return formatTime(getRemainingReadTime(slides, currentSlideIndex));
}
