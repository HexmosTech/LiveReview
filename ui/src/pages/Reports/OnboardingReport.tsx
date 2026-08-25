import React, { useCallback, useEffect, useRef, useState } from 'react';
import apiClient, { authFetch } from '../../api/apiClient';
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
type ExportPhase = 'idle' | 'starting' | 'running' | 'done' | 'error';

interface ExportState {
  jobId: string | null;
  phase: ExportPhase;
  current: number;
  total: number;
  label: string;
  error: string;
}

const emptyExportState = (): ExportState => ({ jobId: null, phase: 'idle', current: 0, total: 0, label: '', error: '' });

interface ExportStatusResponse {
  status: ExportPhase;
  current: number;
  total: number;
  label: string;
  error: string;
}

const OnboardingReport: React.FC = () => {
  const { currentOrg } = useOrgContext();
  const [sections, setSections] = useState<SectionMeta[]>([]);
  const [sectionData, setSectionData] = useState<Record<string, ChartResult[]>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [progress, setProgress] = useState({ current: 0, total: 0, label: '' });
  const [completed, setCompleted] = useState(false);
  const [activeSection, setActiveSection] = useState<string | null>(null);
  const [exports, setExports] = useState<Record<ExportFormat, ExportState>>({ pdf: emptyExportState(), html: emptyExportState() });
  const abortRef = useRef<AbortController | null>(null);
  // Tracks the *current* job id per format so a stale poll loop (e.g. left
  // over from a "Regenerate Report" that started a fresh export job) can
  // tell it's been superseded and stop updating state / stop re-polling.
  const activeJobIdRef = useRef<Record<ExportFormat, string | null>>({ pdf: null, html: null });
  // If the user clicks "Save as X" while that export is still running (it's
  // kicked off automatically as soon as report generation completes, so
  // this is only for someone who clicks before it's finished), the download
  // fires the moment the in-flight job reports done instead of requiring a
  // second click.
  const pendingDownloadRef = useRef<Record<ExportFormat, boolean>>({ pdf: false, html: false });

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
      setError(`Download failed: ${err?.message || 'Unknown error'}`);
    }
  }, []);

  // Polls an export job's progress until it's done or errors, updating the
  // per-format button state (chart N of M) as it goes. Generating a PDF/HTML
  // export re-runs every chart's SQL query and re-renders each chart to a
  // PNG server-side — the same slow pipeline the CLI tool uses — so this is
  // what replaces a plain "still loading?" spinner with real progress.
  const pollExportStatus = useCallback((format: ExportFormat, jobId: string) => {
    const tick = async () => {
      if (activeJobIdRef.current[format] !== jobId) return; // superseded by a newer job
      try {
        const res = await apiClient.get<ExportStatusResponse>(`/api/v1/reports/onboarding/export/${jobId}/status`);
        const data = ((res as any)?.data ?? res) as ExportStatusResponse;
        if (activeJobIdRef.current[format] !== jobId) return;

        const phase = data.status || 'running';
        setExports((prev) => ({
          ...prev,
          [format]: {
            ...prev[format],
            phase,
            current: data.current || 0,
            total: data.total || prev[format].total,
            label: data.label || '',
            error: data.error || '',
          },
        }));

        if (phase === 'done') {
          if (pendingDownloadRef.current[format]) {
            pendingDownloadRef.current[format] = false;
            fetchAndSaveExport(format, jobId);
          }
          return;
        }
        if (phase === 'error') return;
        setTimeout(tick, 900);
      } catch {
        if (activeJobIdRef.current[format] === jobId) setTimeout(tick, 1500);
      }
    };
    tick();
  }, [fetchAndSaveExport]);

  // Kicks off (or restarts) a PDF/HTML export job in the background.
  const startExport = useCallback(async (format: ExportFormat) => {
    setExports((prev) => ({ ...prev, [format]: { ...emptyExportState(), phase: 'starting' } }));
    try {
      const res = await apiClient.post<{ job_id: string; total: number }>(`/api/v1/reports/onboarding/export?format=${format}`, {});
      const data = (res as any)?.data ?? res;
      const jobId = data.job_id as string;
      activeJobIdRef.current[format] = jobId;
      setExports((prev) => ({ ...prev, [format]: { ...prev[format], jobId, phase: 'running', total: data.total || 0 } }));
      pollExportStatus(format, jobId);
    } catch (err: any) {
      activeJobIdRef.current[format] = null;
      setExports((prev) => ({ ...prev, [format]: { ...prev[format], phase: 'error', error: err?.message || 'Failed to start export' } }));
    }
  }, [pollExportStatus]);

  // As soon as the full report finishes generating, start rendering both
  // exports in the background so they're usually already done (or well
  // underway) by the time someone reaches for "Save as PDF/HTML" — the
  // "prep ahead of time" half of avoiding the download-button stall.
  useEffect(() => {
    if (!completed) return;
    pendingDownloadRef.current = { pdf: false, html: false };
    startExport('pdf');
    startExport('html');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [completed]);

  // Button click: download immediately if the prefetched export is ready,
  // otherwise mark it to auto-download the moment the in-flight (or
  // freshly (re)started) job finishes.
  const downloadReport = useCallback((format: ExportFormat) => {
    const state = exports[format];
    if (state.phase === 'done' && state.jobId) {
      fetchAndSaveExport(format, state.jobId);
      return;
    }
    pendingDownloadRef.current[format] = true;
    if (state.phase === 'idle' || state.phase === 'error') {
      startExport(format);
    }
  }, [exports, fetchAndSaveExport, startExport]);

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
                    onClick={() => downloadReport('html')}
                    disabled={downloading !== null}
                    className="px-4 py-2 text-sm rounded-lg border border-slate-600 text-slate-300 hover:bg-slate-800 disabled:opacity-50 transition-colors"
                  >
                    {downloading === 'html' ? 'Preparing...' : 'Save as HTML'}
                  </button>
                  <button
                    onClick={() => downloadReport('pdf')}
                    disabled={downloading !== null}
                    className="px-4 py-2 text-sm rounded-lg border border-slate-600 text-slate-300 hover:bg-slate-800 disabled:opacity-50 transition-colors"
                  >
                    {downloading === 'pdf' ? 'Preparing...' : 'Save as PDF'}
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
    </div>
  );
};

export default OnboardingReport;
