import React, { useEffect, useState } from 'react';
import ReactECharts from 'echarts-for-react/lib/core';
import { LIVEREVIEW_ECHARTS_THEME, ECHARTS_ANIMATION_DEFAULTS, LR_ECHARTS_CORE } from '../../components/Dashboard/widgets/echartsTheme';
import { useChartResize } from '../../components/Dashboard/widgets/useChartResize';
import { Alert, Card, Spinner } from '../../components/UIPrimitives';
import { useOrgContext } from '../../hooks/useOrgContext';
import { AutoApproveInsights, getAutoApproveInsights } from '../../api/riskPolicy';

// The "is this behaving reasonably" view the auto-approve policy engine
// never had: where an org's real review scores land relative to the
// threshold, how many real decisions were approved/blocked/overridden, and
// -- once outcomes are reported -- how often an approval turned out wrong.
// See docs/risk-policy-engine.md and the Settings -> Auto Approve Policy ->
// Production Feedback tab, which links here.
const WINDOWS: Array<7 | 30 | 90> = [7, 30, 90];

const ScoreHistogram: React.FC<{ insights: AutoApproveInsights }> = ({ insights }) => {
  const { containerRef, chartRef } = useChartResize();
  const dist = insights.score_distribution;

  if (dist.total_scored === 0) {
    return <p className="text-slate-400 text-xs">No reviews scored in this window yet.</p>;
  }

  const cutoffBucketIndex = dist.buckets.findIndex((b) => dist.cutoff >= b.range_start && dist.cutoff < b.range_end);
  const option = {
    ...ECHARTS_ANIMATION_DEFAULTS,
    grid: { left: 0, right: 12, top: 8, bottom: 24, containLabel: true },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: dist.buckets.map((b) => b.range_start.toFixed(2)),
      axisLabel: { interval: 1, fontSize: 10 },
    },
    yAxis: { type: 'value' },
    series: [
      {
        name: 'Reviews scored',
        type: 'bar',
        data: dist.buckets.map((b, i) => ({
          value: b.count,
          itemStyle: i === cutoffBucketIndex ? { color: '#f59e0b' } : { color: '#3b82f6' },
        })),
      },
    ],
  };

  return (
    <div className="space-y-2">
      <p className="text-xs text-slate-400">
        Current cutoff for a P{dist.percentile} threshold: <span className="text-amber-400 font-medium">{dist.cutoff.toFixed(3)}</span>
        {' '}(highlighted bar). {dist.total_scored} reviews scored in this window.
      </p>
      <div className="w-full rounded bg-slate-900/50 border border-slate-700 p-2" style={{ height: 180 }}>
        <div ref={containerRef} className="w-full h-full">
          <ReactECharts ref={chartRef} echarts={LR_ECHARTS_CORE} option={option} theme={LIVEREVIEW_ECHARTS_THEME} style={{ height: '100%', width: '100%' }} notMerge />
        </div>
      </div>
    </div>
  );
};

const DecisionsSection: React.FC<{ insights: AutoApproveInsights }> = ({ insights }) => {
  const { containerRef, chartRef } = useChartResize();
  const d = insights.decisions;
  const totalBlocked = d.total_decided - d.approved;

  const kpis: Array<[string, string | number]> = [
    ['Total decided', d.total_decided],
    ['Approved', d.approved],
    ['Blocked', totalBlocked],
    ['Overridden', d.overridden],
    ['Override rate', `${(d.override_rate * 100).toFixed(1)}%`],
  ];

  const option = {
    ...ECHARTS_ANIMATION_DEFAULTS,
    grid: { left: 0, right: 12, top: 8, bottom: 34, containLabel: true },
    tooltip: { trigger: 'axis' },
    legend: { bottom: 0, textStyle: { fontSize: 11 } },
    xAxis: { type: 'category', data: d.trend.map((r) => r.day), boundaryGap: false },
    yAxis: { type: 'value' },
    series: [
      { name: 'Approved', type: 'line', data: d.trend.map((r) => r.approved), itemStyle: { color: '#22c55e' }, areaStyle: { opacity: 0.15 }, smooth: true, showSymbol: false },
      { name: 'Blocked', type: 'line', data: d.trend.map((r) => r.blocked), itemStyle: { color: '#ef4444' }, areaStyle: { opacity: 0.15 }, smooth: true, showSymbol: false },
    ],
  };

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-2">
        {kpis.map(([label, value]) => (
          <div key={label} className="rounded bg-slate-900/50 border border-slate-700 p-2 text-center">
            <div className="text-lg font-semibold text-white">{value}</div>
            <div className="text-[11px] text-slate-400">{label}</div>
          </div>
        ))}
      </div>
      {d.total_decided > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs text-slate-400">
          <div>Blocked at eligibility: <span className="text-slate-200">{d.blocked_at_eligibility}</span></div>
          <div>Blocked at static rules: <span className="text-slate-200">{d.blocked_at_static}</span></div>
          <div>Blocked at risk score: <span className="text-slate-200">{d.blocked_at_risk}</span></div>
          <div>Blocked at LLM gate: <span className="text-slate-200">{d.blocked_at_llm}</span></div>
        </div>
      )}
      {d.trend.length > 0 ? (
        <div className="w-full rounded bg-slate-900/50 border border-slate-700 p-2" style={{ height: 180 }}>
          <div ref={containerRef} className="w-full h-full">
            <ReactECharts ref={chartRef} echarts={LR_ECHARTS_CORE} option={option} theme={LIVEREVIEW_ECHARTS_THEME} style={{ height: '100%', width: '100%' }} notMerge />
          </div>
        </div>
      ) : (
        <p className="text-slate-400 text-xs">No decisions in this window yet.</p>
      )}
    </div>
  );
};

