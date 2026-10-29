import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import { useOrgContext } from '../../hooks/useOrgContext';
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
  const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 2.2, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const };
  const base = 'w-5 h-5 rounded-full p-[3px] shrink-0';
  if (pass) {
    return (
      <svg className={`${base} bg-[#22C55E] text-white`} viewBox="0 0 24 24" role="img" aria-label="passed" {...stroke}>
        <path d="m5 12.5 4.5 4.5L19 7.5" />
      </svg>
    );
  }
  if (fail) {
    return (
      <svg className={`${base} bg-red-500 text-white`} viewBox="0 0 24 24" role="img" aria-label="failed" {...stroke}>
        <path d="M6 6l12 12M18 6 6 18" />
      </svg>
    );
  }
  return (
    <svg className={`${base} bg-amber-400 text-slate-900`} viewBox="0 0 24 24" role="img" aria-label="skipped" {...stroke}>
      <path d="M6 12h12" />
    </svg>
  );
};

// Policy sub-tab where each stage is configured (the final decision has none).
const STAGE_SETTINGS_TAB: Record<string, string> = {
  eligibility: 'eligibility',
  static: 'static-rules',
  risk: 'risk-model',
  llm: 'general',
};

const actionClass =
  'inline-flex items-center gap-1.5 rounded-md border border-slate-700 px-2 py-1 text-xs text-slate-300 ' +
  'hover:border-slate-500 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-400 transition-colors';

const GearIcon = () => (
  <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h0a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5h0a1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v0a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" />
  </svg>
);

const UserIcon = () => (
  <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <circle cx="12" cy="8" r="4" />
    <path d="M4 21a8 8 0 0 1 16 0" />
  </svg>
);

const Chevron: React.FC<{ open: boolean }> = ({ open }) => (
  <svg className={`w-3.5 h-3.5 transition-transform duration-200 ${open ? 'rotate-180' : ''}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="m6 9 6 6 6-6" />
  </svg>
);

// Every stage's detail blob carries more than the one-line reason already
// shown inline (risk's full feature-by-feature breakdown, llm's class +
// confidence, static's matched ruleset, etc.) — this renders whatever else
// is in it so "why" is fully inspectable, not just the summary line.
const StageDetail: React.FC<{ detail: Record<string, unknown> }> = ({ detail }) => {
  const features = Array.isArray(detail.features) ? (detail.features as { feature: string; value: unknown }[]) : null;
  const entries = Object.entries(detail).filter(([k]) => k !== 'reason' && k !== 'features');
  if (!features && entries.length === 0) return null;
  return (
    <div className="mt-2 ml-8 text-xs text-slate-400 space-y-1 border-l border-slate-700 pl-3">
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
  const { canManageCurrentOrg } = useOrgContext();
  const viewerId = useAppSelector((state) => state.Auth.user?.id);

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
        <div className="divide-y divide-slate-700/60">
          {log.stages.map((st) => {
            const canExpand = hasExpandableDetail(st.detail || {});
            const isOpen = expanded.has(st.stage);
            const settingsTab = STAGE_SETTINGS_TAB[st.stage];
            const isFinal = st.stage === 'final';
            const toggle = () =>
              setExpanded((prev) => {
                const next = new Set(prev);
                if (next.has(st.stage)) next.delete(st.stage);
                else next.add(st.stage);
                return next;
              });
            return (
              <div key={st.stage} className={`py-3 first:pt-1 last:pb-1 ${isFinal ? 'border-t border-slate-600' : ''}`}>
                <div
                  className={`grid grid-cols-[1.25rem_1fr] items-center gap-x-3 gap-y-2 text-sm md:grid-cols-[1.25rem_11rem_1fr_auto] ${canExpand ? 'cursor-pointer' : ''}`}
                  onClick={(e) => {
                    // Links, the Details button and the info icon handle their own clicks.
                    if (canExpand && !(e.target as HTMLElement).closest('a,button,[aria-label="Help"]')) toggle();
                  }}
                >
                  <VerdictIcon verdict={st.verdict} />
                  <span className={`inline-flex items-center gap-1.5 ${isFinal ? 'font-bold text-white' : 'font-medium text-slate-200'}`}>
                    {STAGE_LABEL[st.stage] || st.stage}
                    {STAGE_HINT[st.stage] && <InfoHint text={STAGE_HINT[st.stage]} />}
                  </span>
                  <span className={`col-start-2 md:col-start-auto ${st.verdict === 'fail' ? 'text-red-300' : 'text-slate-400'} ${isFinal ? 'font-semibold' : ''}`}>
                    {String((st.detail as any)?.reason ?? '')}
                  </span>
                  <div className="col-start-2 flex flex-wrap items-center gap-2 md:col-start-auto md:justify-end">
                    {canExpand ? (
                      <button
                        type="button"
                        onClick={toggle}
                        aria-expanded={isOpen}
                        className={`${actionClass} w-[5.5rem] justify-between border-transparent text-slate-400`}
                      >
                        Details <Chevron open={isOpen} />
                      </button>
                    ) : (
                      <span className="hidden w-[5.5rem] md:block" aria-hidden="true" />
                    )}
                    {canManageCurrentOrg && st.stage === 'eligibility' && viewerId && (
                      <Link to={`/settings/users/edit/${viewerId}`} className={actionClass} title="Open your user settings (review level)">
                        <UserIcon /> Your level
                      </Link>
                    )}
                    {canManageCurrentOrg && settingsTab && (
                      <Link to={`/settings?subtab=${settingsTab}#auto-approve`} className={actionClass} title="Open this stage in the org policy settings">
                        <GearIcon /> Configure
                      </Link>
                    )}
                  </div>
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
