import React, { useEffect, useState } from 'react';
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

const CATEGORIES = [
  { key: 'blast_radius', label: 'Blast radius', hint: HELP.categoryBlastRadius },
  { key: 'security_findings', label: 'Security findings', hint: HELP.categorySecurityFindings },
  { key: 'critical_findings', label: 'Critical findings', hint: HELP.categoryCriticalFindings },
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
  const [outcomes, setOutcomes] = useState<OutcomeRow[]>([]);
  const [subtab, setSubtab] = useState('risk-model');
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
        setRulesets(rsRes.rows || []);
        setOutcomes(outRes.rows || []);
      })
      .catch((e) => setError(String(e?.message || e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [orgId]);

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

  const saveOverrides = async (key: string, value: number) => {
    setError('');
    try {
      await putRiskModelOverrides({ [key]: value });
      const m = await getRiskModel();
      setModel(m);
    } catch (e: any) {
      setError(String(e?.message || e));
    }
  };

  const runRetrain = async () => {
    setError('');
    try {
      await retrainRiskModel();
      const m = await getRiskModel();
      setModel(m);
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
    { id: 'risk-model', label: 'Risk Model' },
    { id: 'static-rules', label: 'Static Rules' },
    { id: 'eligibility', label: 'Eligibility' },
    { id: 'general', label: 'General' },
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
            {CATEGORIES.map((c) => (
              <div key={c.key}>
                <label className="block text-sm font-medium text-slate-300 mb-1">
                  <FieldLabel label={`${c.label} weight override`} hint={c.hint} />
                </label>
                <div className="flex items-center gap-3">
                  <input
                    type="range"
                    min="0"
                    max="10"
                    step="0.1"
                    className="flex-1"
                    value={model?.overrides[c.key] ?? 1}
                    onChange={(e) => saveOverrides(c.key, parseFloat(e.target.value))}
                    disabled={readOnly}
                  />
                  <span className="w-10 text-right text-sm text-white">{model?.overrides[c.key] ?? 1}×</span>
                </div>
              </div>
            ))}
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
              <p className="text-xs text-slate-500 mt-1">Body: {'{ review_id, outcome_type: "revert" | "incident" | "confirmed_safe", reason, source?: "webhook" | "ci" }'}</p>
            </div>
            <div className="text-sm text-slate-300 flex items-center gap-1.5">
              <span>Outcomes reported in the last 30 days: <span className="text-white font-medium">{outcomes.length}</span></span>
              <InfoHint text={HELP.outcomeCounter} />
            </div>
          </div>
        )}
      </Card>
    </div>
  );
};

export default AutoApprovePolicyTab;
