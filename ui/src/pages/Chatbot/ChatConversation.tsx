import React, { useState, useRef, useEffect, useCallback } from 'react';
import type { View } from 'vega';
import { sendChatMessage, ChatFile, ChatChart, ChartContext, SuggestedQuestionCategory } from '../../api/chatbot';
import { basePathForSurface, createConversation, getConversation, type ChatSurface } from '../../api/chatConversations';
import { BASE_URL, authFetch } from '../../api/apiClient';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
import { InteractiveChart, downloadChartView } from './InteractiveChart';
import { ThinkingIndicator } from './ThinkingIndicator';
import { CONVERSATIONS_QUERY_KEY } from './ConversationSidebar';
import { isDailyTrendChart, buildTrendSpec, formatAxisDate, Granularity } from './rebucketChart';
import { buildJsonExport, downloadJsonExport } from './jsonExport';
import ProductionUrlWarning from '../../components/ProductionUrlWarning';

const GRANULARITY_OPTIONS: { value: Granularity; label: string }[] = [
  { value: 'day', label: 'Day' },
  { value: 'week', label: 'Week' },
  { value: 'month', label: 'Month' },
];

const GranularityToggle: React.FC<{ value: Granularity; onChange: (g: Granularity) => void }> = ({
  value,
  onChange,
}) => (
  <div className="inline-flex rounded-md border border-slate-700 overflow-hidden text-xs">
    {GRANULARITY_OPTIONS.map((opt) => (
      <button
        key={opt.value}
        onClick={() => onChange(opt.value)}
        className={`px-2.5 py-1 font-medium transition-colors ${
          value === opt.value
            ? 'bg-indigo-600 text-white'
            : 'bg-slate-800/90 text-slate-300 hover:text-white hover:bg-slate-700'
        }`}
      >
        {opt.label}
      </button>
    ))}
  </div>
);

const StatChip: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="rounded-md border border-slate-700 bg-slate-800/60 px-2.5 py-1.5 min-w-0">
    <div className="text-[11px] text-slate-500">{label}</div>
    <div className="text-xs text-slate-300 font-medium break-words">{value}</div>
  </div>
);

// Who/what a chart or export is scoped to, always shown as three bullet
// points (Organization/Repository/Person) rather than one flattened line.
const ContextDetails: React.FC<{ context: ChartContext }> = ({ context }) => (
  <div>
    <p className="not-italic font-medium text-slate-400">Context:</p>
    <div className="pl-3 space-y-1">
      <p><span className="not-italic font-medium text-slate-400">Organization:</span> {context.organization}</p>
      {context.repository && context.repository.length > 0 && (
        <p><span className="not-italic font-medium text-slate-400">Repository:</span> {context.repository.join(', ')}</p>
      )}
      {context.person && context.person.length > 0 && (
        <p><span className="not-italic font-medium text-slate-400">Person:</span> {context.person.join(', ')}</p>
      )}
    </div>
  </div>
);

// Debug artifacts (SQL, CSV, Vega spec, schema context, system prompt, raw
// LLM exchange) - present on every turn's response, but only surfaced in the
// UI when `surface === 'chat_debug'` (see DebugTrigger usage below).
interface DebugArtifacts {
  query: string;
  schema_context: string;
  system_prompt: string;
  llm_raw_response: string;
  full_request: string;
  interpretations: Array<{
    sql: string;
    chart_type: string;
    title: string;
    description: string;
    encoding?: Record<string, unknown>;
  }>;
  results: Array<{
    index: number;
    title: string;
    chart_type: string;
    sql: string;
    status: string;
    skip_reason?: string;
    row_count: number;
    stats?: string[];
    csv_data?: string;
    vega_spec?: string;
    retry_count?: number;
    retries?: Array<{
      attempt: number;
      error: string;
      repaired_sql?: string;
    }>;
  }>;
}

interface ChatEntry {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  suggestedQuestions?: SuggestedQuestionCategory[];
  debugArtifacts?: DebugArtifacts | null;
}

function formatRowCount(rows?: number): string {
  if (!rows || rows < 1) return 'CSV';
  return `CSV · ${rows.toLocaleString()} row${rows === 1 ? '' : 's'}`;
}

function generateId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

function resolveImageUrl(url: string): string {
  // Only allow relative, same-origin URLs. The backend always returns
  // relative /api/v1/chat/files/... paths for CSV exports; rejecting absolute
  // http(s) URLs prevents loading arbitrary external content from a
  // model-influenced URL.
  if (!url || !url.startsWith('/')) return '';
  return `${BASE_URL}${url}`;
}

function chartFileName(title?: string): string {
  const now = new Date();
  const dd = String(now.getDate()).padStart(2, '0');
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const yy = String(now.getFullYear()).slice(-2);
  const base = (title || 'livereview-chart').replace(/[\/\\]+/g, '_');
  return `${base}__LiveReview__${dd}${mm}${yy}.png`;
}


const DATA_DETAIL_LABELS = ['Query', 'Time range', 'Granularity', 'Context'];

function extractTrailingDataDetails(text: string): { body: string; details: { label: string; value: string }[] } {
  const lines = text.split('\n');
  const details: { label: string; value: string }[] = [];
  let end = lines.length;
  while (end > 0) {
    const line = lines[end - 1];
    const match = DATA_DETAIL_LABELS.find((label) => line.startsWith(`${label}: `));
    if (!match) break;
    details.unshift({ label: match, value: line.slice(match.length + 2) });
    end -= 1;
  }
  if (details.length === 0) {
    return { body: text, details: [] };
  }
  if (end > 0 && lines[end - 1].trim() === '') {
    end -= 1;
  }
  return { body: lines.slice(0, end).join('\n'), details };
}

