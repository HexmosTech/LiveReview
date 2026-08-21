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
  }>;
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

// Debug panel component
const DebugPanel: React.FC<{ artifacts: DebugArtifacts }> = ({ artifacts }) => {
  const [expanded, setExpanded] = useState(false);
  const [activeSection, setActiveSection] = useState<string | null>(null);

  return (
    <div className="mt-2 border border-dashed border-amber-500/40 rounded-lg bg-amber-500/5 p-3">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 text-amber-400 text-sm font-medium hover:text-amber-300 transition-colors w-full text-left"
      >
        <span className="text-xs">{expanded ? '\u25BC' : '\u25B6'}</span>
        Debug Artifacts
        <span className="text-xs text-amber-400/60 ml-auto">
          {artifacts.interpretations?.length || 0} interpretations,{' '}
          {artifacts.results?.filter(r => r.status === 'rendered').length || 0} rendered
        </span>
      </button>

      {expanded && (
        <div className="mt-3 space-y-3">
          {/* Interpretations summary */}
          <div>
            <h4 className="text-xs font-semibold text-amber-300 mb-2 uppercase tracking-wide">Interpretations</h4>
            <div className="space-y-2">
              {artifacts.interpretations?.map((interp, i) => {
                const result = artifacts.results?.[i];
                return (
                  <div key={i} className="bg-slate-800/50 rounded p-2 text-xs">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`inline-block w-2 h-2 rounded-full ${
                        result?.status === 'rendered' ? 'bg-green-400' :
                        result?.status === 'skipped' ? 'bg-yellow-400' :
                        'bg-red-400'
                      }`} />
                      <span className="text-slate-200 font-medium">{interp.title}</span>
                      <span className="text-slate-500">({interp.chart_type})</span>
                      {result && (
                        <span className="text-slate-500 ml-auto">{result.row_count} rows</span>
                      )}
                    </div>
                    <div className="text-slate-400 mb-1">{interp.description}</div>
                    <button
                      onClick={() => setActiveSection(activeSection === `sql-${i}` ? null : `sql-${i}`)}
                      className="text-cyan-400 hover:text-cyan-300 text-xs"
                    >
                      {activeSection === `sql-${i}` ? 'Hide SQL' : 'Show SQL'}
                    </button>
                    {activeSection === `sql-${i}` && (
                      <pre className="mt-1 bg-slate-900 rounded p-2 text-green-300 overflow-x-auto">
                        {interp.sql}
                      </pre>
                    )}
                    {result?.stats && result.stats.length > 0 && (
                      <div className="mt-1 text-slate-400">
                        {result.stats.map((s, j) => (
                          <div key={j}>{s}</div>
                        ))}
                      </div>
                    )}
                    {result?.skip_reason && (
                      <div className="mt-1 text-yellow-400/80 text-xs">Skipped: {result.skip_reason}</div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Schema context */}
          <div>
            <button
              onClick={() => setActiveSection(activeSection === 'schema' ? null : 'schema')}
              className="text-xs text-amber-300 hover:text-amber-200"
            >
              {activeSection === 'schema' ? '\u25BC' : '\u25B6'} Schema Context
            </button>
            {activeSection === 'schema' && (
              <pre className="mt-1 bg-slate-900 rounded p-2 text-xs text-slate-300 max-h-48 overflow-auto whitespace-pre-wrap">
                {artifacts.schema_context}
              </pre>
            )}
          </div>

          {/* System prompt */}
          <div>
            <button
              onClick={() => setActiveSection(activeSection === 'prompt' ? null : 'prompt')}
              className="text-xs text-amber-300 hover:text-amber-200"
            >
              {activeSection === 'prompt' ? '\u25BC' : '\u25B6'} System Prompt
            </button>
            {activeSection === 'prompt' && (
              <pre className="mt-1 bg-slate-900 rounded p-2 text-xs text-slate-300 max-h-48 overflow-auto whitespace-pre-wrap">
                {artifacts.system_prompt}
              </pre>
            )}
          </div>

          {/* LLM raw response */}
          <div>
            <button
              onClick={() => setActiveSection(activeSection === 'raw' ? null : 'raw')}
              className="text-xs text-amber-300 hover:text-amber-200"
            >
              {activeSection === 'raw' ? '\u25BC' : '\u25B6'} LLM Raw Response
            </button>
            {activeSection === 'raw' && (
              <pre className="mt-1 bg-slate-900 rounded p-2 text-xs text-green-300 max-h-64 overflow-auto whitespace-pre-wrap">
                {artifacts.llm_raw_response}
              </pre>
            )}
          </div>
        </div>
      )}
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
  const [preview, setPreview] = useState<ChatChart | null>(null);
  const previewViewRef = useRef<View | null>(null);
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

  return (
    <div className="flex flex-col h-full bg-slate-900 text-white">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3 border-b border-slate-700">
        <div className="w-8 h-8 rounded-full bg-amber-600 flex items-center justify-center text-sm font-bold">D</div>
        <div>
          <div className="font-semibold">Chat Debug</div>
          <div className="text-xs text-slate-400">Same as /chat but with debug artifacts visible</div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="text-center text-slate-400 mt-12">
            <div className="text-lg mb-2">Debug Mode</div>
            <div className="text-sm">Ask an analytics question to see intermediate pipeline artifacts.</div>
          </div>
        )}
        {messages.map((msg) => (
          <div key={msg.id} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[85%] rounded-lg px-4 py-2 ${
              msg.role === 'user'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-100'
            }`}>
              <div>{formatText(msg.text)}</div>

              {/* Charts */}
              {msg.charts && msg.charts.length > 0 && (
                <div className="mt-3 space-y-3">
                  {msg.charts.map((chart, i) => (
                    <div key={i} className="bg-slate-700 rounded-lg p-3">
                      {chart.title && <div className="text-sm font-medium mb-1">{chart.title}</div>}
                      <InteractiveChart spec={chart.spec} onViewReady={(view) => chartViewsRef.current.set(`${msg.id}-${i}`, view)} />
                    </div>
                  ))}
                </div>
              )}

              {/* Files */}
              {msg.files && msg.files.length > 0 && (
                <div className="mt-2 space-y-1">
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

              {/* Debug artifacts */}
              {msg.debugArtifacts && <DebugPanel artifacts={msg.debugArtifacts} />}
            </div>
          </div>
        ))}
        {isLoading && (
          <div className="flex justify-start">
            <div className="bg-slate-800 rounded-lg px-4 py-2">
              <ThinkingIndicator />
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-slate-700 p-4">
        <div className="flex gap-2">
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

      {/* Chart preview modal */}
      {preview && (
        <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={() => setPreview(null)}>
          <div className="bg-slate-800 rounded-xl p-4 max-w-4xl max-h-[80vh] overflow-auto" onClick={(e) => e.stopPropagation()}>
            <InteractiveChart spec={preview.spec} onViewReady={(view) => { previewViewRef.current = view; }} />
          </div>
        </div>
      )}
    </div>
  );
};

export default ChatDebugPage;
