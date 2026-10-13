import React, { useEffect, useState } from 'react';
import { Alert, Button, Card, Spinner } from '../UIPrimitives';
import { getReviewPolicyLog, forceApproveReview, markReviewOutcome, PolicyLogResponse } from '../../api/riskPolicy';

const STAGE_LABEL: Record<string, string> = {
  eligibility: 'Eligibility',
  static: 'Static rules',
  risk: 'Risk score',
  llm: 'LLM classification',
  final: 'Final decision',
};

const verdictIcon = (v: string) => (v === 'pass' ? '✅' : v === 'fail' ? '❌' : '⏭️');

const AutoApprovePanel: React.FC<{ reviewId: number }> = ({ reviewId }) => {
  const [log, setLog] = useState<PolicyLogResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

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
          {log.stages.map((st) => (
            <div key={st.stage} className="flex items-start gap-3 text-sm">
              <span className="shrink-0">{verdictIcon(st.verdict)}</span>
              <span className="w-36 shrink-0 text-slate-300">{STAGE_LABEL[st.stage] || st.stage}</span>
              <span className={st.verdict === 'fail' ? 'text-red-300' : 'text-slate-400'}>
                {String((st.detail as any)?.reason ?? '')}
              </span>
            </div>
          ))}
        </div>
      </Card>

      {log.blocked && (
        <Card className="p-4">
          <h3 className="text-sm font-semibold text-white mb-2">Force approve</h3>
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
        <h3 className="text-sm font-semibold text-white mb-2">Mark production outcome</h3>
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
