import React, { useEffect, useState } from 'react';
import { Alert, Button, Card, Spinner } from '../UIPrimitives';
import { getReviewPolicyLog, forceApproveReview, markReviewOutcome, PolicyLogResponse } from '../../api/riskPolicy';
import { InfoHint } from '../AutoApprove/InfoHint';
import { HELP } from '../AutoApprove/helpText';

const STAGE_LABEL: Record<string, string> = {
  eligibility: 'Eligibility',
  static: 'Static rules',
  risk: 'Risk score',
  llm: 'LLM classification',
  final: 'Final decision',
};

const STAGE_HINT: Record<string, string> = {
  eligibility: HELP.stageEligibility,
  static: HELP.stageStatic,
  risk: HELP.stageRisk,
  llm: HELP.stageLLM,
  final: HELP.stageFinal,
};

const VerdictIcon: React.FC<{ verdict: string }> = ({ verdict }) => {
  const pass = verdict === 'pass' || verdict === 'approved';
  const fail = verdict === 'fail' || verdict === 'blocked';
  // 16px icon nudged to the cap height of the 14px row text, so icon, label and reason share one baseline.
  const cls = 'w-4 h-4 mt-[3px] shrink-0';
  const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const };
  if (pass) {
    return (
      <svg className={`${cls} text-green-400`} viewBox="0 0 24 24" role="img" aria-label="passed" {...stroke}>
        <circle cx="12" cy="12" r="9" />
        <path d="m8.5 12.4 2.4 2.4 4.6-5" />
      </svg>
    );
  }
  if (fail) {
    return (
      <svg className={`${cls} text-red-400`} viewBox="0 0 24 24" role="img" aria-label="failed" {...stroke}>
        <circle cx="12" cy="12" r="9" />
        <path d="m9 9 6 6M15 9l-6 6" />
      </svg>
    );
  }
  return (
    <svg className={`${cls} text-slate-400`} viewBox="0 0 24 24" role="img" aria-label="skipped" {...stroke}>
      <circle cx="12" cy="12" r="9" />
      <path d="M8 12h8" />
    </svg>
  );
};

