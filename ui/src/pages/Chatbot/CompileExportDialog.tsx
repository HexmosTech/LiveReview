import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { LuX, LuArrowUp, LuArrowDown, LuSearch } from 'react-icons/lu';
import { authFetch } from '../../api/apiClient';
import { listConversationSummaries, type ChatSurface, type ConversationSummary } from '../../api/chatConversations';
import { formatUpdatedAt } from './ConversationSidebar';

type Step = 1 | 2 | 3;
type Format = 'pdf' | 'html' | 'json';

// Condenses a conversation's chart-type list (one DescribeChartShape()
// result per chart, from the backend) into a short "2 bar, 1 line" tally -
// shown at every step so selecting/reordering doesn't require opening each
// conversation to see what it contains.
function summarizeChartTypes(types?: string[]): string {
  if (!types || types.length === 0) return 'No charts';
  const counts = new Map<string, number>();
  for (const t of types) counts.set(t, (counts.get(t) ?? 0) + 1);
  return Array.from(counts.entries())
    .map(([type, count]) => `${count} ${type}`)
    .join(', ');
}

const ChartTally: React.FC<{ summary: ConversationSummary }> = ({ summary }) => (
  <div className="text-xs text-slate-500 mt-0.5">
    {summary.turnCount} turn{summary.turnCount === 1 ? '' : 's'} &middot; {summarizeChartTypes(summary.chartTypes)}
  </div>
);