function formatText(rawText: string): React.ReactNode[] {
  const { body: text, details } = extractTrailingDataDetails(rawText);
  const parts: React.ReactNode[] = [];
  const lines = text.split('\n');
  let inCodeBlock = false;
  let codeContent = '';
  let lineIdx = 0;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith('```')) {
      if (inCodeBlock) {
        parts.push(
          <pre key={`code-${lineIdx++}`} className="bg-slate-800/90 text-cyan-300 p-3.5 rounded-xl border border-slate-700/60 overflow-x-auto text-sm my-3 shadow-inner">
            {codeContent}
          </pre>
        );
        codeContent = '';
        inCodeBlock = false;
      } else {
        inCodeBlock = true;
      }
      continue;
    }
    if (inCodeBlock) {
      codeContent += (codeContent ? '\n' : '') + line;
      continue;
    }

    if (line.trim() === '') {
      parts.push(<div key={`empty-${lineIdx++}`} className="h-2" />);
      continue;
    }

    // Markdown headings
    if (line.startsWith('### ')) {
      parts.push(
        <h3 key={`h3-${lineIdx++}`} className="text-base font-semibold text-slate-100 mt-4 mb-1.5 flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-indigo-400 inline-block" />
          {formatLine(line.slice(4))}
        </h3>
      );
      continue;
    }
    if (line.startsWith('## ')) {
      parts.push(
        <h2 key={`h2-${lineIdx++}`} className="text-lg font-bold text-slate-100 mt-5 mb-2 border-b border-slate-700/60 pb-1.5">
          {formatLine(line.slice(3))}
        </h2>
      );
      continue;
    }
    if (line.startsWith('# ')) {
      parts.push(
        <h1 key={`h1-${lineIdx++}`} className="text-xl font-bold text-white mt-6 mb-3">
          {formatLine(line.slice(2))}
        </h1>
      );
      continue;
    }

    // Bullet list items (* or -)
    const trimmedLine = line.trim();
    if (trimmedLine.startsWith('* ') || trimmedLine.startsWith('- ')) {
      const content = trimmedLine.slice(2);
      parts.push(
        <div key={`list-${lineIdx++}`} className="flex items-start gap-2.5 my-1 pl-2 text-slate-200">
          <span className="text-indigo-400 select-none mt-1 text-xs">•</span>
          <div className="flex-1 leading-relaxed">{formatLine(content)}</div>
        </div>
      );
      continue;
    }

    // Numbered list items (1., 2., etc.)
    const numMatch = trimmedLine.match(/^(\d+)\.\s+(.*)$/);
    if (numMatch) {
      parts.push(
        <div key={`numlist-${lineIdx++}`} className="flex items-start gap-2.5 my-1 pl-2 text-slate-200">
          <span className="font-semibold text-indigo-400 select-none text-sm">{numMatch[1]}.</span>
          <div className="flex-1 leading-relaxed">{formatLine(numMatch[2])}</div>
        </div>
      );
      continue;
    }

    const formattedLine = formatLine(line);
    parts.push(<div key={`line-${lineIdx++}`} className="mb-1 leading-relaxed">{formattedLine}</div>);
  }

  if (inCodeBlock && codeContent) {
    parts.push(
      <pre key={`code-${lineIdx++}`} className="bg-slate-800/90 text-cyan-300 p-3.5 rounded-xl border border-slate-700/60 overflow-x-auto text-sm my-3 shadow-inner">
        {codeContent}
      </pre>
    );
  }

  if (details.length > 0) {
    parts.push(
      <details key="data-details" className="group mt-2">
        <summary className="text-xs text-slate-500 cursor-pointer hover:text-slate-400 select-none">
          Data details
        </summary>
        <div className="mt-1.5 space-y-1 text-xs text-slate-400 italic">
          {details.map((d) => (
            <p key={d.label}><span className="not-italic font-medium text-slate-400">{d.label}:</span> {d.value}</p>
          ))}
        </div>
      </details>
    );
  }

  return parts;
}

function isSafeUrl(url: string): boolean {
  try {
    const trimmed = url.trim().toLowerCase();
    if (trimmed.startsWith('/') || trimmed.startsWith('#')) return true;
    const parsed = new URL(url, window.location.origin);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' || parsed.protocol === 'mailto:';
  } catch {
    return false;
  }
}

function formatLine(line: string): React.ReactNode {
  const parts: React.ReactNode[] = [];
  let i = 0;
  let partIdx = 0;

  while (i < line.length) {
    // Markdown links: [label](url)
    if (line[i] === '[') {
      const labelEnd = line.indexOf(']', i + 1);
      if (labelEnd > i && line[labelEnd + 1] === '(') {
        const urlEnd = line.indexOf(')', labelEnd + 2);
        if (urlEnd > labelEnd + 1) {
          const label = line.slice(i + 1, labelEnd);
          const url = line.slice(labelEnd + 2, urlEnd);
          if (isSafeUrl(url)) {
            parts.push(
              <a
                key={`link-${partIdx++}`}
                href={url}
                target={url.startsWith('/') || url.startsWith('#') ? '_self' : '_blank'}
                rel="noopener noreferrer"
                className="text-indigo-400 hover:text-indigo-300 underline underline-offset-2 font-medium transition-colors"
              >
                {label}
              </a>
            );
          } else {
            parts.push(<span key={`text-${partIdx++}`}>{label}</span>);
          }
          i = urlEnd + 1;
          continue;
        }
      }
    }

    if (line[i] === '*' && i + 1 < line.length && line[i + 1] === '*') {
      const end = line.indexOf('**', i + 2);
      if (end >= 0) {
        parts.push(<strong key={`b-${partIdx++}`} className="font-semibold text-slate-100">{line.slice(i + 2, end)}</strong>);
        i = end + 2;
        continue;
      }
    }
    if (line[i] === '*' && line[i + 1] !== '*') {
      const end = line.indexOf('*', i + 1);
      if (end >= 0) {
        parts.push(<em key={`i-${partIdx++}`} className="italic text-slate-300">{line.slice(i + 1, end)}</em>);
        i = end + 1;
        continue;
      }
      i += 1;
      continue;
    }
    if (line[i] === '`') {
      const end = line.indexOf('`', i + 1);
      if (end >= 0) {
        parts.push(
          <code key={`c-${partIdx++}`} className="bg-slate-800 text-cyan-300 border border-slate-700/60 px-1.5 py-0.5 rounded text-xs font-mono">
            {line.slice(i + 1, end)}
          </code>
        );
        i = end + 1;
        continue;
      }
    }
    if (line[i] === '>' && (i === 0 || line[i - 1] === ' ')) {
      const rest = line.slice(i + 1).trim();
      parts.push(
        <blockquote key={`q-${partIdx++}`} className="border-l-2 border-indigo-400 pl-3 text-slate-300 italic my-1">
          {formatLine(rest)}
        </blockquote>
      );
      i = line.length;
      continue;
    }

    let segEnd = findNextSpecial(line, i);
    if (segEnd === i) segEnd = i + 1;
    parts.push(<span key={`t-${partIdx++}`}>{line.slice(i, segEnd)}</span>);
    i = segEnd;
  }

  return <>{parts}</>;
}