const CalibrationSection: React.FC<{ insights: AutoApproveInsights }> = ({ insights }) => {
  const cal = insights.calibration;
  const outcomeTypes: Array<CalibrationCellOutcome> = ['revert', 'incident', 'confirmed_safe'];

  const countFor = (decision: 'approved' | 'blocked', outcomeType: string) =>
    cal.cells.find((c) => c.decision === decision && c.outcome_type === outcomeType)?.count ?? 0;

  return (
    <div className="space-y-3">
      {cal.low_volume_warning && (
        <Alert variant="warning">
          Fewer than 10 outcomes reported in this window ({cal.total_outcomes} so far) &mdash; these rates aren&apos;t reliable yet.
        </Alert>
      )}
      {cal.false_approve_rate !== undefined && (
        <p className="text-sm text-slate-300">
          Of approved reviews with a reported outcome, <span className="text-amber-400 font-medium">{(cal.false_approve_rate * 100).toFixed(1)}%</span> were later reported as an incident or revert.
        </p>
      )}
      <table className="w-full text-xs text-left">
        <thead>
          <tr className="text-slate-400 border-b border-slate-700">
            <th className="py-1 pr-2">Policy decision</th>
            {outcomeTypes.map((t) => (
              <th key={t} className="py-1 pr-2 capitalize">{t.replace('_', ' ')}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {(['approved', 'blocked'] as const).map((decision) => (
            <tr key={decision} className="border-b border-slate-800">
              <td className="py-1 pr-2 capitalize text-slate-200">{decision}</td>
              {outcomeTypes.map((t) => (
                <td key={t} className="py-1 pr-2 text-slate-300">{countFor(decision, t)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {cal.total_outcomes === 0 && (
        <p className="text-slate-400 text-xs">No outcomes reported in this window yet.</p>
      )}
    </div>
  );
};

type CalibrationCellOutcome = 'revert' | 'incident' | 'confirmed_safe';

const AutoApproveReport: React.FC = () => {
  const { canManageCurrentOrg, currentOrg } = useOrgContext();
  const [windowDays, setWindowDays] = useState<7 | 30 | 90>(30);
  const [insights, setInsights] = useState<AutoApproveInsights | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!currentOrg) return;
    setLoading(true);
    setError(null);
    getAutoApproveInsights(windowDays)
      .then((res) => setInsights(res.insights))
      .catch((err) => setError(err?.message || 'Failed to load auto-approve insights'))
      .finally(() => setLoading(false));
  }, [windowDays, currentOrg]);

  if (!canManageCurrentOrg) {
    return (
      <div className="p-6">
        <Alert variant="warning">Only org owners and admins can view auto-approve insights.</Alert>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4 max-w-5xl">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div>
          <h1 className="text-xl font-semibold text-white">Auto Approve</h1>
          <p className="text-sm text-slate-400">
            How the risk-based auto-approve policy is deciding on your org&apos;s real pull requests.
          </p>
        </div>
        <div className="flex gap-1">
          {WINDOWS.map((w) => (
            <button
              key={w}
              onClick={() => setWindowDays(w)}
              className={`px-3 py-1 rounded text-sm ${windowDays === w ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
            >
              {w}d
            </button>
          ))}
        </div>
      </div>

      {loading && <Spinner />}
      {error && <Alert variant="error">{error}</Alert>}

      {insights && (
        <>
          <Card title="Score distribution" subtitle="Where real reviews land relative to the current threshold">
            <ScoreHistogram insights={insights} />
          </Card>
          <Card title="Decisions" subtitle="Approved / blocked / overridden counts and trend">
            <DecisionsSection insights={insights} />
          </Card>
          <Card title="Calibration" subtitle="Reported outcomes vs. what the policy decided">
            <CalibrationSection insights={insights} />
          </Card>
        </>
      )}
    </div>
  );
};

export default AutoApproveReport;
