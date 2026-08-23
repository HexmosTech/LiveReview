import React, { useState, useRef, useEffect, useCallback } from 'react';
import type { View } from 'vega';
import { sendChatMessage, ChatFile, ChatChart } from '../../api/chatbot';
import { createConversation, getConversation } from '../../api/chatConversations';
import { BASE_URL } from '../../api/apiClient';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useAppSelector } from '../../store/configureStore';
import { InteractiveChart, downloadChartView } from './InteractiveChart';
import { ThinkingIndicator } from './ThinkingIndicator';
import { CONVERSATIONS_QUERY_KEY } from './ConversationSidebar';

interface ChatEntry {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  debugArtifacts?: DebugArtifacts | null;
}

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
  }>;
}

// Reads a mark's "type" whether it's the bare-string form ("bar") or the
// object form ({"type": "bar", ...}) Vega-Lite also allows.
function markTypeLabel(mark: unknown): string {
  if (typeof mark === 'string') return mark;
  if (mark && typeof mark === 'object' && typeof (mark as any).type === 'string') return (mark as any).type;
  return 'unknown';
}

// Describes a chart spec's shape for the debug view - what mark(s) it
// actually renders as, independent of whatever chart_type name the model
// picked (chart_type is a hint the finalize pipeline doesn't even receive,
// and the model's own encoding can diverge from it - this reads the spec
// Go actually built/sanitized instead of trusting that label).
function describeChartShape(spec: Record<string, unknown>): string {
  if (Array.isArray((spec as any).layer)) {
    const marks = ((spec as any).layer as Array<Record<string, unknown>>).map((l) => markTypeLabel(l.mark));
    return `layered (${marks.join(' + ')})`;
  }
  if ((spec as any).facet) {
    const innerMark = (spec as any).spec?.mark;
    return `faceted (${markTypeLabel(innerMark)})`;
  }
  return markTypeLabel((spec as any).mark);
}

function generateId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

function resolveImageUrl(url: string): string {
  if (!url || !url.startsWith('/')) return '';
  return `${BASE_URL}${url}`;
}

function formatRowCount(rows?: number): string {
  if (!rows || rows < 1) return 'CSV';
  return `CSV - ${rows.toLocaleString()} row${rows === 1 ? '' : 's'}`;
}

function formatText(text: string): React.ReactNode[] {
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
          <pre key={`code-${lineIdx++}`} className="bg-slate-800 text-green-300 p-3 rounded-lg overflow-x-auto text-sm my-2">
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
    parts.push(<div key={`line-${lineIdx++}`} className="mb-1">{line}</div>);
  }

  if (inCodeBlock && codeContent) {
    parts.push(
      <pre key={`code-${lineIdx++}`} className="bg-slate-800 text-green-300 p-3 rounded-lg overflow-x-auto text-sm my-2">
        {codeContent}
      </pre>
    );
  }
  return parts;
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

// Collapsible section with copy button
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

// Compact inline button that opens the full debug info in a dialog, so the
// chart card stays clean instead of growing a wall of raw SQL/JSON beneath it.
const DebugTrigger: React.FC<{ artifacts: DebugArtifacts; onOpen: () => void }> = ({ artifacts, onOpen }) => {
  const rendered = artifacts.results?.filter((r) => r.status === 'rendered').length || 0;
  const skipped = artifacts.results?.filter((r) => r.status === 'skipped').length || 0;
  const failed = artifacts.results?.filter((r) => r.status === 'failed').length || 0;

  return (
    <button
      onClick={onOpen}
      className="mt-1 inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-amber-500/20 bg-amber-500/5 hover:bg-amber-500/10 text-left transition-colors"
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

// Full debug artifacts content, shown inside a modal dialog (DebugModal below).
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
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <span className="text-[10px] text-slate-500 font-mono">{artifacts.interpretations?.length || 0} interpretations</span>
        <button
          onClick={toggleExpandAll}
          className="text-[10px] text-slate-500 hover:text-slate-300 transition-colors"
        >
          {expandAll ? 'Collapse All' : 'Expand All'}
        </button>
      </div>

      {/* Per-interpretation cards — numbered, with SQL + error + CSV + Vega coupled */}
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
              {/* Header */}
              <div className="flex items-center gap-2 px-3 py-2 bg-slate-800/50">
                <span className="text-[10px] font-mono text-slate-500 w-4">{i + 1}.</span>
                <span className={`w-2 h-2 rounded-full ${statusColor}`} />
                <span className="text-xs font-medium text-slate-200">{interp.title}</span>
                <span className="text-[10px] text-slate-500 font-mono">{interp.chart_type}</span>
                {result && <span className="text-[10px] text-slate-600 ml-auto font-mono">{result.row_count} rows</span>}
              </div>
              {/* Description */}
              <div className="px-3 py-2 text-[11px] text-slate-400 border-b border-slate-700/30">{interp.description}</div>
              {/* SQL + CSV + Vega coupled in one section */}
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
              </div>
            </div>
          );
        })}
      </div>

      {/* Separator */}
      <div className="border-t border-amber-500/10" />

      {/* General artifacts — uniform card style */}
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

      {/* Full LLM Request + Raw Response coupled */}
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