function findNextSpecial(line: string, from: number): number {
  for (let i = from; i < line.length; i++) {
    if (line[i] === '*' || line[i] === '`' || line[i] === '>' || line[i] === '[') return i;
  }
  return line.length;
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

function prettyJSON(raw: string): string {
  try {
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return raw;
  }
}

// Collapsible section with copy button, used inside the debug modal.
const CollapsibleSection: React.FC<{
  id: string;
  label: string;
  content: string;
  prefix?: React.ReactNode;
  isGreen?: boolean;
  maxH?: string;
  activeSection: string | null;
  toggleSection: (id: string) => void;
  copiedId: string | null;
  handleCopy: (id: string, text: string) => void;
}> = ({ id, label, content, prefix, isGreen, maxH = 'max-h-48', activeSection, toggleSection, copiedId, handleCopy }) => {
  const isOpen = activeSection === 'all' || activeSection === id;
  return (
    <div className="border border-slate-700/50 rounded-md overflow-hidden">
      <button
        onClick={() => toggleSection(id)}
        className="flex items-center gap-2 w-full px-3 py-1.5 bg-slate-800/40 hover:bg-slate-800/60 text-xs text-slate-300 transition-colors"
      >
        <span className="text-slate-500 w-3">{isOpen ? '▼' : '▶'}</span>
        <span className="font-medium">{label}</span>
        <span className="ml-auto text-slate-600 text-[10px] font-mono">{content.length.toLocaleString()} chars</span>
        <button
          onClick={(e) => { e.stopPropagation(); handleCopy(id, content); }}
          className="ml-1 px-1.5 py-0.5 rounded bg-slate-700/50 hover:bg-slate-600/50 text-slate-400 hover:text-slate-200 text-[10px] transition-colors"
        >
          {copiedId === id ? 'Copied' : 'Copy'}
        </button>
      </button>
      {isOpen && (
        <>
          {prefix}
          <pre className={`p-3 text-[11px] leading-relaxed ${maxH} overflow-auto whitespace-pre-wrap border-t border-slate-700/30 ${
            isGreen ? 'bg-slate-900 text-green-300' : 'bg-slate-950 text-slate-300'
          }`}>
            {content}
          </pre>
        </>
      )}
    </div>
  );
};

// Compact inline button (chat_debug surface only) that opens the full debug
// info in a dialog, so the message stays visually identical to /chat instead
// of growing a wall of raw SQL/JSON beneath it.
const DebugTrigger: React.FC<{ artifacts: DebugArtifacts; onOpen: () => void }> = ({ artifacts, onOpen }) => {
  const rendered = artifacts.results?.filter((r) => r.status === 'rendered').length || 0;
  const skipped = artifacts.results?.filter((r) => r.status === 'skipped').length || 0;
  const failed = artifacts.results?.filter((r) => r.status === 'failed').length || 0;

  return (
    <button
      onClick={onOpen}
      className="mt-3 inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-amber-500/20 bg-amber-500/5 hover:bg-amber-500/10 text-left transition-colors"
    >
      <span className="text-amber-300 text-[11px] font-medium tracking-wide uppercase">Debug Artifacts</span>
      <span className="flex items-center gap-2 text-[10px] font-mono">
        {rendered > 0 && <span className="text-green-400/80">{rendered} ok</span>}
        {skipped > 0 && <span className="text-yellow-400/80">{skipped} skip</span>}
        {failed > 0 && <span className="text-red-400/80">{failed} fail</span>}
      </span>
    </button>
  );
};

// Full debug artifacts content, shown inside DebugModal below.
const DebugPanelBody: React.FC<{ artifacts: DebugArtifacts }> = ({ artifacts }) => {
  const [expandAll, setExpandAll] = useState(false);
  const [activeSection, setActiveSection] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  // Auto-expand on failure — only on initial mount
  const hasFailure = artifacts.results?.some((r) => r.status === 'failed');
  const didAutoExpand = useRef(false);
  useEffect(() => {
    if (hasFailure && !didAutoExpand.current) {
      didAutoExpand.current = true;
      setActiveSection('all');
      setExpandAll(true);
    }
  }, [hasFailure]);

  const toggleSection = (id: string) => {
    setActiveSection(activeSection === id ? null : id);
  };

  const toggleExpandAll = () => {
    if (expandAll) {
      setActiveSection(null);
      setExpandAll(false);
    } else {
      setExpandAll(true);
      setActiveSection('all');
    }
  };

  const handleCopy = async (id: string, text: string) => {
    const ok = await copyToClipboard(text);
    if (ok) {
      setCopiedId(id);
      setTimeout(() => setCopiedId(null), 1500);
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] text-slate-500 font-mono">{artifacts.interpretations?.length || 0} interpretations</span>
        <button
          onClick={toggleExpandAll}
          className="text-[10px] text-slate-500 hover:text-slate-300 transition-colors"
        >
          {expandAll ? 'Collapse All' : 'Expand All'}
        </button>
      </div>

      <div className="space-y-2">
        {artifacts.interpretations?.map((interp, i) => {
          const result = artifacts.results?.[i];
          const statusColor = result?.status === 'rendered' ? 'bg-green-400'
            : result?.status === 'skipped' ? 'bg-yellow-400' : 'bg-red-400';
          const hasError = result?.status === 'failed' || !!result?.skip_reason;
          const errorPrefix = hasError ? (
            <div className="px-3 py-2 bg-red-500/10 border-b border-red-500/20 text-[11px]">
              <span className="text-red-400 font-medium">Status: {result?.status}</span>
              {result?.skip_reason && (
                <span className="text-red-400/80 ml-2">— {result.skip_reason}</span>
              )}
            </div>
          ) : undefined;
          return (
            <div key={i} className={`rounded-md border overflow-hidden ${
              hasError ? 'border-red-500/30' : 'border-slate-700/40'
            }`}>
              <div className="flex items-center gap-2 px-3 py-2 bg-slate-800/50">
                <span className="text-[10px] font-mono text-slate-500 w-4">{i + 1}.</span>
                <span className={`w-2 h-2 rounded-full ${statusColor}`} />
                <span className="text-xs font-medium text-slate-200">{interp.title}</span>
                <span className="text-[10px] text-slate-500 font-mono">{interp.chart_type}</span>
                {result && result.retry_count != null && result.retry_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 font-mono">
                    {result.retry_count} retry{result.retry_count > 1 ? 's' : ''}
                  </span>
                )}
                {result && <span className="text-[10px] text-slate-600 ml-auto font-mono">{result.row_count} rows</span>}
              </div>
              <div className="px-3 py-2 text-[11px] text-slate-400 border-b border-slate-700/30">{interp.description}</div>
              <div className="px-3 py-2 space-y-1.5">
                {interp.sql && (
                  <CollapsibleSection id={`sql-${i}`} label="SQL Query" content={interp.sql} prefix={errorPrefix} isGreen
                    activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy} />
                )}
                {result?.csv_data && (
                  <CollapsibleSection id={`csv-${i}`} label="Query Result (CSV)" content={result.csv_data} isGreen maxH="max-h-40"
                    activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy} />
                )}
                {result?.vega_spec && (
                  <CollapsibleSection id={`vega-${i}`} label="Vega-Lite Spec" content={prettyJSON(result.vega_spec)} isGreen maxH="max-h-48"
                    activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy} />
                )}
                {result?.retries && result.retries.length > 0 && (
                  <CollapsibleSection
                    id={`retries-${i}`}
                    label={`Retry Details (${result.retries.length} attempt${result.retries.length > 1 ? 's' : ''})`}
                    content={result.retries.map((r) => {
                      let text = `Attempt ${r.attempt}: ${r.error}`;
                      if (r.repaired_sql) {
                        text += `\nRepaired SQL:\n${r.repaired_sql}`;
                      }
                      return text;
                    }).join('\n\n')}
                    activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy}
                  />
                )}
              </div>
            </div>
          );
        })}
      </div>

      <div className="border-t border-amber-500/10" />

      <div className="rounded-md border border-slate-700/40 bg-slate-800/30 overflow-hidden">
        <div className="px-3 py-2 bg-slate-800/50 text-[11px] font-medium text-slate-300 border-b border-slate-700/30">
          Pipeline Context
        </div>
        <div className="px-3 py-2 space-y-1.5">
          {artifacts.schema_context && (
            <CollapsibleSection id="schema" label="Schema Context (sent to LLM)" content={artifacts.schema_context}
              activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy} />
          )}
          {artifacts.system_prompt && (
            <CollapsibleSection id="prompt" label="System Prompt" content={artifacts.system_prompt}
              activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy} />
          )}
        </div>
      </div>

      <div className="rounded-md border border-slate-700/40 bg-slate-800/30 overflow-hidden">
        <div className="px-3 py-2 bg-slate-800/50 text-[11px] font-medium text-slate-300 border-b border-slate-700/30">
          LLM Exchange
        </div>
        <div className="px-3 py-2 space-y-1.5">
          <CollapsibleSection
            id="full-request"
            label="Full Request (system + schema + query)"
            content={artifacts.full_request || (artifacts.system_prompt + '\n---\n' + artifacts.schema_context)}
            activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy}
          />
          <CollapsibleSection
            id="raw"
            label="Raw Response"
            content={artifacts.llm_raw_response} isGreen maxH="max-h-56"
            activeSection={activeSection} toggleSection={toggleSection} copiedId={copiedId} handleCopy={handleCopy}
          />
        </div>
      </div>
    </div>
  );
};