// Every stage's detail blob carries more than the one-line reason already
// shown inline (risk's full feature-by-feature breakdown, llm's class +
// confidence, static's matched ruleset, etc.) — this renders whatever else
// is in it so "why" is fully inspectable, not just the summary line.
const StageDetail: React.FC<{ detail: Record<string, unknown> }> = ({ detail }) => {
  const features = Array.isArray(detail.features) ? (detail.features as { feature: string; value: unknown }[]) : null;
  const entries = Object.entries(detail).filter(([k]) => k !== 'reason' && k !== 'features');
  if (!features && entries.length === 0) return null;
  return (
    <div className="mt-1 ml-9 text-xs text-slate-400 space-y-1 border-l border-slate-700 pl-3">
      {entries.map(([k, v]) => (
        <div key={k}>
          <span className="text-slate-500">{k.replace(/_/g, ' ')}:</span>{' '}
          <span className="text-slate-300">{typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}</span>
        </div>
      ))}
      {features && features.length > 0 && (
        <table className="mt-1">
          <thead>
            <tr className="text-slate-500">
              <th className="text-left font-normal pr-4">Feature</th>
              <th className="text-left font-normal">Value</th>
            </tr>
          </thead>
          <tbody>
            {features.map((f) => (
              <tr key={f.feature}>
                <td className="pr-4 text-slate-400">{f.feature}</td>
                <td className="text-slate-300">{typeof f.value === 'number' ? f.value.toFixed(3) : String(f.value)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
};

const hasExpandableDetail = (detail: Record<string, unknown>) => Object.keys(detail).some((k) => k !== 'reason');

const AutoApprovePanel: React.FC<{ reviewId: number }> = ({ reviewId }) => {
  const [log, setLog] = useState<PolicyLogResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const [forceReason, setForceReason] = useState('');
  const [forceBusy, setForceBusy] = useState(false);
  const [forceDone, setForceDone] = useState(false);

  const [outcomeType, setOutcomeType] = useState<'revert' | 'incident' | 'confirmed_safe'>('revert');
  const [outcomeReason, setOutcomeReason] = useState('');
  const [outcomeBusy, setOutcomeBusy] = useState(false);
  const [outcomeDone, setOutcomeDone] = useState(false);

  const load = () => {
    setLoading(true);
    setError('');
    getReviewPolicyLog(reviewId)
      .then(setLog)
      .catch((e) => setError(String(e?.message || e)))
      .finally(() => setLoading(false));
  };

  useEffect(load, [reviewId]);

  if (loading) {
    return (
      <div className="py-16 flex justify-center">
        <Spinner />
      </div>
    );
  }

  if (error) {
    return <Alert variant="error">{error}</Alert>;
  }

  if (!log || !log.stages.length) {
    return (
      <div className="bg-slate-800 rounded-lg p-4 border border-slate-700">
        <p className="text-sm text-slate-400">No policy evaluation for this review.</p>
      </div>
    );
  }

  const forceApprove = async () => {
    setForceBusy(true);
    setError('');
    try {
      await forceApproveReview(reviewId, forceReason || 'Manual override');
      setForceDone(true);
    } catch (e: any) {
      setError(String(e?.message || e));
    } finally {
      setForceBusy(false);
    }
  };

  const mark = async () => {
    setOutcomeBusy(true);
    setError('');
    try {
      await markReviewOutcome(reviewId, outcomeType, outcomeReason);
      setOutcomeDone(true);
    } catch (e: any) {
      setError(String(e?.message || e));
    } finally {
      setOutcomeBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {error && <Alert variant="error">{error}</Alert>}

      <Card className="p-4">
        <div className="space-y-2">
          {log.stages.map((st) => {
            const canExpand = hasExpandableDetail(st.detail || {});
            const isOpen = expanded.has(st.stage);
            return (
              <div key={st.stage}>
                <div
                  className={`flex items-start gap-3 text-sm ${canExpand ? 'cursor-pointer' : ''}`}
                  onClick={() => {
                    if (!canExpand) return;
                    setExpanded((prev) => {
                      const next = new Set(prev);
                      if (next.has(st.stage)) next.delete(st.stage);
                      else next.add(st.stage);
                      return next;
                    });
                  }}
                >
                  <span className="shrink-0"><VerdictIcon verdict={st.verdict} /></span>
                  <span className="w-36 shrink-0 text-slate-300 inline-flex items-center gap-1.5">
                    {STAGE_LABEL[st.stage] || st.stage}
                    {STAGE_HINT[st.stage] && <InfoHint text={STAGE_HINT[st.stage]} />}
                  </span>
                  <span className={st.verdict === 'fail' ? 'text-red-300' : 'text-slate-400'}>
                    {String((st.detail as any)?.reason ?? '')}
                  </span>
                  {canExpand && <span className="text-slate-500 text-xs shrink-0">{isOpen ? '▾ hide details' : '▸ show details'}</span>}
                </div>
                {isOpen && <StageDetail detail={st.detail || {}} />}
              </div>
            );
          })}
        </div>
      </Card>

      {log.blocked && (
        <Card className="p-4">
          <h3 className="text-sm font-semibold text-white mb-2 inline-flex items-center gap-1.5">
            Force approve
            <InfoHint text={HELP.forceApproveSection} />
          </h3>
          <p className="text-xs text-slate-400 mb-3">
            Requires review level ≥ {log.force_approve_min_level} or owner/admin. Recorded in the audit log.
          </p>
          <textarea
            className="w-full rounded-lg border border-slate-600 bg-slate-700 text-white px-3 py-2 text-sm mb-2"
            rows={2}
            placeholder="Reason for overriding the policy"
            value={forceReason}
            onChange={(e) => setForceReason(e.target.value)}
          />
          <Button variant="primary" onClick={forceApprove} disabled={forceBusy}>
            {forceDone ? 'Force-approved' : 'Force approve'}
          </Button>
        </Card>
      )}

      <Card className="p-4">
        <h3 className="text-sm font-semibold text-white mb-2 inline-flex items-center gap-1.5">
          Mark production outcome
          <InfoHint text={HELP.markOutcomeSection} />
        </h3>
        <div className="flex flex-wrap items-center gap-2 mb-2">
          {(['revert', 'incident', 'confirmed_safe'] as const).map((t) => (
            <label key={t} className="flex items-center gap-1 text-xs text-slate-300">
              <input type="radio" checked={outcomeType === t} onChange={() => setOutcomeType(t)} />
              {t.replace('_', ' ')}
            </label>
          ))}
        </div>
        <textarea
          className="w-full rounded-lg border border-slate-600 bg-slate-700 text-white px-3 py-2 text-sm mb-2"
          rows={2}
          placeholder="What happened in production?"
          value={outcomeReason}
          onChange={(e) => setOutcomeReason(e.target.value)}
        />
        <Button variant="secondary" onClick={mark} disabled={outcomeBusy || !outcomeReason.trim()}>
          {outcomeDone ? 'Recorded' : 'Record outcome'}
        </Button>
      </Card>
    </div>
  );
};

export default AutoApprovePanel;
