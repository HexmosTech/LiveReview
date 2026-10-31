import React, { useEffect, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useOrgContext } from '../../hooks/useOrgContext';
import { Alert, Button, Card, Spinner, Tabs } from '../../components/UIPrimitives';
import {
  AutoApprovePolicy,
  AutoApprovePolicyUpsert,
  RiskModelResponse,
  getRiskModel,
  listAutoApprovePolicies,
  createAutoApprovePolicy,
  updateAutoApprovePolicy,
  putRiskModelOverrides,
  retrainRiskModel,
  simulatePolicy,
  SimulateRow,
  listReviewOutcomes,
  OutcomeRow,
} from '../../api/riskPolicy';
import { CIRuleset } from '../CiRulesets/shared';
import apiClient from '../../api/apiClient';
import { StaticRulesFields, EligibilityFields, GeneralFields } from '../../components/AutoApprove/PolicyFieldsEditor';
import { FieldLabel, InfoHint } from '../../components/AutoApprove/InfoHint';
import { HELP } from '../../components/AutoApprove/helpText';
import { TAXONOMY_CATEGORIES, categoryHint } from '../../components/AutoApprove/taxonomyHelp';

// All weight dials shown on the Risk Model tab, in the order they're
// rendered. Blast radius leads — how far a change's effects reach is the
// single strongest signal the model has, so it gets the highest default
// weight of any slider (see DefaultCategoryMultiplier in
// internal/riskscore/model.go) and is never hidden behind a collapsed
// section. The 10 finding-taxonomy categories follow (every finding the
// reviewer produces is classified into exactly one of these), then the two
// remaining structural signals (an any-category "critical" catch-all and
// raw diff size).
const CATEGORIES = [
  { key: 'blast_radius', label: 'Blast radius', hint: HELP.categoryBlastRadius },
  ...TAXONOMY_CATEGORIES.map((c) => ({ key: c.key, label: c.label, hint: categoryHint(c) })),
  { key: 'critical_findings', label: 'Critical findings (any category)', hint: HELP.categoryCriticalFindings },
  { key: 'diff_size', label: 'Diff size', hint: HELP.categoryDiffSize },
];

const DEFAULT_POLICY: AutoApprovePolicyUpsert = {
  mode: 'custom',
  min_author_level: 1,
  eligibility_whitelist_user_ids: [],
  risk_threshold_percentile: 20,
  require_llm_confidence: 8,
  blocking_ruleset_ids: [],
  landing_delay_seconds: 0,
  restrict_outcome_reporting_to_admins: false,
  force_approve_min_level: 3,
};