// Dialog wrapper — all debug artifacts for one message, opened from DebugTrigger.
const DebugModal: React.FC<{ artifacts: DebugArtifacts; onClose: () => void }> = ({ artifacts, onClose }) => {
  const rendered = artifacts.results?.filter((r) => r.status === 'rendered').length || 0;
  const skipped = artifacts.results?.filter((r) => r.status === 'skipped').length || 0;
  const failed = artifacts.results?.filter((r) => r.status === 'failed').length || 0;

  return (
    <div className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4" onClick={onClose}>
      <div
        className="relative w-full max-w-3xl max-h-[85vh] bg-slate-900 rounded-2xl border border-slate-700 overflow-hidden flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 px-4 py-3 bg-slate-800 border-b border-slate-700 flex-shrink-0">
          <span className="text-amber-300 text-sm font-medium tracking-wide uppercase">Debug Artifacts</span>
          <span className="flex items-center gap-2 text-[10px] font-mono">
            {rendered > 0 && <span className="text-green-400/80">{rendered} ok</span>}
            {skipped > 0 && <span className="text-yellow-400/80">{skipped} skip</span>}
            {failed > 0 && <span className="text-red-400/80">{failed} fail</span>}
          </span>
          <button
            onClick={onClose}
            className="ml-auto p-1.5 rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors"
            title="Close"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="px-4 py-3 overflow-y-auto">
          <DebugPanelBody artifacts={artifacts} />
        </div>
      </div>
    </div>
  );
};

