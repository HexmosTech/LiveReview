import React, { useCallback, useEffect, useRef, useState } from 'react';
import apiClient, { authFetch } from '../../api/apiClient';
import { ChartStats } from '../../api/chatbot';
import { useOrgContext } from '../../hooks/useOrgContext';
import { InteractiveChart } from '../Chatbot/InteractiveChart';

interface ChartResult {
  id: string;
  section: string;
  section_label: string;
  title: string;
  description: string;
  query_summary: string;
  chart_type: string;
  granularity: string;
  time_range: string;
  vega_spec: Record<string, unknown>;
  row_count: number;
  stats?: ChartStats;
  error?: string;
}

interface SectionMeta {
  id: string;
  label: string;
}

interface SectionResponse {
  section: string;
  section_label: string;
  charts: ChartResult[];
  total_charts: number;
}

type ExportFormat = 'pdf' | 'html';
type ExportPhase = 'starting' | 'running' | 'done' | 'error';

interface ExportModalState {
  open: boolean;
  format: ExportFormat;
  phase: ExportPhase;
  jobId: string | null;
  current: number;
  total: number;
  label: string;
  error: string;
}

interface ExportStatusResponse {
  status: ExportPhase;
  current: number;
  total: number;
  label: string;
  error: string;
}

const closedModal: ExportModalState = { open: false, format: 'pdf', phase: 'starting', jobId: null, current: 0, total: 0, label: '', error: '' };

const StatChip: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="rounded-md border border-slate-700 bg-slate-800/60 px-2.5 py-1.5 min-w-0">
    <div className="text-[11px] text-slate-500">{label}</div>
    <div className="text-xs text-slate-300 font-medium break-words">{value}</div>
  </div>
);

const fmtNum = (v: number) => v.toLocaleString();

const ChartStatsDisplay: React.FC<{ stats: ChartStats }> = ({ stats }) => {
  const finest = (s: { day?: unknown; week?: unknown; month?: unknown }) => s.day ?? s.week ?? s.month;

  switch (stats.kind) {
    case 'trend': {
      const s = finest(stats) as { total: number; avgPerPeriod: number; peak: { value: number; date: string }; low: { value: number; date: string }; trendPct: number | null } | undefined;
      if (!s) return null;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
          <StatChip label="Total" value={fmtNum(s.total)} />
          <StatChip label="Avg per period" value={fmtNum(s.avgPerPeriod)} />
          <StatChip label="Peak" value={`${fmtNum(s.peak.value)} (${s.peak.date})`} />
          <StatChip label="Low" value={`${fmtNum(s.low.value)} (${s.low.date})`} />
          <StatChip label="Trend" value={s.trendPct === null ? 'n/a' : `${s.trendPct >= 0 ? '+' : ''}${s.trendPct}%`} />
        </div>
      );
    }
    case 'multi_series_trend': {
      const s = finest(stats) as { total: number; seriesCount: number; topSeries: { label: string; value: number } } | undefined;
      if (!s) return null;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <StatChip label="Total" value={fmtNum(s.total)} />
          <StatChip label="Series" value={String(s.seriesCount)} />
          <StatChip label="Top series" value={`${s.topSeries.label} (${fmtNum(s.topSeries.value)})`} />
        </div>
      );
    }
    case 'category': {
      const s = stats.stats;
      return (
        <div className="grid grid-cols-2 gap-2">
          <StatChip label="Highest" value={`${s.highest.label} (${fmtNum(s.highest.value)})`} />
          <StatChip label="Lowest" value={`${s.lowest.label} (${fmtNum(s.lowest.value)})`} />
          <StatChip label="Top 3" value={s.top3.map((x) => `${x.label} (${fmtNum(x.value)})`).join(', ')} />
          <StatChip label="Bottom 3" value={s.bottom3.map((x) => `${x.label} (${fmtNum(x.value)})`).join(', ')} />
        </div>
      );
    }
    case 'band': {
      const s = stats.stats;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <StatChip label="Total active" value={fmtNum(s.totalActive)} />
          <StatChip label="Largest" value={`${s.largest.label} (${fmtNum(s.largest.value)})`} />
          <StatChip label="Largest share" value={`${s.largestSharePct}%`} />
        </div>
      );
    }
    case 'heatmap': {
      const s = stats.stats;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          <StatChip label="Total" value={fmtNum(s.total)} />
          <StatChip label="Active days" value={String(s.activeDays)} />
          <StatChip label="Avg on active days" value={fmtNum(s.avgOnActiveDays)} />
          <StatChip label="Busiest" value={`${s.busiest.date} (${fmtNum(s.busiest.value)})`} />
        </div>
      );
    }
    case 'slope': {
      const s = stats.stats;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          <StatChip label="Entities" value={String(s.entityCount)} />
          <StatChip label="Gained / Lost / Flat" value={`${s.gained} / ${s.lost} / ${s.flat}`} />
          <StatChip label="Biggest gain" value={`${s.biggestGain.label} (+${fmtNum(s.biggestGain.delta)})`} />
          <StatChip label="Biggest loss" value={`${s.biggestLoss.label} (${fmtNum(s.biggestLoss.delta)})`} />
        </div>
      );
    }
    case 'generic': {
      const s = stats.stats;
      return (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          <StatChip label="Total" value={fmtNum(s.total)} />
          <StatChip label="Count" value={String(s.count)} />
          <StatChip label="Highest" value={`${s.highest.label} (${fmtNum(s.highest.value)})`} />
          <StatChip label="Lowest" value={`${s.lowest.label} (${fmtNum(s.lowest.value)})`} />
        </div>
      );
    }
    default:
      return null;
  }
};

