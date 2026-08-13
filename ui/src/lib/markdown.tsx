// Lightweight, dependency-free markdown renderer for AI-generated review
// summaries (headers, bold/italic, inline code, code fences, lists, links).
// Renders directly to React elements (never dangerouslySetInnerHTML), so
// there's no sanitization step needed — untrusted text can never become
// markup. Not a general-purpose CommonMark implementation, just enough for
// the summary text LiveReview's AI actually produces.
import React from 'react';

export type OpenFileFromText = (filePath: string, line?: number) => void;

// Ported from git-lrc:internal/staticserve/static/components/Summary.js's
// enhanceTextWithFileChips/parseFullPathToken — any `path:line`-shaped code
// span or bold span in summary markdown becomes a clickable chip that jumps
// the diff viewer to that file. Kept narrow (must look like a real file path
// with an extension) so ordinary inline code like `npm install` never
// misfires.
const FILE_LINE_TOKEN = /^([\w.-]+(?:\/[\w.-]+)*\.\w+):(\d+)(?:-\d+)?$/;

function parseFileLineToken(text: string): { path: string; line: number } | null {
  const m = FILE_LINE_TOKEN.exec(text.trim());
  if (!m) return null;
  return { path: m[1], line: parseInt(m[2], 10) };
}

const FileChip: React.FC<{ token: { path: string; line: number }; onOpenFile: OpenFileFromText; bold?: boolean }> = ({ token, onOpenFile, bold }) => (
  <button
    type="button"
    onClick={() => onOpenFile(token.path, token.line)}
    title={`Open in diff: ${token.path}:${token.line}`}
    className={
      bold
        ? 'font-semibold text-sky-300 underline decoration-dotted decoration-sky-500/50 hover:text-sky-200'
        : 'rounded bg-slate-900 px-1 py-0.5 font-mono text-[0.85em] text-sky-300 underline decoration-dotted decoration-sky-500/50 hover:text-sky-200'
    }
  >
    {token.path}:{token.line}
  </button>
);

export function renderInline(text: string, keyPrefix: string, onOpenFile?: OpenFileFromText): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  // Order matters: code spans first (so ** inside `code` isn't touched),
  // then links, then bold, then italic.
  const pattern = /`([^`]+)`|\[([^\]]+)\]\(([^)]+)\)|\*\*([^*]+)\*\*|\*([^*]+)\*/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let i = 0;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }
    const key = `${keyPrefix}-${i++}`;
    if (match[1] !== undefined) {
      const token = onOpenFile ? parseFileLineToken(match[1]) : null;
      nodes.push(token
        ? <FileChip key={key} token={token} onOpenFile={onOpenFile!} />
        : <code key={key} className="rounded bg-slate-900 px-1 py-0.5 font-mono text-[0.85em] text-slate-200">{match[1]}</code>);
    } else if (match[2] !== undefined) {
      const href = match[3];
      const safe = /^(https?:|mailto:)/i.test(href);
      nodes.push(safe
        ? <a key={key} href={href} target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300 underline">{match[2]}</a>
        : match[2]);
    } else if (match[4] !== undefined) {
      const token = onOpenFile ? parseFileLineToken(match[4]) : null;
      nodes.push(token
        ? <FileChip key={key} token={token} onOpenFile={onOpenFile!} bold />
        : <strong key={key} className="font-semibold text-slate-100">{match[4]}</strong>);
    } else if (match[5] !== undefined) {
      nodes.push(<em key={key}>{match[5]}</em>);
    }
    lastIndex = pattern.lastIndex;
  }
  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
}

export interface Block {
  type: 'h1' | 'h2' | 'h3' | 'h4' | 'ul' | 'ol' | 'p' | 'code' | 'hr';
  content: string;
  items?: string[];
}

export function parseBlocks(markdown: string): Block[] {
  const lines = (markdown || '').replace(/\r\n/g, '\n').split('\n');
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    if (trimmed === '') {
      i++;
      continue;
    }

    if (trimmed.startsWith('```')) {
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trim().startsWith('```')) {
        codeLines.push(lines[i]);
        i++;
      }
      i++; // skip closing fence
      blocks.push({ type: 'code', content: codeLines.join('\n') });
      continue;
    }

    const headerMatch = trimmed.match(/^(#{1,4})\s+(.*)$/);
    if (headerMatch) {
      const level = headerMatch[1].length;
      blocks.push({ type: (`h${level}` as Block['type']), content: headerMatch[2] });
      i++;
      continue;
    }

    if (/^(-{3,}|\*{3,})$/.test(trimmed)) {
      blocks.push({ type: 'hr', content: '' });
      i++;
      continue;
    }

    if (/^[-*]\s+/.test(trimmed)) {
      const items: string[] = [];
      while (i < lines.length && /^[-*]\s+/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^[-*]\s+/, ''));
        i++;
      }
      blocks.push({ type: 'ul', content: '', items });
      continue;
    }

    if (/^\d+\.\s+/.test(trimmed)) {
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s+/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^\d+\.\s+/, ''));
        i++;
      }
      blocks.push({ type: 'ol', content: '', items });
      continue;
    }

    // Paragraph: collect consecutive non-blank, non-special lines.
    const paraLines: string[] = [];
    while (i < lines.length && lines[i].trim() !== '' && !/^(#{1,4})\s+/.test(lines[i].trim()) && !/^[-*]\s+/.test(lines[i].trim()) && !/^\d+\.\s+/.test(lines[i].trim()) && !lines[i].trim().startsWith('```')) {
      paraLines.push(lines[i].trim());
      i++;
    }
    blocks.push({ type: 'p', content: paraLines.join(' ') });
  }
  return blocks;
}

