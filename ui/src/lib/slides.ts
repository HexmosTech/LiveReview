// Ported from git-lrc:internal/staticserve/static/components/SummarySlideshow/slideshowParser.js
// (as of the git-lrc HEAD current when this port was written) — deliberately a much
// smaller version. git-lrc's parser does auto-generated per-slide color themes,
// sentence-boundary-aware text splitting, word-count-based read-time estimates, and
// nested chapter/section tracking (~860 lines). This keeps the one part that actually
// matters for "review this as a deck instead of a wall of text": splitting the
// summary into slides at heading boundaries, in document order.
import { Block, parseBlocks } from './markdown';

export interface Slide {
  title?: string;
  blocks: Block[];
}

/**
 * Splits markdown into slides at each h1/h2 heading — content before the
 * first heading becomes an untitled intro slide. A summary with no
 * headings at all becomes a single slide.
 */
export function splitIntoSlides(markdown: string): Slide[] {
  const blocks = parseBlocks(markdown);
  const slides: Slide[] = [];
  let current: Slide | null = null;

  blocks.forEach((block) => {
    if (block.type === 'h1' || block.type === 'h2') {
      if (current) slides.push(current);
      current = { title: block.content, blocks: [] };
      return;
    }
    if (!current) current = { blocks: [] };
    current.blocks.push(block);
  });
  if (current) slides.push(current);

  return slides.filter((s) => s.title || s.blocks.length > 0);
}
