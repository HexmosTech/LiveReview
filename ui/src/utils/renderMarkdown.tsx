import React from 'react';

// Minimal, safe markdown-to-JSX renderer for AI-generated summaries (headings, bold, inline
// code, bullet lists, paragraphs). No dangerouslySetInnerHTML — everything goes through React
// elements, so there's no injection surface. Intentionally not a full CommonMark parser: it
// covers the subset the review-summary prompt actually produces.

const renderInline = (text: string, keyPrefix: string): React.ReactNode[] =>
  text
    .split(/(\*\*[^*]+\*\*|`[^`]+`)/g)
    .filter((part) => part.length > 0)
    .map((part, i) => {
      if (part.startsWith('**') && part.endsWith('**')) {
        return (
          <strong key={`${keyPrefix}-${i}`} className="font-semibold text-white">
            {part.slice(2, -2)}
          </strong>
        );
      }
      if (part.startsWith('`') && part.endsWith('`') && part.length > 1) {
        return (
          <code key={`${keyPrefix}-${i}`} className="rounded bg-slate-950 px-1 py-0.5 font-mono text-[0.85em] text-purple-300">
            {part.slice(1, -1)}
          </code>
        );
      }
      return <React.Fragment key={`${keyPrefix}-${i}`}>{part}</React.Fragment>;
    });

export const renderMarkdown = (markdown: string): React.ReactNode => {
  const lines = markdown.split('\n');
  const blocks: React.ReactNode[] = [];
  let paragraph: string[] = [];
  let listItems: string[] = [];

  const flushParagraph = (key: string) => {
    if (paragraph.length === 0) return;
    blocks.push(
      <p key={key} className="mb-3 text-sm leading-relaxed text-slate-300 last:mb-0">
        {renderInline(paragraph.join(' '), key)}
      </p>
    );
    paragraph = [];
  };

  const flushList = (key: string) => {
    if (listItems.length === 0) return;
    blocks.push(
      <ul key={key} className="mb-3 list-disc space-y-1.5 pl-5 text-sm leading-relaxed text-slate-300 last:mb-0">
        {listItems.map((item, i) => (
          <li key={`${key}-${i}`}>{renderInline(item, `${key}-${i}`)}</li>
        ))}
      </ul>
    );
    listItems = [];
  };

  lines.forEach((rawLine, idx) => {
    const trimmed = rawLine.trim();

    if (trimmed === '') {
      flushParagraph(`p-${idx}`);
      flushList(`l-${idx}`);
      return;
    }

    const h3 = /^###\s+(.*)/.exec(trimmed);
    const h2 = /^##\s+(.*)/.exec(trimmed);
    const h1 = /^#\s+(.*)/.exec(trimmed);
    const li = /^[-*]\s+(.*)/.exec(trimmed);

    if (h1) {
      flushParagraph(`p-${idx}`);
      flushList(`l-${idx}`);
      blocks.push(
        <div key={`h-${idx}`} className="mb-2 mt-1 text-base font-semibold text-white first:mt-0">
          {renderInline(h1[1], `h-${idx}`)}
        </div>
      );
    } else if (h2) {
      flushParagraph(`p-${idx}`);
      flushList(`l-${idx}`);
      blocks.push(
        <div key={`h-${idx}`} className="mb-2 mt-4 text-xs font-semibold uppercase tracking-wider text-purple-300 first:mt-0">
          {renderInline(h2[1], `h-${idx}`)}
        </div>
      );
    } else if (h3) {
      flushParagraph(`p-${idx}`);
      flushList(`l-${idx}`);
      blocks.push(
        <div key={`h-${idx}`} className="mb-1 mt-3 text-sm font-semibold text-slate-200 first:mt-0">
          {renderInline(h3[1], `h-${idx}`)}
        </div>
      );
    } else if (li) {
      flushParagraph(`p-${idx}`);
      listItems.push(li[1]);
    } else {
      flushList(`l-${idx}`);
      paragraph.push(trimmed);
    }
  });

  flushParagraph('p-end');
  flushList('l-end');

  return <>{blocks}</>;
};
