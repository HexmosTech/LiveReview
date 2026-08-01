import React, { useState, useRef, useEffect, useCallback } from 'react';
import { sendChatMessage, ChatHistoryEntry } from '../../api/chatbot';
import { BASE_URL } from '../../api/apiClient';
import { useNavigate } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';

interface ChatImage {
  url: string;
  title?: string;
  description?: string;
}

interface ChatEntry {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  images?: ChatImage[];
}

function generateId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

function resolveImageUrl(url: string): string {
  // Only allow relative, same-origin chart URLs. The backend always returns
  // relative /api/v1/chat/charts/... paths; rejecting absolute http(s) URLs
  // prevents loading arbitrary external content from a model-influenced URL.
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

async function downloadImage(img: ChatImage): Promise<void> {
  try {
    const res = await fetch(resolveImageUrl(img.url), { credentials: 'same-origin' });
    if (!res.ok) throw new Error('download failed');
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = chartFileName(img.title);
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch (err) {
    alert('Could not download the chart.');
  }
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

    const formattedLine = formatLine(line);
    parts.push(<div key={`line-${lineIdx++}`} className="mb-1">{formattedLine}</div>);
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

function formatLine(line: string): React.ReactNode {
  const parts: React.ReactNode[] = [];
  let i = 0;
  let partIdx = 0;

  while (i < line.length) {
    if (line[i] === '*' && i + 1 < line.length && line[i + 1] === '*') {
      const end = line.indexOf('**', i + 2);
      if (end >= 0) {
        parts.push(<strong key={`b-${partIdx++}`}>{line.slice(i + 2, end)}</strong>);
        i = end + 2;
        continue;
      }
    }
    if (line[i] === '*' && line[i + 1] !== '*') {
      const end = line.indexOf('*', i + 1);
      if (end >= 0) {
        parts.push(<em key={`i-${partIdx++}`}>{line.slice(i + 1, end)}</em>);
        i = end + 1;
        continue;
      }
    }
    if (line[i] === '`') {
      const end = line.indexOf('`', i + 1);
      if (end >= 0) {
        parts.push(
          <code key={`c-${partIdx++}`} className="bg-slate-700 text-cyan-300 px-1.5 py-0.5 rounded text-sm">
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
        <blockquote key={`q-${partIdx++}`} className="border-l-4 border-indigo-400 pl-3 text-slate-300 italic my-1">
          {formatLine(rest)}
        </blockquote>
      );
      i = line.length;
      continue;
    }

    const segEnd = findNextSpecial(line, i);
    parts.push(<span key={`t-${partIdx++}`}>{line.slice(i, segEnd)}</span>);
    i = segEnd;
  }

  return <>{parts}</>;
}

function findNextSpecial(line: string, from: number): number {
  for (let i = from; i < line.length; i++) {
    if (line[i] === '*' || line[i] === '`' || line[i] === '>') return i;
  }
  return line.length;
}

const Chatbot: React.FC = () => {
  const navigate = useNavigate();
  const user = useAppSelector((state) => state.Auth.user);
  const userName = user?.name || 'there';
  const [messages, setMessages] = useState<ChatEntry[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [history, setHistory] = useState<ChatHistoryEntry[]>([]);
  const [preview, setPreview] = useState<ChatImage | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  const handleSend = async () => {
    const text = input.trim();
    if (!text || isLoading) return;
    setInput('');

    const userEntry: ChatEntry = { id: generateId(), role: 'user', text };
    setMessages((prev) => [...prev, userEntry]);
    setIsLoading(true);

    try {
      const result = await sendChatMessage(text, history);
      setHistory(result.history);

      const assistantEntry: ChatEntry = {
        id: generateId(),
        role: 'assistant',
        text: result.response,
        images: result.images && result.images.length > 0 ? result.images : undefined,
      };
      setMessages((prev) => [...prev, assistantEntry]);
    } catch (err: any) {
      const errMsg = err?.response?.data?.error || err?.message || 'Request failed';
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

  return (
    <div className="h-[calc(100vh-4rem)] flex flex-col bg-slate-900">
      <div className="flex-none flex items-center justify-between px-6 py-3 border-b border-slate-700 bg-slate-800">
        <div className="flex items-center gap-3">
          <img src="/assets/lrbot/lrbot.png" alt="Bot" className="w-7 h-7 rounded-full" />
          <h1 className="text-lg font-semibold text-slate-100">Chat with Livereview Bot</h1>
        </div>
        <button
          onClick={() => navigate(-1)}
          className="p-2 rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors"
          title="Close"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-4 py-6 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center space-y-6 px-4">
            <p className="text-2xl font-semibold text-slate-200">Hello {userName}! How can I help you?</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-xl w-full">
              <ExampleCard
                text="Show me the top reviewers with a chart"
                onClick={() => setInput('Show me the top reviewers with a chart')}
              />
              <ExampleCard
                text="Give me a dashboard of reviews per status"
                onClick={() => setInput('Give me a dashboard of reviews per status')}
              />
              <ExampleCard
                text="Who reviewed the most lines of code?"
                onClick={() => setInput('Who reviewed the most lines of code?')}
              />
              <ExampleCard
                text="Show me review trends over time"
                onClick={() => setInput('Show me review trends over time')}
              />
            </div>
            <p className="text-sm text-slate-500">...and much more. Try asking about reviews, LOC, billing, trends, or comparisons.</p>
          </div>
        )}

        {messages.map((msg) => (
          <div key={msg.id} className={`flex items-start gap-2 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            {msg.role === 'assistant' && (
              <img src="/assets/lrbot/lrbot.png" alt="Bot" className="w-8 h-8 rounded-full flex-shrink-0 mt-1" />
            )}
            <div
              className={`max-w-[75%] rounded-2xl px-4 py-3 ${
                msg.role === 'user'
                  ? 'bg-indigo-600 text-white rounded-br-md'
                  : 'bg-slate-800 text-slate-200 rounded-bl-md border border-slate-700'
              }`}
            >
              {msg.role === 'assistant' && msg.images && msg.images.length > 0 && (
                <div className="space-y-4 mb-3">
                  {msg.images.map((img, i) => (
                    <div key={i} className="bg-slate-900 rounded-xl p-3 border border-slate-700">
                      {img.title && (
                        <div className="flex items-center justify-between gap-2 mb-2">
                          <h3 className="text-sm font-semibold text-slate-300">{img.title}</h3>
                          <button
                            onClick={() => downloadImage(img)}
                            className="inline-flex items-center gap-1 text-xs font-medium px-2.5 py-1 rounded-md bg-slate-700 text-slate-200 hover:bg-indigo-600 hover:text-white cursor-pointer transition-colors"
                            title="Download chart"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 3v12m-4-4l4 4 4-4" />
                            </svg>
                            Download
                          </button>
                        </div>
                      )}
                      <button
                        onClick={() => setPreview(img)}
                        className="block group relative overflow-hidden rounded-lg"
                        title="Click to expand"
                      >
                        <img
                          src={resolveImageUrl(img.url)}
                          alt={img.title || 'Chart'}
                          className="max-w-[280px] max-h-[200px] w-auto h-auto rounded-lg cursor-zoom-in border border-slate-700 transition-transform duration-300 ease-out group-hover:scale-105"
                          loading="lazy"
                        />
                        <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-slate-800/90 text-xs font-medium text-white shadow-lg">
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v6m-3-3h6" />
                            </svg>
                            Click to expand
                          </span>
                        </div>
                      </button>
                      {img.description && (
                        <p className="text-sm text-slate-300 mt-2">{img.description}</p>
                      )}
                    </div>
                  ))}
                </div>
              )}
              {msg.text && (
                <div className="text-sm leading-relaxed whitespace-pre-wrap break-words">
                  {formatText(msg.text)}
                </div>
              )}
            </div>
          </div>
        ))}

        {isLoading && (
          <div className="flex items-start gap-2 justify-start">
            <img src="/assets/lrbot/lrbot.png" alt="Bot" className="w-8 h-8 rounded-full flex-shrink-0 mt-1" />
            <div className="bg-slate-800 text-slate-200 rounded-2xl rounded-bl-md px-4 py-3 border border-slate-700">
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 bg-indigo-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <div className="w-2 h-2 bg-indigo-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <div className="w-2 h-2 bg-indigo-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <div className="flex-none border-t border-slate-700 bg-slate-800 px-4 py-3">
        <div className="flex items-center gap-2 max-w-4xl mx-auto">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Engineering insights — reviews, LOC, trends, billing..."
            disabled={isLoading}
            className="flex-1 bg-slate-700 text-slate-100 placeholder-slate-400 rounded-xl px-4 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-500 border border-slate-600 disabled:opacity-50"
          />
          <button
            onClick={handleSend}
            disabled={isLoading || !input.trim()}
            className="bg-indigo-600 hover:bg-indigo-700 disabled:bg-slate-600 text-white rounded-xl px-4 py-2.5 transition-colors disabled:opacity-50"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
            </svg>
          </button>
        </div>
      </div>

      {preview && (
        <div
          className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4"
          onClick={() => setPreview(null)}
        >
          <div
            className="relative max-w-4xl w-full bg-slate-900 rounded-2xl border border-slate-700 overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-4 py-3 bg-slate-800 border-b border-slate-700">
              <h3 className="text-sm font-semibold text-slate-200 truncate pr-4">
                {preview.title || 'Chart Preview'}
              </h3>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => downloadImage(preview)}
                  className="inline-flex items-center gap-1 text-xs font-medium text-slate-100 hover:text-white px-3 py-1.5 rounded-lg bg-slate-700 hover:bg-indigo-600 cursor-pointer transition-colors"
                  title="Download"
                >
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 3v12m-4-4l4 4 4-4" />
                  </svg>
                  Download
                </button>
                <button
                  onClick={() => setPreview(null)}
                  className="p-1.5 rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors"
                  title="Close"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>
            <div className="max-h-[75vh] overflow-auto bg-slate-950">
              <img
                src={resolveImageUrl(preview.url)}
                alt={preview.title || 'Chart'}
                className="w-full h-auto"
              />
            </div>
            {preview.description && (
              <div className="px-4 py-3 bg-slate-800 border-t border-slate-700 text-sm text-slate-300">
                {preview.description}
              </div>
            )}
          </div>
        </div>
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

export default Chatbot;