const OnboardingReport: React.FC = () => {
  const { currentOrg } = useOrgContext();
  const [sections, setSections] = useState<SectionMeta[]>([]);
  const [sectionData, setSectionData] = useState<Record<string, ChartResult[]>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [progress, setProgress] = useState({ current: 0, total: 0, label: '' });
  const [completed, setCompleted] = useState(false);
  const [activeSection, setActiveSection] = useState<string | null>(null);
  const [exportModal, setExportModal] = useState<ExportModalState>(closedModal);
  const abortRef = useRef<AbortController | null>(null);
  const activeJobIdRef = useRef<string | null>(null);

  useEffect(() => {
    apiClient
      .get<{ sections: SectionMeta[]; total: number }>('/api/v1/reports/onboarding/sections')
      .then((res) => {
        const data = (res as any)?.data ?? res;
        setSections(data.sections || []);
      })
      .catch(() => setError('Failed to load report sections.'));
  }, []);

  const generateReport = useCallback(async () => {
    if (sections.length === 0) return;

    setLoading(true);
    setError('');
    setCompleted(false);
    setSectionData({});
    setProgress({ current: 0, total: sections.length, label: 'Preparing...' });

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      for (let i = 0; i < sections.length; i++) {
        if (controller.signal.aborted) break;

        const section = sections[i];
        setProgress({ current: i, total: sections.length, label: section.label });

        const res = await apiClient.get<SectionResponse>(
          `/api/v1/reports/onboarding/charts/${section.id}`,
          { signal: controller.signal } as any,
        );

        const data = (res as any)?.data ?? res;
        setSectionData((prev) => ({ ...prev, [section.id]: data.charts || [] }));
        setProgress({ current: i + 1, total: sections.length, label: section.label });
      }

      if (!controller.signal.aborted) {
        setCompleted(true);
        setActiveSection(sections[0]?.id || null);
      }
    } catch (err: any) {
      if (err?.name !== 'AbortError') {
        setError(`Generation failed: ${err?.message || 'Unknown error'}`);
      }
    } finally {
      setLoading(false);
      abortRef.current = null;
    }
  }, [sections]);

  const cancelGeneration = useCallback(() => {
    abortRef.current?.abort();
    setLoading(false);
  }, []);

  // Fetches a finished export job's bytes and triggers the browser's save-file flow.
  const fetchAndSaveExport = useCallback(async (format: ExportFormat, jobId: string) => {
    try {
      const response = await authFetch(`/api/v1/reports/onboarding/export/${jobId}/file`);
      if (!response.ok) {
        const text = await response.text().catch(() => '');
        throw new Error(text || `Download failed with status ${response.status}`);
      }
      const blob = await response.blob();
      const objectUrl = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      const contentDisposition = response.headers.get('Content-Disposition') || '';
      const match = contentDisposition.match(/filename="?([^";]+)"?/i);
      a.href = objectUrl;
      a.download = match?.[1] || `livereview-onboarding-report.${format}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(objectUrl);
    } catch (err: any) {
      setExportModal((prev) => ({ ...prev, phase: 'error', error: `Download failed: ${err?.message || 'Unknown error'}` }));
    }
  }, []);

  // Kicks off an export job and opens the progress modal.
  const startExport = useCallback(async (format: ExportFormat) => {
    setExportModal({ open: true, format, phase: 'starting', jobId: null, current: 0, total: 0, label: '', error: '' });
    try {
      const res = await apiClient.post<{ job_id: string; total: number }>(`/api/v1/reports/onboarding/export?format=${format}`, {});
      const data = (res as any)?.data ?? res;
      const jobId = data.job_id as string;
      activeJobIdRef.current = jobId;
      setExportModal((prev) => ({ ...prev, jobId, phase: 'running', total: data.total || 0 }));

      // Poll until done
      const poll = async () => {
        while (activeJobIdRef.current === jobId) {
          try {
            const statusRes = await apiClient.get<ExportStatusResponse>(`/api/v1/reports/onboarding/export/${jobId}/status`);
            const statusData = ((statusRes as any)?.data ?? statusRes) as ExportStatusResponse;
            if (activeJobIdRef.current !== jobId) return;

            const phase = statusData.status || 'running';
            setExportModal((prev) => ({ ...prev, phase, current: statusData.current || 0, total: statusData.total || prev.total, label: statusData.label || '', error: statusData.error || '' }));

            if (phase === 'done') {
              fetchAndSaveExport(format, jobId);
              return;
            }
            if (phase === 'error') return;
            await new Promise((r) => setTimeout(r, 900));
          } catch {
            await new Promise((r) => setTimeout(r, 1500));
          }
        }
      };
      poll();
    } catch (err: any) {
      activeJobIdRef.current = null;
      setExportModal((prev) => ({ ...prev, phase: 'error', error: err?.message || 'Failed to start export' }));
    }
  }, [fetchAndSaveExport]);

  const closeExportModal = useCallback(() => {
    activeJobIdRef.current = null;
    setExportModal(closedModal);
  }, []);

  const totalCharts = Object.values(sectionData).reduce((sum, charts) => sum + charts.length, 0);
  const errorCharts = Object.values(sectionData).reduce(
    (sum, charts) => sum + charts.filter((c) => c.error).length,
    0,
  );

  const activeCharts = activeSection ? sectionData[activeSection] || [] : [];

  return (
    <div className="min-h-screen bg-slate-950 text-slate-200">
      <div className="border-b border-slate-800 bg-slate-900/80 backdrop-blur-sm sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-slate-100">Onboarding Report</h1>
              <p className="text-sm text-slate-400 mt-1">
                {currentOrg?.name || 'Your organization'} &middot; LiveReview adoption metrics
              </p>
            </div>
            <div className="flex items-center gap-3">
              {loading && (
                <button
                  onClick={cancelGeneration}
                  className="px-4 py-2 text-sm rounded-lg border border-slate-600 text-slate-300 hover:bg-slate-800 transition-colors"
                >
                  Cancel
                </button>
              )}
              {!loading && completed && (
                <>
                  <button
                    onClick={() => startExport('html')}
                    className="px-4 py-2 text-sm rounded-lg border border-slate-600 text-slate-300 hover:bg-slate-800 transition-colors"
                  >
                    Save as HTML
                  </button>
                  <button
                    onClick={() => startExport('pdf')}
                    className="px-4 py-2 text-sm rounded-lg border border-slate-600 text-slate-300 hover:bg-slate-800 transition-colors"
                  >
                    Save as PDF
                  </button>
                </>
              )}
              {!loading && (
                <button
                  onClick={generateReport}
                  disabled={sections.length === 0}
                  className="px-5 py-2.5 text-sm font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  {completed ? 'Regenerate Report' : 'Generate Report'}
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        {!loading && !completed && Object.keys(sectionData).length === 0 && (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="w-16 h-16 rounded-full bg-slate-800 flex items-center justify-center mb-6">
              <svg className="w-8 h-8 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            </div>
            <h2 className="text-xl font-semibold text-slate-100 mb-2">Generate Your Onboarding Report</h2>
            <p className="text-slate-400 text-center max-w-md mb-2">
              Get a comprehensive view of your organization's LiveReview adoption across{' '}
              {sections.length} sections with charts covering adoption, repositories, engineers,
              quality, cost, and engagement.
            </p>
            <p className="text-slate-500 text-center max-w-md mb-8 text-xs">
              You can generate this report at any time, as many times as you want.
            </p>
            <button
              onClick={generateReport}
              disabled={sections.length === 0}
              className="px-6 py-3 text-base font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50 transition-colors"
            >
              Generate Report
            </button>
          </div>
        )}

        {(loading || (completed && Object.keys(sectionData).length > 0)) && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm text-slate-400">
                {loading ? (
                  <>
                    <span className="inline-block animate-pulse mr-1.5">&#9679;</span>
                    Generating: {progress.label}
                  </>
                ) : completed ? (
                  <span className="text-emerald-400">
                    &#10003; Report complete &mdash; {totalCharts} charts generated
                    {errorCharts > 0 && (
                      <span className="text-amber-400 ml-2">({errorCharts} with errors)</span>
                    )}
                  </span>
                ) : null}
              </span>
              <span className="text-sm font-mono text-slate-500">
                {progress.current}/{progress.total}
              </span>
            </div>
            <div className="w-full h-2 bg-slate-800 rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-500 ease-out ${
                  completed ? 'bg-emerald-500' : 'bg-blue-500'
                }`}
                style={{ width: `${progress.total > 0 ? (progress.current / progress.total) * 100 : 0}%` }}
              />
            </div>
            <div className="flex flex-wrap gap-2 mt-3">
              {sections.map((s) => {
                const done = !!sectionData[s.id];
                const active = loading && progress.label === s.label;
                return (
                  <span
                    key={s.id}
                    className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium transition-colors ${
                      active
                        ? 'bg-blue-600/20 text-blue-300 ring-1 ring-blue-500/30'
                        : done
                          ? 'bg-emerald-600/15 text-emerald-400'
                          : 'bg-slate-800 text-slate-500'
                    }`}
                  >
                    {active && <span className="inline-block animate-pulse">&#9679;</span>}
                    {done && !active && <span>&#10003;</span>}
                    {s.label}
                    {done && sectionData[s.id] && (
                      <span className="text-[10px] opacity-60 ml-0.5">
                        ({sectionData[s.id].length})
                      </span>
                    )}
                  </span>
                );
              })}
            </div>
          </div>
        )}

        {loading && (
          <div className="mb-6 px-4 py-3 rounded-lg bg-amber-900/20 border border-amber-700/30 text-amber-300 text-sm flex items-center gap-2">
            <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            Please stay on this page while the report is generating. Navigating away will cancel the process.
          </div>
        )}

        {error && (
          <div className="mb-6 px-4 py-3 rounded-lg bg-red-900/20 border border-red-700/30 text-red-300 text-sm">
            {error}
          </div>
        )}

        {completed && !loading && (
          <div className="mb-6 px-4 py-3 rounded-lg bg-emerald-900/20 border border-emerald-700/30 text-emerald-300 text-sm flex items-center gap-2">
            <svg className="w-5 h-5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Onboarding report generated successfully! {totalCharts} charts across {sections.length} sections are ready below.
          </div>
        )}

        {Object.keys(sectionData).length > 0 && (
          <div className="flex gap-6">
            <nav className="hidden lg:block w-56 shrink-0 sticky top-24 self-start">
              <ul className="space-y-1">
                {sections.map((s) => {
                  const done = !!sectionData[s.id];
                  if (!done) return null;
                  return (
                    <li key={s.id}>
                      <button
                        onClick={() => setActiveSection(s.id)}
                        className={`w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                          activeSection === s.id
                            ? 'bg-slate-800 text-slate-100 font-medium'
                            : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
                        }`}
                      >
                        {s.label}
                        <span className="ml-1.5 text-[11px] text-slate-600">
                          {sectionData[s.id]?.length || 0}
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </nav>

            <div className="flex-1 min-w-0">
              {activeSection && activeCharts.length > 0 && (
                <div>
                  <h2 className="text-lg font-semibold text-slate-100 mb-4">
                    {sections.find((s) => s.id === activeSection)?.label}
                  </h2>
                  <div className="space-y-6">
                    {activeCharts.map((chart) => (
                      <div
                        key={chart.id}
                        className="rounded-lg border border-slate-800 bg-slate-900/50 p-4"
                      >
                        <div className="flex items-start justify-between mb-2">
                          <div>
                            <h3 className="text-sm font-medium text-slate-100">{chart.title}</h3>
                            <p className="text-xs text-slate-500 mt-0.5">{chart.description}</p>
                          </div>
                          <div className="flex items-center gap-2 ml-4 shrink-0">
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">
                              {chart.chart_type}
                            </span>
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">
                              {chart.granularity}
                            </span>
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-500">
                              {chart.row_count} rows
                            </span>
                          </div>
                        </div>
                        {chart.error ? (
                          <div className="px-3 py-2 rounded bg-red-900/20 border border-red-800/30 text-red-400 text-xs">
                            {chart.error}
                          </div>
                        ) : (
                          <InteractiveChart spec={chart.vega_spec} />
                        )}
                        {chart.stats && (
                          <div className="mt-3">
                            <ChartStatsDisplay stats={chart.stats} />
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {!activeSection && completed && (
                <p className="text-slate-500 text-sm">Select a section from the sidebar to view charts.</p>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Export progress modal */}
      {exportModal.open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="bg-slate-900 border border-slate-700 rounded-xl shadow-2xl w-full max-w-md p-6">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-slate-100">
                Exporting {exportModal.format.toUpperCase()}
              </h3>
              {(exportModal.phase === 'done' || exportModal.phase === 'error') && (
                <button onClick={closeExportModal} className="text-slate-400 hover:text-slate-200 transition-colors">
                  <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              )}
            </div>

            {exportModal.phase === 'starting' && (
              <div className="flex items-center gap-3 py-6">
                <div className="animate-spin w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full" />
                <span className="text-slate-300 text-sm">Preparing export...</span>
              </div>
            )}

            {exportModal.phase === 'running' && (
              <div className="py-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-sm text-slate-300">
                    <span className="inline-block animate-pulse mr-1.5 text-blue-400">&#9679;</span>
                    Rendering chart {exportModal.current} of {exportModal.total}
                  </span>
                  <span className="text-xs font-mono text-slate-500">
                    {exportModal.total > 0 ? Math.round((exportModal.current / exportModal.total) * 100) : 0}%
                  </span>
                </div>
                <div className="w-full h-2 bg-slate-800 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-blue-500 rounded-full transition-all duration-500 ease-out"
                    style={{ width: `${exportModal.total > 0 ? (exportModal.current / exportModal.total) * 100 : 0}%` }}
                  />
                </div>
                {exportModal.label && (
                  <p className="text-xs text-slate-500 mt-2 truncate">{exportModal.label}</p>
                )}
                <p className="text-xs text-slate-600 mt-3">This may take a minute for large reports. Please keep this tab open.</p>
              </div>
            )}

            {exportModal.phase === 'done' && (
              <div className="flex flex-col items-center py-6">
                <div className="w-12 h-12 rounded-full bg-emerald-600/20 flex items-center justify-center mb-3">
                  <svg className="w-6 h-6 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                  </svg>
                </div>
                <p className="text-slate-200 text-sm font-medium">Download started</p>
                <p className="text-slate-500 text-xs mt-1">Your {exportModal.format.toUpperCase()} is ready.</p>
                <button onClick={closeExportModal} className="mt-4 px-4 py-2 text-sm rounded-lg bg-slate-800 text-slate-300 hover:bg-slate-700 transition-colors">
                  Close
                </button>
              </div>
            )}

            {exportModal.phase === 'error' && (
              <div className="flex flex-col items-center py-6">
                <div className="w-12 h-12 rounded-full bg-red-600/20 flex items-center justify-center mb-3">
                  <svg className="w-6 h-6 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
                  </svg>
                </div>
                <p className="text-red-300 text-sm font-medium">Export failed</p>
                <p className="text-slate-500 text-xs mt-1 text-center">{exportModal.error || 'Something went wrong.'}</p>
                <div className="flex gap-2 mt-4">
                  <button onClick={closeExportModal} className="px-4 py-2 text-sm rounded-lg bg-slate-800 text-slate-300 hover:bg-slate-700 transition-colors">
                    Close
                  </button>
                  <button onClick={() => startExport(exportModal.format)} className="px-4 py-2 text-sm rounded-lg bg-blue-600 text-white hover:bg-blue-500 transition-colors">
                    Retry
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default OnboardingReport;