const AutoApprovePolicyTab: React.FC = () => {
  const { currentOrgId, currentOrg, canManageCurrentOrg } = useOrgContext();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');

  const [policyId, setPolicyId] = useState<number | undefined>();
  const [draft, setDraft] = useState<AutoApprovePolicyUpsert>(DEFAULT_POLICY);
  const [rulesets, setRulesets] = useState<CIRuleset[]>([]);
  const [model, setModel] = useState<RiskModelResponse | null>(null);
  // Local, instantly-updated slider positions, decoupled from `model` so a
  // drag never visibly jumps while a save is still in flight — previously
  // every tick re-fetched the whole model and rebound the slider to it,
  // which is what made dragging feel laggy/buggy.
  const [overrideDraft, setOverrideDraft] = useState<Record<string, number>>({});
  const saveTimers = useRef<Record<string, ReturnType<typeof setTimeout>>>({});
  const [outcomes, setOutcomes] = useState<OutcomeRow[]>([]);
  const location = useLocation();
  const [subtab, setSubtab] = useState(() => new URLSearchParams(window.location.search).get('subtab') || 'general');
  // Re-sync whenever ?subtab= changes, not just on first mount: the "Configure"
  // links on the review detail page's Auto-Approve tab all route to this same
  // /settings#auto-approve URL with a different ?subtab=, so React Router updates
  // location.search without remounting this component — a useState initializer
  // alone would miss every navigation after the first.
  useEffect(() => {
    setSubtab(new URLSearchParams(location.search).get('subtab') || 'general');
  }, [location.search]);
  const [simRows, setSimRows] = useState<SimulateRow[] | null>(null);

  const orgId = currentOrgId ?? 0;
  const readOnly = !canManageCurrentOrg && currentOrg?.role !== 'owner';

  useEffect(() => {
    if (!orgId) return;
    let cancelled = false;
    setLoading(true);
    Promise.all([
      listAutoApprovePolicies(),
      getRiskModel(),
      apiClient.get<{ rows: CIRuleset[] }>('/ci-rulesets'),
      listReviewOutcomes(orgId),
    ])
      .then(([polRes, modelRes, rsRes, outRes]) => {
        if (cancelled) return;
        const orgDefault = polRes.rows.find((p) => p.repository_id == null);
        if (orgDefault) {
          setPolicyId(orgDefault.id);
          setDraft({
            mode: orgDefault.mode,
            min_author_level: orgDefault.min_author_level,
            eligibility_whitelist_user_ids: orgDefault.eligibility_whitelist_user_ids,
            risk_threshold_percentile: orgDefault.risk_threshold_percentile,
            require_llm_confidence: orgDefault.require_llm_confidence,
            blocking_ruleset_ids: orgDefault.blocking_ruleset_ids,
            landing_delay_seconds: orgDefault.landing_delay_seconds,
            daily_approve_cap: orgDefault.daily_approve_cap,
            restrict_outcome_reporting_to_admins: orgDefault.restrict_outcome_reporting_to_admins,
            force_approve_min_level: orgDefault.force_approve_min_level,
          });
        }
        setModel(modelRes);
        setOverrideDraft(modelRes.overrides);
        setRulesets(rsRes.rows || []);
        setOutcomes(outRes.rows || []);
      })
      .catch((e) => setError(String(e?.message || e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [orgId]);

  // Flush any pending debounced slider saves if the tab unmounts mid-drag.
  useEffect(() => {
    const timers = saveTimers.current;
    return () => {
      Object.values(timers).forEach((t) => clearTimeout(t));
    };
  }, []);

  const patch = (partial: Partial<AutoApprovePolicyUpsert>) => setDraft((d) => ({ ...d, ...partial }));

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      if (policyId) {
        await updateAutoApprovePolicy(policyId, draft);
      } else {
        const res = await createAutoApprovePolicy(draft);
        setPolicyId(res.policy.id);
      }
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
    } catch (e: any) {
      setError(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  // Updates the slider instantly (local state only — no network round trip
  // on every tick of a drag), then saves after a short pause once the user
  // stops moving it. Dragging back and forth before the pause elapses just
  // resets the timer, so a single drag gesture sends one request, not one
  // per pixel.
  const onCategorySlide = (key: string, value: number) => {
    setOverrideDraft((d) => ({ ...d, [key]: value }));
    if (saveTimers.current[key]) clearTimeout(saveTimers.current[key]);
    saveTimers.current[key] = setTimeout(() => {
      delete saveTimers.current[key];
      putRiskModelOverrides({ [key]: value }).catch((e: any) => setError(String(e?.message || e)));
    }, 300);
  };

  const runRetrain = async () => {
    setError('');
    try {
      await retrainRiskModel();
      const m = await getRiskModel();
      setModel(m);
      setOverrideDraft(m.overrides);
    } catch (e: any) {
      setError(String(e?.message || e));
    }
  };

  const runSimulate = async () => {
    setError('');
    try {
      const res = await simulatePolicy({ policy: draft, limit: 20 });
      setSimRows(res.rows);
    } catch (e: any) {
      setError(String(e?.message || e));
    }
  };

  if (loading) {
    return (
      <div className="py-16 flex justify-center">
        <Spinner />
      </div>
    );
  }

  const subtabs = [
    { id: 'general', label: 'General' },
    { id: 'risk-model', label: 'Risk Model' },
    { id: 'static-rules', label: 'Static Rules' },
    { id: 'eligibility', label: 'Eligibility' },
    { id: 'feedback', label: 'Production Feedback' },
  ];

  return (
    <div className="space-y-6">
      <Card className="p-6">
        <div className="flex items-center justify-between mb-2">
          <div>
            <h2 className="text-lg font-semibold text-white">Auto Approve</h2>
            <p className="text-sm text-slate-400">Risk-based approval policy (docs/risk-policy-engine.md).</p>
          </div>
          <div className="flex items-center gap-2">
            {saved && <span className="text-sm text-green-400">Saved</span>}
            <Button variant="primary" onClick={save} disabled={saving || readOnly}>
              {saving ? 'Saving…' : 'Save policy'}
            </Button>
          </div>
        </div>
        {error && <Alert variant="error" className="mb-4">{error}</Alert>}
        {readOnly && <Alert variant="warning" className="mb-4">You need the owner role to edit this policy.</Alert>}

        <Tabs className="mb-6" activeTab={subtab} onChange={(id) => setSubtab(id)} tabs={subtabs} />

        {subtab === 'risk-model' && (
          <div className="space-y-4">
            {model && (
              <div className="text-sm text-slate-400 flex items-center gap-1.5">
                <span>
                  Active model: <span className="text-white">v{model.model.version}</span>
                  {model.model.is_seed
                    ? ' · common-sense seed (no outcomes yet)'
                    : ` · trained on ${model.model.training_samples} outcomes`}
                </span>
                <InfoHint text={HELP.activeModel} />
              </div>
            )}
            <p className="text-xs text-slate-500 flex items-start gap-1.5">
              <span>Category weight overrides</span>
              <InfoHint text={HELP.categoryOverrideIntro} />
            </p>
            <div className="space-y-3">
              {CATEGORIES.map((c) => (
                <div key={c.key}>
                  <label className="block text-sm font-medium text-slate-300 mb-1">
                    <FieldLabel label={`${c.label} weight`} hint={c.hint} />
                  </label>
                  <div className="flex items-center gap-3">
                    <input
                      type="range"
                      min="0"
                      max="10"
                      step="0.1"
                      className="flex-1 accent-blue-500"
                      value={overrideDraft[c.key] ?? 1}
                      onChange={(e) => onCategorySlide(c.key, parseFloat(e.target.value))}
                      disabled={readOnly}
                    />
                    <span className="w-12 text-right text-sm text-white tabular-nums">
                      {(overrideDraft[c.key] ?? 1).toFixed(1)}×
                    </span>
                  </div>
                </div>
              ))}
            </div>

            <div className="flex items-center gap-3 pt-2">
              <span className="inline-flex items-center gap-1.5">
                <Button variant="secondary" onClick={runRetrain} disabled={readOnly}>Refit model now</Button>
                <InfoHint text={HELP.retrainNow} />
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Button variant="secondary" onClick={runSimulate}>Simulate on recent reviews</Button>
                <InfoHint text={HELP.simulate} />
              </span>
            </div>
            {simRows && (
              <div className="space-y-1">
                <p className="text-sm text-slate-400">Simulation (last {simRows.length} completed reviews):</p>
                {simRows.map((r) => (
                  <div key={r.review_id} className="flex items-center gap-3 text-sm">
                    <span className="text-slate-300">Review #{r.review_id}</span>
                    <span className={r.would_approve ? 'text-green-400' : 'text-red-400'}>
                      {r.would_approve ? '✅ approve' : `❌ block (${r.blocked_at || 'policy'})`}
                    </span>
                    {typeof r.score === 'number' && <span className="text-slate-500">score {r.score.toFixed(2)}</span>}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {subtab === 'static-rules' && (
          <StaticRulesFields draft={draft} patch={patch} readOnly={readOnly} rulesets={rulesets} />
        )}

        {subtab === 'eligibility' && (
          <EligibilityFields draft={draft} patch={patch} readOnly={readOnly} />
        )}

        {subtab === 'general' && (
          <GeneralFields draft={draft} patch={patch} readOnly={readOnly} />
        )}

        {subtab === 'feedback' && (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1">
                <FieldLabel label="Outcome webhook" hint={HELP.outcomeWebhook} />
              </label>
              <code className="block bg-slate-700 rounded p-2 text-xs text-slate-200 break-all">
                POST {window.location.origin}/api/v1/orgs/{orgId}/review-outcomes
              </code>
              <p className="text-xs text-slate-500 mt-2 mb-1">Request body:</p>
              <pre className="bg-slate-700 rounded p-2 text-xs text-slate-200 overflow-x-auto">
{`{
  "review_id": 12345,
  "outcome_type": "revert" | "incident" | "confirmed_safe",
  "reason": "why this was reported",
  "source": "webhook" | "ci"   // optional
}`}
              </pre>
            </div>
            <div className="text-sm text-slate-300 flex items-center gap-1.5">
              <span>Outcomes reported in the last 30 days: <span className="text-white font-medium">{outcomes.length}</span></span>
              <InfoHint text={HELP.outcomeCounter} />
            </div>
            <div className="text-sm">
              <Link to="/reports/auto-approve" className="text-blue-400 hover:text-blue-300 underline">
                View full report: score distribution, decision rate, outcome calibration &rarr;
              </Link>
            </div>
          </div>
        )}
      </Card>
    </div>
  );
};

export default AutoApprovePolicyTab;
