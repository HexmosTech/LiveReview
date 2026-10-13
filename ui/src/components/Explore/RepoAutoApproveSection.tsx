import React, { useEffect, useState } from 'react';
import { Alert, Button, Select, Spinner, Tabs } from '../UIPrimitives';
import { useOrgContext } from '../../hooks/useOrgContext';
import {
  AutoApprovePolicy,
  AutoApprovePolicyUpsert,
  listAutoApprovePolicies,
  createAutoApprovePolicy,
  updateAutoApprovePolicy,
} from '../../api/riskPolicy';
import { CIRuleset } from '../../pages/CiRulesets/shared';
import apiClient from '../../api/apiClient';
import { StaticRulesFields, EligibilityFields, GeneralFields } from '../AutoApprove/PolicyFieldsEditor';

// Draft fields a repo can customize — everything on auto_approve_policies
// except the trained risk model itself (weights/category overrides stay
// org-wide only, see docs/risk-policy-engine.md §3/§9).
const draftFromPolicy = (p: AutoApprovePolicy | null): AutoApprovePolicyUpsert => ({
  min_author_level: p?.min_author_level ?? 1,
  eligibility_whitelist_user_ids: p?.eligibility_whitelist_user_ids ?? [],
  risk_threshold_percentile: p?.risk_threshold_percentile ?? 20,
  require_llm_confidence: p?.require_llm_confidence ?? 8,
  blocking_ruleset_ids: p?.blocking_ruleset_ids ?? [],
  landing_delay_seconds: p?.landing_delay_seconds ?? 0,
  daily_approve_cap: p?.daily_approve_cap,
  restrict_outcome_reporting_to_admins: p?.restrict_outcome_reporting_to_admins ?? false,
  force_approve_min_level: p?.force_approve_min_level ?? 3,
});

const RepoAutoApproveSection: React.FC<{ repositoryId: number }> = ({ repositoryId }) => {
  const { currentOrg, canManageCurrentOrg } = useOrgContext();
  const readOnly = !canManageCurrentOrg && currentOrg?.role !== 'owner';

  const [orgDefault, setOrgDefault] = useState<AutoApprovePolicy | null>(null);
  const [repoPolicy, setRepoPolicy] = useState<AutoApprovePolicy | null>(null);
  const [rulesets, setRulesets] = useState<CIRuleset[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');

  const [mode, setMode] = useState<'inherit' | 'custom' | 'always_approve' | 'never_approve'>('inherit');
  const [draft, setDraft] = useState<AutoApprovePolicyUpsert>(draftFromPolicy(null));
  const [subtab, setSubtab] = useState('static-rules');

  useEffect(() => {
    Promise.all([listAutoApprovePolicies(), apiClient.get<{ rows: CIRuleset[] }>('/ci-rulesets')])
      .then(([polRes, rsRes]) => {
        const org = polRes.rows.find((p) => p.repository_id == null) || null;
        const repo = polRes.rows.find((p) => p.repository_id === repositoryId) || null;
        setOrgDefault(org);
        setRepoPolicy(repo);
        setRulesets(rsRes.rows || []);
        setMode(repo?.mode ?? 'inherit');
        // A repo's "Customize" starts from its own saved values if it has any,
        // else from the org default — never from bare schema defaults — so
        // picking Customize can't silently loosen an org-wide setting.
        setDraft(draftFromPolicy(repo ?? org));
      })
      .catch((e) => setError(String(e?.message || e)))
      .finally(() => setLoading(false));
  }, [repositoryId]);

  const patch = (partial: Partial<AutoApprovePolicyUpsert>) => setDraft((d) => ({ ...d, ...partial }));

  const onModeChange = (next: typeof mode) => {
    // Switching into custom for the first time: seed from the org default so
    // the repo starts aligned with it rather than at hardcoded defaults.
    if (next === 'custom' && !repoPolicy) {
      setDraft(draftFromPolicy(orgDefault));
    }
    setMode(next);
  };

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      const body: AutoApprovePolicyUpsert = { mode, ...(mode === 'custom' ? draft : { risk_threshold_percentile: draft.risk_threshold_percentile }) };
      if (repoPolicy) {
        const res = await updateAutoApprovePolicy(repoPolicy.id, body);
        setRepoPolicy(res.policy);
      } else {
        const res = await createAutoApprovePolicy({ repository_id: repositoryId, ...body });
        setRepoPolicy(res.policy);
      }
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
    } catch (e: any) {
      setError(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="flex justify-center py-16"><Spinner /></div>;
  }

  const effective = repoPolicy && repoPolicy.mode !== 'inherit' ? repoPolicy : orgDefault;

  return (
    <div className="p-6 space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-white">Auto Approve</h2>
        <p className="text-sm text-slate-400">Override this repository's risk-based approval policy.</p>
      </div>

      {error && <Alert variant="error">{error}</Alert>}
      {readOnly && <Alert variant="warning">You need the owner role to edit this repository's policy.</Alert>}

      <div className="text-sm text-slate-300">
        Effective: <span className="text-white font-medium">{effective ? effective.mode : 'off'}</span>
        {!effective && ' (no organization policy configured — visit Settings → Auto Approve)'}
      </div>
      <p className="text-xs text-slate-500">
        The trained risk model (weights, category overrides, retraining) always stays the one org-wide classifier —
        only the threshold compared against its score, and everything below, can differ per repository.
      </p>

      <Select
        label="Mode"
        value={mode}
        onChange={(e) => onModeChange(e.target.value as any)}
        disabled={readOnly}
        options={[
          { value: 'inherit', label: 'Inherit organization default' },
          { value: 'custom', label: 'Custom' },
          { value: 'always_approve', label: 'Always approve' },
          { value: 'never_approve', label: 'Never approve' },
        ]}
      />

      {mode === 'custom' && (
        <>
          <Tabs
            className="mt-2"
            activeTab={subtab}
            onChange={(id) => setSubtab(id)}
            tabs={[
              { id: 'static-rules', label: 'Static Rules' },
              { id: 'eligibility', label: 'Eligibility' },
              { id: 'general', label: 'General' },
            ]}
          />
          {subtab === 'static-rules' && (
            <StaticRulesFields draft={draft} patch={patch} readOnly={readOnly} rulesets={rulesets} />
          )}
          {subtab === 'eligibility' && <EligibilityFields draft={draft} patch={patch} readOnly={readOnly} />}
          {subtab === 'general' && <GeneralFields draft={draft} patch={patch} readOnly={readOnly} />}
        </>
      )}

      <div className="flex items-center gap-2">
        <Button variant="primary" onClick={save} disabled={saving || readOnly}>
          {saving ? 'Saving…' : 'Save repository policy'}
        </Button>
        {saved && <span className="text-sm text-green-400">Saved</span>}
      </div>
    </div>
  );
};

export default RepoAutoApproveSection;