const ChatDebugPage: React.FC = () => {
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
  const user = useAppSelector((state) => state.Auth.user);
  const accessToken = useAppSelector((state) => state.Auth.accessToken);
  const currentOrgId = useAppSelector((state) => state.Organizations.currentOrgId);

  const downloadFile = useCallback(
    async (file: ChatFile) => {
      const url = resolveImageUrl(file.url);
      if (!url) return;
      try {
        const headers: Record<string, string> = {};
        if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`;
        if (currentOrgId) headers['X-Org-Context'] = String(currentOrgId);
        const res = await fetch(url, { headers, credentials: 'same-origin' });
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
        alert('Could not download the file.');
      }
    },
    [accessToken, currentOrgId],
  );

  const userName = user?.name || 'there';
  const [messages, setMessages] = useState<ChatEntry[]>([]);
  const [input, setInput] = useState(() => searchParams.get('prefill') || '');
  const [isLoading, setIsLoading] = useState(false);
  const [debugModalMsgId, setDebugModalMsgId] = useState<string | null>(null);
  const chartViewsRef = useRef<Map<string, View>>(new Map());
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => { scrollToBottom(); }, [messages, scrollToBottom]);
  useEffect(() => { inputRef.current?.focus(); }, []);
  useEffect(() => { if (!isLoading) inputRef.current?.focus(); }, [isLoading]);

  useEffect(() => {
    if (!conversationDetail) return;
    setMessages(
      conversationDetail.messages.map((m) => ({
        id: String(m.id),
        role: m.role,
        text: m.content,
        charts: m.charts && m.charts.length > 0 ? m.charts : undefined,
        files: m.files && m.files.length > 0 ? m.files : undefined,
        debugArtifacts: m.debug_artifacts as DebugArtifacts | undefined,
      })),
    );
  }, [conversationDetail]);

  const pendingConversationIdRef = useRef<number | undefined>(undefined);
  useEffect(() => { pendingConversationIdRef.current = undefined; }, [conversationId]);

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
        const created = await createConversation(text);
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
        debugArtifacts: result.debug_artifacts as DebugArtifacts | undefined,
      };
      setMessages((prev) => [...prev, assistantEntry]);
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
      if (!conversationId && activeConversationId) {
        navigate(`/chat-debug/${activeConversationId}`, { replace: true });
      }
    } catch (err) {
      const errMsg = err instanceof Error ? err.message : 'Unknown error';
      setMessages((prev) => [...prev, { id: generateId(), role: 'assistant', text: `Error: ${errMsg}` }]);
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

  const debugModalMsg = debugModalMsgId ? messages.find((m) => m.id === debugModalMsgId) : undefined;

  return (
    <div className="h-full flex flex-col bg-slate-900 text-white">
      {/* Header */}
      <div className="flex-none px-4 py-2 border-b border-slate-700">
        <div className="max-w-4xl mx-auto flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-amber-600 flex items-center justify-center text-sm font-bold flex-shrink-0">D</div>
          <div>
            <div className="font-semibold text-sm">Chat Debug</div>
            <div className="text-xs text-slate-400">Same as /chat but with debug artifacts visible</div>
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 h-0 min-h-0 overflow-y-auto px-4 py-6">
        <div className="max-w-4xl mx-auto w-full space-y-6">
          {messages.length === 0 && (
            <div className="text-center text-slate-400 mt-12">
              <div className="text-lg mb-2">Debug Mode</div>
              <div className="text-sm">Ask an analytics question to see intermediate pipeline artifacts.</div>
            </div>
          )}
          {messages.map((msg) =>
            msg.role === 'user' ? (
              <div key={msg.id} className="flex items-start justify-end">
                <div className="bg-indigo-600 text-white rounded-full rounded-br-md px-4 py-2 max-w-[75%] whitespace-pre-wrap break-words text-base">
                  {formatText(msg.text)}
                </div>
              </div>
            ) : (
              <div key={msg.id} className="relative flex items-start">
                <div className="absolute -left-11 top-0 w-8 flex justify-end">
                  <div className="w-8 h-8 rounded-full bg-amber-600 flex items-center justify-center text-xs font-bold">D</div>
                </div>
                <div className="min-w-0 flex-1 space-y-3">
                  {/* Charts — full width of the column, aspect-ratio preserved by InteractiveChart's fill-container mode */}
                  {msg.charts && msg.charts.length > 0 && (
                    <div className="space-y-6">
                      {msg.charts.map((chart, i) => (
                        <div key={i} className="space-y-1.5">
                          <div className="flex items-center gap-2">
                            {chart.title && <h3 className="text-sm font-semibold text-slate-300">{chart.title}</h3>}
                            <span className="text-[10px] uppercase tracking-wide font-mono text-amber-300/90 bg-amber-500/10 border border-amber-500/30 rounded px-1.5 py-0.5">
                              {describeChartShape(chart.spec)}
                            </span>
                          </div>
                          <div className="overflow-x-auto max-w-full rounded-lg border border-slate-700 bg-slate-800 p-3">
                            <InteractiveChart
                              spec={chart.spec}
                              className="block"
                              onViewReady={(view) => chartViewsRef.current.set(`${msg.id}-${i}`, view)}
                            />
                          </div>
                        </div>
                      ))}
                    </div>
                  )}

                  {/* Files */}
                  {msg.files && msg.files.length > 0 && (
                    <div className="space-y-1">
                      {msg.files.map((file, i) => (
                        <button
                          key={i}
                          onClick={() => downloadFile(file)}
                          className="flex items-center gap-2 text-cyan-400 hover:text-cyan-300 text-sm"
                        >
                          <span>📄</span>
                          <span>{file.title || file.filename}</span>
                          <span className="text-xs text-slate-500">{formatRowCount(file.rows)}</span>
                        </button>
                      ))}
                    </div>
                  )}

                  {/* Text */}
                  {msg.text && (
                    <div className="text-base leading-snug whitespace-pre-wrap break-words text-slate-200">
                      {formatText(msg.text)}
                    </div>
                  )}

                  {/* Debug artifacts trigger — opens a dialog instead of an inline dump */}
                  {msg.debugArtifacts && (
                    <DebugTrigger artifacts={msg.debugArtifacts} onOpen={() => setDebugModalMsgId(msg.id)} />
                  )}
                </div>
              </div>
            )
          )}
          {isLoading && (
            <div className="relative flex items-start">
              <div className="absolute -left-11 top-0 w-8 flex justify-end">
                <div className="w-8 h-8 rounded-full bg-amber-600 flex items-center justify-center text-xs font-bold">D</div>
              </div>
              <ThinkingIndicator />
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>
      </div>

      {/* Input */}
      <div className="flex-none border-t border-slate-700 px-4 py-4">
        <div className="max-w-4xl mx-auto flex gap-2">
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask an analytics question..."
            className="flex-1 bg-slate-800 text-white rounded-lg px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            disabled={isLoading}
          />
          <button
            onClick={handleSend}
            disabled={isLoading || !input.trim()}
            className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white rounded-lg px-4 py-2 text-sm font-medium"
          >
            Send
          </button>
        </div>
      </div>

      {/* Debug artifacts dialog */}
      {debugModalMsg?.debugArtifacts && (
        <DebugModal artifacts={debugModalMsg.debugArtifacts} onClose={() => setDebugModalMsgId(null)} />
      )}
    </div>
  );
};

export default ChatDebugPage;