const HEADER_CLASSES: Record<string, string> = {
  h1: 'text-xl font-bold text-white mt-4 mb-2 first:mt-0',
  h2: 'text-lg font-semibold text-white mt-4 mb-2 first:mt-0',
  h3: 'text-base font-semibold text-slate-100 mt-3 mb-1.5 first:mt-0',
  h4: 'text-sm font-semibold text-slate-200 mt-3 mb-1 first:mt-0',
};

/** Renders a block list to React nodes — shared by Markdown (whole document)
 * and SummarySlideshow.tsx (one slide's worth of blocks at a time). */
export function renderBlocks(blocks: Block[], keyPrefix = 'b', onOpenFile?: OpenFileFromText): React.ReactNode[] {
  return blocks.map((block, idx) => {
    const key = `${keyPrefix}-${idx}`;
    switch (block.type) {
      case 'h1':
      case 'h2':
      case 'h3':
      case 'h4':
        return React.createElement(block.type, { key, className: HEADER_CLASSES[block.type] }, renderInline(block.content, key, onOpenFile));
      case 'hr':
        return <hr key={key} className="my-3 border-slate-700" />;
      case 'ul':
        return (
          <ul key={key} className="list-disc space-y-1 pl-5 my-2 text-sm text-slate-300">
            {(block.items || []).map((item, i2) => <li key={i2}>{renderInline(item, `${key}-${i2}`, onOpenFile)}</li>)}
          </ul>
        );
      case 'ol':
        return (
          <ol key={key} className="list-decimal space-y-1 pl-5 my-2 text-sm text-slate-300">
            {(block.items || []).map((item, i2) => <li key={i2}>{renderInline(item, `${key}-${i2}`, onOpenFile)}</li>)}
          </ol>
        );
      case 'code':
        return (
          <pre key={key} className="my-2 overflow-x-auto rounded-md bg-slate-950 p-3 text-xs text-slate-300">
            <code>{block.content}</code>
          </pre>
        );
      default:
        return <p key={key} className="my-2 text-sm leading-relaxed text-slate-300">{renderInline(block.content, key, onOpenFile)}</p>;
    }
  });
}

export const Markdown: React.FC<{ text: string; className?: string; onOpenFile?: OpenFileFromText }> = ({ text, className, onOpenFile }) => {
  const blocks = parseBlocks(text);
  return <div className={className}>{renderBlocks(blocks, 'b', onOpenFile)}</div>;
};