// Shared implementation behind both /chat and /chat-debug. The two routes
// (Chatbot.tsx and ChatDebugPage.tsx) are thin wrappers around this
// component so a single message-rendering/send/loading path can never drift
// between the two surfaces - see the "Chat UI" rule in AGENTS.md. The only
// surface-specific behavior gated here is the debug-artifacts button/dialog
// (surface === 'chat_debug'); everything else renders identically.
export const ChatConversation: React.FC<{ surface: ChatSurface }> = ({ surface }) => {
  const showDebug = surface === 'chat_debug';
  const basePath = basePathForSurface(surface);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const { conversationId: conversationIdParam } = useParams<{ conversationId?: string }>();
  const conversationId = conversationIdParam ? Number(conversationIdParam) : undefined;
  const { data: conversationDetail } = useQuery({
    queryKey: ['chat', 'conversation', conversationId],
    queryFn: () => getConversation(conversationId as number),
    enabled: conversationId !== undefined,
  });
  // True only when we're switching to a conversation we don't already have
  // (from cache priming below, or a normal cache hit) - lets the loading
  // state below distinguish "fetching a thread" from "genuinely new, empty
  // chat", instead of flashing the empty-state screen while data is in
  // flight.
  const isLoadingConversation = conversationId !== undefined && conversationDetail === undefined;
  const user = useAppSelector((state) => state.Auth.user);
  const organizations = useAppSelector((state) => state.Auth.organizations);
  const currentOrgId = useAppSelector((state) => state.Organizations.currentOrgId);
  const currentOrg = useAppSelector((state) => state.Organizations.currentOrg);
  const { isSuperAdmin } = useOrgContext();

  const downloadFile = useCallback(
    async (file: ChatFile) => {
      const url = resolveImageUrl(file.url);
      if (!url) return;
      try {
        // authFetch carries the same auth headers apiClient sends, and
        // transparently refreshes+retries once on a 401 - a bare fetch()
        // has neither, so a download made right as the access token
        // expires would otherwise fail outright instead of recovering.
        const res = await authFetch(url);
        if (!res.ok) throw new Error('download failed');
        const blob = await res.blob();
        const objectUrl = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objectUrl;
        a.download = file.filename || 'livereview-export.csv';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(objectUrl);
      } catch {
        alert('Could not download the file. It may have expired — try asking again.');
      }
    },
    [],
  );

  // "Export As" - lives in the shared component so it's identical on both
  // surfaces (see the Chat UI rule in AGENTS.md); only the backend decides
  // whether debug artifacts belong in the file, from the conversation's own
  // stored surface, not from anything sent here.
  const [exportMenuOpen, setExportMenuOpen] = useState(false);
  const [exportingFormat, setExportingFormat] = useState<'pdf' | 'html' | 'json' | null>(null);
  const exportMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!exportMenuOpen) return;
    const onClickAway = (e: MouseEvent) => {
      if (exportMenuRef.current && !exportMenuRef.current.contains(e.target as Node)) {
        setExportMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', onClickAway);
    return () => document.removeEventListener('mousedown', onClickAway);
  }, [exportMenuOpen]);

  const userName = user?.name || 'there';
  const [messages, setMessages] = useState<ChatEntry[]>([]);
  const [input, setInput] = useState(() => searchParams.get('prefill') || '');
  const [isLoading, setIsLoading] = useState(false);
  const [preview, setPreview] = useState<ChatChart | null>(null);
  const [previewKey, setPreviewKey] = useState<string | null>(null);
  const [chartGranularity, setChartGranularity] = useState<Record<string, Granularity>>({});
  const [debugModalMsgId, setDebugModalMsgId] = useState<string | null>(null);
  const previewViewRef = useRef<View | null>(null);
  // Inline charts render their own View independent of the modal's, so a
  // chart can be downloaded straight from the chat without first expanding
  // it. Keyed per chart instance since one message can carry several charts.
  const chartViewsRef = useRef<Map<string, View>>(new Map());
  const [showAISetup, setShowAISetup] = useState(false);
  const [dismissedProdUrlWarning, setDismissedProdUrlWarning] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleExport = useCallback(
    async (format: 'pdf' | 'html' | 'json') => {
      if (conversationId === undefined) return;
      setExportMenuOpen(false);
      setExportingFormat(format);

      if (format === 'json') {
        try {
          const exportData = buildJsonExport(
            messages,
            user,
            organizations,
            currentOrgId,
            currentOrg,
            conversationDetail,
          );
          downloadJsonExport(exportData);
        } catch {
          alert('Could not export this conversation. Please try again.');
        } finally {
          setExportingFormat(null);
        }
        return;
      }

      try {
        const res = await authFetch(`/api/v1/chat/${conversationId}/export?format=${format}`);
        if (!res.ok) throw new Error('export failed');
        const blob = await res.blob();
        const disposition = res.headers.get('Content-Disposition') || '';
        const match = disposition.match(/filename="?([^";]+)"?/);
        const filename = match?.[1] || `conversation.${format}`;
        const objectUrl = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objectUrl;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(objectUrl);
      } catch {
        alert('Could not export this conversation. Please try again.');
      } finally {
        setExportingFormat(null);
      }
    },
    [conversationId, messages, user, organizations, currentOrgId, currentOrg, conversationDetail],
  );

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!isLoading) {
      inputRef.current?.focus();
    }
  }, [isLoading]);

  // Seed local message state once the persisted conversation loads. The
  // server is the source of truth for history now; this component only
  // mounts fresh per conversation (see the `key` on the route in
  // ChatbotRoutes.tsx), so this only needs to run when the fetched detail
  // itself changes, not on every render.
  useEffect(() => {
    if (!conversationDetail) return;
    setMessages(
      conversationDetail.messages.map((m) => ({
        id: String(m.id),
        role: m.role,
        text: m.content,
        charts: m.charts && m.charts.length > 0 ? m.charts : undefined,
        files: m.files && m.files.length > 0 ? m.files : undefined,
        suggestedQuestions: m.suggested_questions,
        debugArtifacts: m.debug_artifacts as DebugArtifacts | undefined,
      })),
    );
  }, [conversationDetail]);

  // Tracks a conversation created for THIS "new chat" send before the URL
  // catches up - see handleSend. Cleared whenever the route's conversationId
  // changes (a genuine switch, not our own pending creation).
  const pendingConversationIdRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    pendingConversationIdRef.current = undefined;
  }, [conversationId]);

  const handleSend = async () => {
    const text = input.trim();
    if (!text || isLoading) return;
    setInput('');

    const userEntry: ChatEntry = { id: generateId(), role: 'user', text };
    setMessages((prev) => [...prev, userEntry]);
    setIsLoading(true);

    try {
      let activeConversationId = conversationId ?? pendingConversationIdRef.current;
      if (activeConversationId === undefined) {
        // Create the conversation (and surface it in the sidebar) right
        // away, before waiting on the agent's answer - otherwise a new
        // chat wouldn't appear in the sidebar until the first reply lands.
        const created = await createConversation(text, surface);
        activeConversationId = created.id;
        pendingConversationIdRef.current = created.id;
        queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
      }

      const result = await sendChatMessage(text, activeConversationId);

      const assistantEntry: ChatEntry = {
        id: generateId(),
        role: 'assistant',
        text: result.response,
        charts: result.charts && result.charts.length > 0 ? result.charts : undefined,
        files: result.files && result.files.length > 0 ? result.files : undefined,
        suggestedQuestions: result.suggested_questions,
        debugArtifacts: result.debug_artifacts as DebugArtifacts | undefined,
      };
      setMessages((prev) => [...prev, assistantEntry]);
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
      // First message of a new conversation: move to its URL so it's
      // bookmarkable/shareable and the sidebar highlights it. The route is
      // keyed by conversationId (see ChatbotRoutes.tsx), so this remounts
      // and re-hydrates from the now-persisted turn - simpler and more
      // correct than reconciling local optimistic state by hand, and safe
      // to do only now since the turn is already fully persisted.
      //
      // The remount would otherwise start from an empty local state and
      // wait on a fresh GET before showing anything - a jarring flash right
      // after the answer already appeared once. Priming the query cache
      // with what we already have lets the remounted page render instantly;
      // the query still revalidates in the background and reconciles once
      // the real, server-assigned message/chart ids come back.
      if (conversationId === undefined && activeConversationId !== undefined) {
        queryClient.setQueryData(['chat', 'conversation', activeConversationId], {
          id: activeConversationId,
          title: text,
          updatedAt: new Date().toISOString(),
          messages: [userEntry, assistantEntry].map((entry, i) => ({
            id: -(i + 1),
            role: entry.role,
            content: entry.text,
            charts: entry.charts,
            files: entry.files,
            suggested_questions: entry.suggestedQuestions,
            debug_artifacts: entry.debugArtifacts,
          })),
        });
        navigate(`${basePath}/${activeConversationId}`, { replace: true });
      }
    } catch (err: any) {
      const errMsg = err?.response?.data?.error || err?.message || 'Request failed';
      if (errMsg.toLowerCase().includes('ai connector')) {
        setShowAISetup(true);
        setMessages((prev) => [
          ...prev,
          {
            id: generateId(),
            role: 'assistant',
            text: 'No AI provider is configured for this organization yet. Add one below to start chatting.',
          },
        ]);
        return;
      }
      setMessages((prev) => [
        ...prev,
        {
          id: generateId(),
          role: 'assistant',
          text: `Error: ${errMsg}`,
        },
      ]);
    } finally {
      setIsLoading(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const [previewSize, setPreviewSize] = useState<{ width: number; height: number }>({ width: 840, height: 480 });

  const openPreview = useCallback((chart: ChatChart, chartKey: string) => {
    // Size the expanded chart off the current viewport so "expand" reads as
    // a real full-screen analysis view rather than a slightly bigger card.
    setPreviewSize({
      width: Math.round(Math.min(1200, window.innerWidth * 0.85)),
      height: Math.round(Math.min(720, window.innerHeight * 0.7)),
    });
    setPreview(chart);
    setPreviewKey(chartKey);
  }, []);

  const closePreview = useCallback(() => {
    setPreview(null);
    setPreviewKey(null);
    previewViewRef.current = null;
  }, []);

  const debugModalMsg = debugModalMsgId ? messages.find((m) => m.id === debugModalMsgId) : undefined;

  return (
    <div className="h-full flex flex-col bg-slate-900">
      <div className="flex-none px-4 py-2">
        <div className="max-w-4xl mx-auto flex items-center justify-between">
          <div className="flex items-center gap-2">
            <img src="/assets/lrbot/lrbot.png" alt="Bot" width={20} height={20} decoding="async" className="w-5 h-5 rounded-full opacity-80" />
            <h1 className="text-sm font-medium text-slate-400">Chat with Livi</h1>
          </div>
          <div className="flex items-center gap-2">
            {conversationId !== undefined && (
              <div className="relative" ref={exportMenuRef}>
                <button
                  onClick={() => setExportMenuOpen((v) => !v)}
                  disabled={exportingFormat !== null}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-slate-200 transition-colors text-xs font-medium disabled:opacity-50"
                  title="Export this conversation"
                >
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v12m0 0l-4-4m4 4l4-4M4 20h16" />
                  </svg>
                  {exportingFormat ? `Exporting ${exportingFormat.toUpperCase()}…` : 'Export As'}
                </button>
                {exportMenuOpen && (
                  <div className="absolute right-0 mt-1 w-40 rounded-md bg-slate-800 border border-slate-700 shadow-lg py-1 z-20">
                    <button
                      onClick={() => handleExport('pdf')}
                      className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 hover:text-slate-100"
                    >
                      Export as PDF
                    </button>
                    <button
                      onClick={() => handleExport('html')}
                      className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 hover:text-slate-100"
                    >
                      Export as HTML
                    </button>
                    {showDebug && (
                      <button
                        onClick={() => handleExport('json')}
                        className="w-full text-left px-3 py-1.5 text-xs text-amber-300 hover:bg-slate-700 hover:text-amber-200"
                      >
                        Export as JSON (debug)
                      </button>
                    )}
                  </div>
                )}
              </div>
            )}
            <button
              onClick={() => navigate('/dashboard')}
              className="p-1.5 rounded-md bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-slate-200 transition-colors"
              title="Back to dashboard"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-4 py-6">
        <div className="max-w-4xl mx-auto w-full min-h-full flex flex-col relative">
          {isSuperAdmin && !dismissedProdUrlWarning && (
            <ProductionUrlWarning floating onClose={() => setDismissedProdUrlWarning(true)} />
          )}
          {showAISetup && (
            <div className="mb-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-slate-800 border border-slate-700 rounded-xl px-4 py-3">
              <div>
                <p className="text-sm font-medium text-slate-200">No AI provider configured</p>
                <p className="text-xs text-slate-400 mt-0.5">This chatbot needs an AI provider to work. Add one to start chatting.</p>
              </div>
              <button
                onClick={() => navigate('/ai')}
                className="inline-flex items-center gap-1.5 text-sm font-medium px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white transition-colors cursor-pointer whitespace-nowrap"
              >
                Add an AI provider
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                </svg>
              </button>
            </div>
          )}
          {isLoadingConversation ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-3">
              <div className="w-8 h-8 border-2 border-slate-600 border-t-indigo-400 rounded-full animate-spin" />
              <p className="text-sm text-slate-500">Loading conversation…</p>
            </div>
          ) : messages.length === 0 ? (
            <div className="flex-1 flex flex-col items-center justify-center text-center space-y-6 px-4">
              <p className="text-2xl font-semibold text-slate-200">Hello {userName}! How can I help you?</p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-2xl w-full">
                <ExampleCard
                  text="How many reviews had critical or high blast radius issues this month? Show those repositories"
                  onClick={() => setInput('How many reviews had critical or high blast radius issues this month? Show those repositories')}
                />
                <ExampleCard
                  text="Who has adopted LiveReview—and who hasn't?"
                  onClick={() => setInput('Who has adopted LiveReview—and who hasn\u0027t?')}
                />
                <ExampleCard
                  text="Are engineers actually incorporating reviews into their daily workflow?"
                  onClick={() => setInput('Are engineers actually incorporating reviews into their daily workflow?')}
                />
                <ExampleCard
                  text="Show me how our review productivity and throughput have trended over the last few months"
                  onClick={() => setInput('Show me how our review productivity and throughput have trended over the last few months')}
                />
                <ExampleCard
                  text="Trigger a full code review on my latest pull request"
                  onClick={() => setInput('Trigger a full code review on my latest pull request')}
                />
                <ExampleCard
                  text="Help me add an AI provider to Livereview"
                  onClick={() => setInput('Help me add an AI provider to Livereview')}
                />
              </div>
              <p className="text-sm text-slate-500">Try one of these, or ask your own question.</p>
            </div>
          ) : (
            <div className="space-y-6 py-2">
              {messages.map((msg) =>
                msg.role === 'user' ? (
                  <div key={msg.id} className="flex items-start justify-end">
                    <div className="bg-indigo-600 text-white rounded-full rounded-br-md px-4 py-2 max-w-[75%] whitespace-pre-wrap break-words text-base">
                      {formatText(msg.text)}
                    </div>
                  </div>
                ) : (
                  <div key={msg.id} className="relative flex items-start">
                    <div className="absolute -left-16 bottom-0 w-16 flex justify-end pr-2">
                      <img src="/assets/lrbot/lrbot.png" alt="Bot" width={32} height={32} decoding="async" className="w-8 h-8 rounded-full" />
                    </div>
                    <div className="min-w-0 flex-1">
                      {msg.charts && msg.charts.length > 0 && (
                        <div className="space-y-6">
                          {msg.charts.map((chart, i) => {
                            const chartKey = `${msg.id}-${i}`;
                            // Chart pixels (the rolling-avg/baseline re-bucketing) stay a
                            // frontend rendering concern - isDailyTrendChart/buildTrendSpec
                            // are unchanged. The KPI numbers themselves are precomputed
                            // backend-side (internal/chatstats.ComputeAllStats) and arrive
                            // as chart.stats; this just picks which precomputed object to
                            // show, with no client-side math at all.
                            const trendChart = isDailyTrendChart(chart.spec);
                            const granularity = chartGranularity[chartKey] ?? 'day';
                            const displaySpec = trendChart ? buildTrendSpec(chart.spec, granularity) : chart.spec;
                            const stats = chart.stats;
                            const trendStats = stats?.kind === 'trend' ? stats[granularity] ?? null : null;
                            const multiSeriesTrendStats =
                              stats?.kind === 'multi_series_trend' ? stats[granularity] ?? null : null;
                            const bandStats = stats?.kind === 'band' ? stats.stats : null;
                            const heatmapStats = stats?.kind === 'heatmap' ? stats.stats : null;
                            const slopeStats = stats?.kind === 'slope' ? stats.stats : null;
                            const categoryStats = stats?.kind === 'category' ? stats.stats : null;
                            const genericStats = stats?.kind === 'generic' ? stats.stats : null;
                            return (
                            <div key={chart.title || i} className="space-y-3">
                              <div className="space-y-3 !mt-2 !mb-8">
                                {(chart.title || trendChart) && (
                                  <div className="flex items-center justify-between gap-3">
                                    {chart.title && (
                                      <h3 className="text-sm font-semibold text-slate-300">{chart.title}</h3>
                                    )}
                                    {trendChart && (
                                      <GranularityToggle
                                        value={granularity}
                                        onChange={(g) =>
                                          setChartGranularity((prev) => ({ ...prev, [chartKey]: g }))
                                        }
                                      />
                                    )}
                                  </div>
                                )}
                                <div className="group relative overflow-x-auto max-w-full rounded-lg border border-slate-700">
                                  <InteractiveChart
                                    spec={displaySpec}
                                    className="block"
                                    onViewReady={(view) => {
                                      chartViewsRef.current.set(chartKey, view);
                                    }}
                                  />
                                  <div className="absolute top-2 right-2 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                                    <button
                                      onClick={() =>
                                        downloadChartView(chartViewsRef.current.get(chartKey) ?? null, chartFileName(chart.title))
                                      }
                                      className="p-1.5 rounded-md bg-slate-800/90 text-slate-300 hover:text-white hover:bg-slate-700 shadow-lg transition-colors"
                                      title="Download chart"
                                      aria-label="Download chart"
                                    >
                                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 3v12m-4-4l4 4 4-4" />
                                      </svg>
                                    </button>
                                    <button
                                      onClick={() => openPreview(chart, chartKey)}
                                      className="p-1.5 rounded-md bg-slate-800/90 text-slate-300 hover:text-white hover:bg-slate-700 shadow-lg transition-colors"
                                      title="Expand chart"
                                      aria-label="Expand chart"
                                    >
                                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 8V4m0 0h4M4 4l5 5m11-5v4m0-4h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
                                      </svg>
                                    </button>
                                  </div>
                                </div>
                              </div>
                              {chart.description && (
                                <p className="text-sm text-slate-300 whitespace-pre-line">{chart.description}</p>
                              )}
                              {bandStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                                  <StatChip label="Active users" value={bandStats.totalActive.toLocaleString()} />
                                  <StatChip
                                    label="Largest band"
                                    value={`${bandStats.largest.label} (${bandStats.largest.value})`}
                                  />
                                  <StatChip label="Largest band's share" value={`${bandStats.largestSharePct}%`} />
                                </div>
                              )}
                              {slopeStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                                  <StatChip label="Entities" value={String(slopeStats.entityCount)} />
                                  <StatChip
                                    label="Gained / Lost / Flat"
                                    value={`${slopeStats.gained} / ${slopeStats.lost} / ${slopeStats.flat}`}
                                  />
                                  <StatChip
                                    label="Biggest gain"
                                    value={`${slopeStats.biggestGain.label} (+${slopeStats.biggestGain.delta.toLocaleString()})`}
                                  />
                                  <StatChip
                                    label="Biggest loss"
                                    value={`${slopeStats.biggestLoss.label} (${slopeStats.biggestLoss.delta.toLocaleString()})`}
                                  />
                                </div>
                              )}
                              {heatmapStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                                  <StatChip label="Total" value={heatmapStats.total.toLocaleString()} />
                                  <StatChip label="Active days" value={String(heatmapStats.activeDays)} />
                                  <StatChip label="Avg on active days" value={heatmapStats.avgOnActiveDays.toLocaleString()} />
                                  <StatChip
                                    label="Busiest day"
                                    value={`${formatAxisDate(heatmapStats.busiest.date)} (${heatmapStats.busiest.value.toLocaleString()})`}
                                  />
                                </div>
                              )}
                              {multiSeriesTrendStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                                  <StatChip label="Total" value={multiSeriesTrendStats.total.toLocaleString()} />
                                  <StatChip label="Series" value={String(multiSeriesTrendStats.seriesCount)} />
                                  <StatChip
                                    label="Top series"
                                    value={`${multiSeriesTrendStats.topSeries.label} (${multiSeriesTrendStats.topSeries.value.toLocaleString()})`}
                                  />
                                  <StatChip
                                    label="Range"
                                    value={`${formatAxisDate(multiSeriesTrendStats.firstDate)} → ${formatAxisDate(multiSeriesTrendStats.lastDate)}`}
                                  />
                                </div>
                              )}
                              {trendStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                                  <StatChip label="Total" value={trendStats.total.toLocaleString()} />
                                  <StatChip label="Avg per period" value={trendStats.avgPerPeriod.toLocaleString()} />
                                  <StatChip
                                    label="Peak"
                                    value={`${trendStats.peak.value.toLocaleString()} (${formatAxisDate(trendStats.peak.date)})`}
                                  />
                                  <StatChip
                                    label="Low"
                                    value={`${trendStats.low.value.toLocaleString()} (${formatAxisDate(trendStats.low.date)})`}
                                  />
                                  <StatChip
                                    label="Trend"
                                    value={
                                      trendStats.trendPct === null
                                        ? 'n/a'
                                        : `${trendStats.trendPct >= 0 ? 'up' : 'down'} ${Math.abs(trendStats.trendPct)}% (${formatAxisDate(trendStats.firstDate)} → ${formatAxisDate(trendStats.lastDate)})`
                                    }
                                  />
                                </div>
                              )}
                              {categoryStats && (
                                <div className="grid grid-cols-2 gap-2">
                                  <StatChip
                                    label="Highest"
                                    value={`${categoryStats.highest.label} (${categoryStats.highest.value.toLocaleString()})`}
                                  />
                                  <StatChip
                                    label="Lowest"
                                    value={`${categoryStats.lowest.label} (${categoryStats.lowest.value.toLocaleString()})`}
                                  />
                                  <StatChip
                                    label="Top 3"
                                    value={categoryStats.top3.map((s) => `${s.label} (${s.value.toLocaleString()})`).join(', ')}
                                  />
                                  <StatChip
                                    label="Bottom 3"
                                    value={categoryStats.bottom3.map((s) => `${s.label} (${s.value.toLocaleString()})`).join(', ')}
                                  />
                                </div>
                              )}
                              {genericStats && (
                                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                                  <StatChip label="Total" value={genericStats.total.toLocaleString()} />
                                  <StatChip label="Count" value={String(genericStats.count)} />
                                  <StatChip
                                    label="Highest"
                                    value={`${genericStats.highest.label} (${genericStats.highest.value.toLocaleString()})`}
                                  />
                                  <StatChip
                                    label="Lowest"
                                    value={`${genericStats.lowest.label} (${genericStats.lowest.value.toLocaleString()})`}
                                  />
                                </div>
                              )}
                              {(chart.query || chart.time_range || chart.granularity || chart.context) && (
                                <details className="group mt-1">
                                  <summary className="text-xs text-slate-500 cursor-pointer hover:text-slate-400 select-none">
                                    Data details
                                  </summary>
                                  <div className="mt-1.5 space-y-1 text-xs text-slate-400 italic">
                                    {chart.context && <ContextDetails context={chart.context} />}
                                    {chart.time_range && (
                                      <p><span className="not-italic font-medium text-slate-400">Time range:</span> {chart.time_range}</p>
                                    )}
                                    {chart.granularity && (
                                      <p>
                                        <span className="not-italic font-medium text-slate-400">Granularity:</span>{' '}
                                        {trendChart
                                          ? GRANULARITY_OPTIONS.find((o) => o.value === granularity)?.label
                                          : chart.granularity}
                                      </p>
                                    )}
                                    {chart.query && (
                                      <p><span className="not-italic font-medium text-slate-400">Query:</span> {chart.query}</p>
                                    )}
                                  </div>
                                </details>
                              )}
                            </div>
                          );})}
                        </div>
                      )}
                      {msg.files && msg.files.length > 0 && (
                        <div className="space-y-3">
                          {msg.files.map((file, i) => (
                            <div key={file.url || file.filename || i} className="space-y-2">
                              {file.title && (
                                <h3 className="text-sm font-semibold text-slate-300">{file.title}</h3>
                              )}
                              <button
                                onClick={() => downloadFile(file)}
                                className="flex items-center gap-3 w-full max-w-md px-4 py-3 rounded-lg border border-slate-700 bg-slate-800/60 hover:bg-slate-800 hover:border-slate-600 transition-colors text-left"
                                title={`Download ${file.filename}`}
                              >
                                <svg className="w-5 h-5 shrink-0 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                                </svg>
                                <span className="min-w-0 flex-1">
                                  <span className="block text-sm font-medium text-slate-200 truncate">{file.filename}</span>
                                  <span className="block text-xs text-slate-400">{formatRowCount(file.rows)}</span>
                                </span>
                              </button>
                              {file.description && (
                                <p className="text-sm text-slate-300 whitespace-pre-line">{file.description}</p>
                              )}
                              {(file.query || file.time_range || file.granularity || file.context) && (
                                <details className="group mt-1">
                                  <summary className="text-xs text-slate-500 cursor-pointer hover:text-slate-400 select-none">
                                    Data details
                                  </summary>
                                  <div className="mt-1.5 space-y-1 text-xs text-slate-400 italic">
                                    {file.context && <ContextDetails context={file.context} />}
                                    {file.time_range && (
                                      <p><span className="not-italic font-medium text-slate-400">Time range:</span> {file.time_range}</p>
                                    )}
                                    {file.granularity && (
                                      <p><span className="not-italic font-medium text-slate-400">Granularity:</span> {file.granularity}</p>
                                    )}
                                    {file.query && (
                                      <p><span className="not-italic font-medium text-slate-400">Query:</span> {file.query}</p>
                                    )}
                                  </div>
                                </details>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                      {msg.text && (
                        <div className={`${(msg.charts && msg.charts.length > 0) || (msg.files && msg.files.length > 0) ? 'mt-6' : ''} text-base leading-snug whitespace-pre-wrap break-words text-slate-200`}>
                          {formatText(msg.text)}
                        </div>
                      )}
                      {msg.suggestedQuestions && msg.suggestedQuestions.length > 0 && (
                        <div className="mt-4 space-y-4">
                          {msg.suggestedQuestions.map((cat: SuggestedQuestionCategory, catIdx: number) => (
                            <div key={catIdx} className="space-y-2">
                              <h4 className="text-sm font-semibold text-indigo-400">{cat.category}</h4>
                              <div className="flex flex-wrap gap-2">
                                {cat.questions.map((q: string, qIdx: number) => (
                                  <button
                                    key={qIdx}
                                    onClick={() => {
                                      setInput(q);
                                      inputRef.current?.focus();
                                    }}
                                    className="text-left text-sm bg-slate-800/80 hover:bg-slate-700 border border-slate-700/80 hover:border-indigo-500 text-slate-200 px-3.5 py-2 rounded-lg transition-all cursor-pointer shadow-sm hover:shadow-indigo-500/10"
                                  >
                                    {q}
                                  </button>
                                ))}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                      {/* Debug artifacts trigger - chat_debug surface only, opens a dialog instead of an inline dump. */}
                      {showDebug && msg.debugArtifacts && (
                        <DebugTrigger artifacts={msg.debugArtifacts} onOpen={() => setDebugModalMsgId(msg.id)} />
                      )}
                    </div>
                  </div>
                )
              )}
            </div>
          )}

          {isLoading && (
            <div className="relative flex items-start mt-4">
              <div className="absolute -left-16 top-1/2 -translate-y-1/2 w-16 flex justify-end pr-2">
                <img src="/assets/lrbot/lrbot.png" alt="Bot" width={32} height={32} decoding="async" className="w-8 h-8 rounded-full" />
              </div>
              <ThinkingIndicator />
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>
      </div>

      <div className="flex-none px-4 pb-4">
        <div className="max-w-4xl mx-auto">
          <div className="relative flex items-center">
            <input
              ref={inputRef}
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask for insights or get things done — reviews, trends, billing, and more..."
              disabled={isLoading}
              className="w-full bg-slate-700 text-slate-100 placeholder-slate-400 rounded-full pl-5 pr-14 py-3 focus:outline-none focus:ring-2 focus:ring-indigo-500 border border-slate-600 disabled:opacity-50"
            />
            <button
              onClick={handleSend}
              disabled={isLoading || !input.trim()}
              className="absolute right-2 top-1/2 -translate-y-1/2 bg-indigo-600 hover:bg-indigo-700 disabled:bg-slate-600 disabled:cursor-not-allowed text-white rounded-full p-2 transition-colors disabled:opacity-50"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12Zm0 0h7.5" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      {preview && (
        <div
          className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4"
          onClick={closePreview}
        >
          <div
            className="relative w-full max-w-[95vw] max-h-[95vh] bg-slate-900 rounded-2xl border border-slate-700 overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-4 py-3 bg-slate-800 border-b border-slate-700">
              <h3 className="text-sm font-semibold text-slate-200 truncate pr-4">
                {preview.title || 'Chart Preview'}
              </h3>
              <div className="flex items-center gap-2">
                {isDailyTrendChart(preview.spec) && (
                  <GranularityToggle
                    value={previewKey ? chartGranularity[previewKey] ?? 'day' : 'day'}
                    onChange={(g) =>
                      previewKey && setChartGranularity((prev) => ({ ...prev, [previewKey]: g }))
                    }
                  />
                )}
                <button
                  onClick={() => downloadChartView(previewViewRef.current, chartFileName(preview.title))}
                  className="inline-flex items-center gap-1 text-xs font-medium text-slate-100 hover:text-white px-3 py-1.5 rounded-lg bg-slate-700 hover:bg-indigo-600 cursor-pointer transition-colors"
                  title="Download"
                >
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 3v12m-4-4l4 4 4-4" />
                  </svg>
                  Download
                </button>
                <button
                  onClick={closePreview}
                  className="p-1.5 rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors"
                  title="Close"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>
            <div className="max-h-[85vh] overflow-auto bg-slate-950 flex items-center justify-center p-4">
              <InteractiveChart
                spec={
                  isDailyTrendChart(preview.spec)
                    ? buildTrendSpec(preview.spec, previewKey ? chartGranularity[previewKey] ?? 'day' : 'day')
                    : preview.spec
                }
                width={previewSize.width}
                height={previewSize.height}
                onViewReady={(view) => {
                  previewViewRef.current = view;
                }}
              />
            </div>
          </div>
        </div>
      )}

      {showDebug && debugModalMsg?.debugArtifacts && (
        <DebugModal artifacts={debugModalMsg.debugArtifacts} onClose={() => setDebugModalMsgId(null)} />
      )}
    </div>
  );
};

const ExampleCard: React.FC<{ text: string; onClick: () => void }> = ({ text, onClick }) => (
  <button
    onClick={onClick}
    className="text-left px-4 py-3 rounded-xl bg-slate-800 border border-slate-700 hover:border-indigo-500 hover:bg-slate-750 text-slate-300 text-sm transition-all"
  >
    {text}
  </button>
);

export default ChatConversation;