// "Export As" (single conversation) lives in ChatConversation.tsx; this is
// the multi-conversation counterpart, opened from the sidebar so it's
// available on both /chat and /chat-debug for free. A 3-step wizard:
// select conversations, order them + name the compiled document, then pick
// a download format.
export const CompileExportDialog: React.FC<{ surface: ChatSurface; onClose: () => void }> = ({ surface, onClose }) => {
  const [step, setStep] = useState<Step>(1);
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [filter, setFilter] = useState('');
  const [title, setTitle] = useState('');
  const [subtitle, setSubtitle] = useState('');
  const [format, setFormat] = useState<Format>('pdf');
  const [submitting, setSubmitting] = useState(false);
  const [justDownloaded, setJustDownloaded] = useState(false);

  const { data: summaries = [], isLoading } = useQuery({
    queryKey: ['chat', 'conversationSummaries', surface],
    queryFn: () => listConversationSummaries(surface),
  });

  const byId = useMemo(() => new Map(summaries.map((s) => [s.id, s])), [summaries]);

  const filteredSummaries = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return summaries;
    return summaries.filter((s) => (s.title || 'New conversation').toLowerCase().includes(q));
  }, [summaries, filter]);

  const toggleSelected = (id: number) => {
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  const moveSelected = (index: number, direction: -1 | 1) => {
    setSelectedIds((prev) => {
      const next = [...prev];
      const target = index + direction;
      if (target < 0 || target >= next.length) return prev;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  };

  const handleDownload = async () => {
    setSubmitting(true);
    setJustDownloaded(false);
    try {
      const res = await authFetch('/api/v1/chat/export/compile', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversationIds: selectedIds, title, subtitle, format }),
      });
      if (!res.ok) throw new Error('compile export failed');
      const blob = await res.blob();
      const disposition = res.headers.get('Content-Disposition') || '';
      const match = disposition.match(/filename="?([^";]+)"?/);
      const filename = match?.[1] || `compiled-conversations.${format}`;
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = objectUrl;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(objectUrl);
      // Deliberately don't onClose() here - the user may want to download
      // the same compiled selection again in the other format without
      // redoing the whole wizard. Closing is left to an explicit action
      // (Cancel/X/Done).
      setJustDownloaded(true);
    } catch {
      alert('Could not compile this export. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4" onClick={onClose}>
      <div
        className="w-full max-w-xl max-h-[85vh] flex flex-col rounded-xl border border-slate-700 bg-slate-900 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex-none flex items-center justify-between px-5 py-4 border-b border-slate-800">
          <div>
            <h2 className="text-sm font-semibold text-slate-100">Compile conversations</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              Step {step} of 3 &middot; {step === 1 ? 'Select conversations' : step === 2 ? 'Order & title' : 'Download'}
            </p>
          </div>
          <button onClick={onClose} className="p-1.5 rounded-md text-slate-500 hover:text-slate-200 hover:bg-slate-800" aria-label="Close">
            <LuX className="w-4 h-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-4">
          {step === 1 && (
            <div className="space-y-3">
              <div className="relative">
                <LuSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500 pointer-events-none" />
                <input
                  type="text"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  placeholder="Filter conversations"
                  className="w-full rounded-md border border-slate-700/60 bg-slate-800/40 text-slate-200 placeholder-slate-500 text-sm pl-8 pr-2.5 py-1.5 outline-none focus:border-slate-600"
                />
              </div>
              {isLoading ? (
                <p className="text-sm text-slate-500 py-6 text-center">Loading conversations…</p>
              ) : filteredSummaries.length === 0 ? (
                <p className="text-sm text-slate-500 py-6 text-center">No conversations found.</p>
              ) : (
                <div className="space-y-1 max-h-80 overflow-y-auto">
                  {filteredSummaries.map((s) => {
                    const checked = selectedIds.includes(s.id);
                    return (
                      <label
                        key={s.id}
                        className={`flex items-start gap-2.5 px-2.5 py-2 rounded-md cursor-pointer ${checked ? 'bg-slate-800/80' : 'hover:bg-slate-800/50'}`}
                      >
                        <input type="checkbox" checked={checked} onChange={() => toggleSelected(s.id)} className="mt-1" />
                        <div className="min-w-0 flex-1">
                          <div className="text-sm text-slate-200 truncate">{s.title || 'New conversation'}</div>
                          <div className="text-xs text-slate-500">{formatUpdatedAt(s.updatedAt)}</div>
                          <ChartTally summary={s} />
                        </div>
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {step === 2 && (
            <div className="space-y-4">
              <div className="space-y-2">
                <input
                  type="text"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="Title (required)"
                  className="w-full rounded-md border border-slate-700/60 bg-slate-800/40 text-slate-200 placeholder-slate-500 text-sm px-2.5 py-1.5 outline-none focus:border-slate-600"
                />
                <input
                  type="text"
                  value={subtitle}
                  onChange={(e) => setSubtitle(e.target.value)}
                  placeholder="Subtitle (optional)"
                  className="w-full rounded-md border border-slate-700/60 bg-slate-800/40 text-slate-200 placeholder-slate-500 text-sm px-2.5 py-1.5 outline-none focus:border-slate-600"
                />
              </div>
              <div className="space-y-1">
                {selectedIds.map((id, index) => {
                  const s = byId.get(id);
                  if (!s) return null;
                  return (
                    <div key={id} className="flex items-start gap-2.5 px-2.5 py-2 rounded-md bg-slate-800/50">
                      <span className="text-xs text-slate-500 mt-1 w-4 text-right">{index + 1}</span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm text-slate-200 truncate">{s.title || 'New conversation'}</div>
                        <ChartTally summary={s} />
                      </div>
                      <div className="flex flex-col gap-0.5">
                        <button
                          onClick={() => moveSelected(index, -1)}
                          disabled={index === 0}
                          className="p-1 rounded text-slate-500 hover:text-slate-200 hover:bg-slate-700 disabled:opacity-30 disabled:hover:bg-transparent"
                          aria-label="Move up"
                        >
                          <LuArrowUp className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => moveSelected(index, 1)}
                          disabled={index === selectedIds.length - 1}
                          className="p-1 rounded text-slate-500 hover:text-slate-200 hover:bg-slate-700 disabled:opacity-30 disabled:hover:bg-transparent"
                          aria-label="Move down"
                        >
                          <LuArrowDown className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-4">
              <div className="flex gap-2">
                {(['pdf', 'html', ...(surface === 'chat_debug' ? ['json'] : [])] as Format[]).map((f) => (
                  <button
                    key={f}
                    onClick={() => {
                      setFormat(f);
                      setJustDownloaded(false);
                    }}
                    className={`flex-1 px-3 py-2 rounded-md border text-sm font-medium transition-colors ${
                      format === f
                        ? f === 'json'
                          ? 'border-amber-500 bg-amber-500/10 text-amber-300'
                          : 'border-blue-500 bg-blue-500/10 text-blue-300'
                        : 'border-slate-700/60 bg-slate-800/40 text-slate-300 hover:bg-slate-800'
                    }`}
                  >
                    {f.toUpperCase()}
                  </button>
                ))}
              </div>
              <div className="space-y-1">
                <p className="text-xs font-medium text-slate-400 uppercase tracking-wide">{title || 'Untitled export'}</p>
                {subtitle && <p className="text-xs text-slate-500 -mt-1">{subtitle}</p>}
                {selectedIds.map((id, index) => {
                  const s = byId.get(id);
                  if (!s) return null;
                  return (
                    <div key={id} className="flex items-start gap-2.5 px-2.5 py-2 rounded-md bg-slate-800/50">
                      <span className="text-xs text-slate-500 mt-1 w-4 text-right">{index + 1}</span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm text-slate-200 truncate">{s.title || 'New conversation'}</div>
                        <ChartTally summary={s} />
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        <div className="flex-none flex items-center justify-between px-5 py-4 border-t border-slate-800">
          <button
            onClick={() => (step === 1 ? onClose() : setStep((s) => (s - 1) as Step))}
            className="px-3 py-1.5 rounded-md text-sm text-slate-400 hover:text-slate-200 hover:bg-slate-800"
          >
            {step === 1 ? 'Cancel' : 'Back'}
          </button>
          {step < 3 ? (
            <button
              onClick={() => setStep((s) => (s + 1) as Step)}
              disabled={step === 1 ? selectedIds.length === 0 : title.trim() === ''}
              className="px-4 py-1.5 rounded-md bg-blue-600 hover:bg-blue-500 disabled:opacity-40 disabled:hover:bg-blue-600 text-white text-sm font-medium transition-colors"
            >
              Next
            </button>
          ) : (
            <div className="flex items-center gap-3">
              {justDownloaded && !submitting && (
                <span className="text-xs text-emerald-400">Downloaded &#10003;</span>
              )}
              <button
                onClick={handleDownload}
                disabled={submitting}
                className="px-4 py-1.5 rounded-md bg-blue-600 hover:bg-blue-500 disabled:opacity-40 text-white text-sm font-medium transition-colors"
              >
                {submitting ? 'Compiling…' : `Download ${format.toUpperCase()}`}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
