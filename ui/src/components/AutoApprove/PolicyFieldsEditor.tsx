import React from 'react';
import { Input, Toggle } from '../UIPrimitives';
import { AutoApprovePolicyUpsert } from '../../api/riskPolicy';
import { CIRuleset } from '../../pages/CiRulesets/shared';

// Shared field groups for the auto-approve policy editor, used both by the
// org-wide Settings tab (AutoApprovePolicyTab.tsx) and a repository's
// "Customize" override (RepoAutoApproveSection.tsx), so the two never drift
// out of sync — only the save target (org default vs. repository_id) differs.

export interface PolicyFieldsProps {
  draft: AutoApprovePolicyUpsert;
  patch: (partial: Partial<AutoApprovePolicyUpsert>) => void;
  readOnly: boolean;
}

export const StaticRulesFields: React.FC<PolicyFieldsProps & { rulesets: CIRuleset[] }> = ({ draft, patch, readOnly, rulesets }) => (
  <div className="space-y-2">
    <p className="text-sm text-slate-400">
      Blocking rulesets always run (even under "always approve"). Manage them under{' '}
      <a href="#/ci-rulesets" className="text-blue-400">CI Rulesets</a>.
    </p>
    {rulesets.length === 0 && <p className="text-sm text-slate-500">No rulesets defined yet.</p>}
    {rulesets.map((r) => {
      const on = (draft.blocking_ruleset_ids || []).includes(r.id);
      return (
        <label key={r.id} className="flex items-center gap-3 text-sm text-slate-200">
          <input
            type="checkbox"
            checked={on}
            disabled={readOnly}
            onChange={(e) => {
              const ids = draft.blocking_ruleset_ids || [];
              patch({ blocking_ruleset_ids: e.target.checked ? [...ids, r.id] : ids.filter((x) => x !== r.id) });
            }}
          />
          {r.name}
        </label>
      );
    })}
  </div>
);

export const EligibilityFields: React.FC<PolicyFieldsProps> = ({ draft, patch, readOnly }) => (
  <div className="space-y-4">
    <Input
      type="number"
      label="Minimum author review level"
      value={String(draft.min_author_level ?? 1)}
      onChange={(e) => patch({ min_author_level: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
      helperText="Unmapped authors default to level 1; bots to level 0."
    />
    <Input
      type="number"
      label="Force-approve minimum level"
      value={String(draft.force_approve_min_level ?? 3)}
      onChange={(e) => patch({ force_approve_min_level: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
      helperText="Users at/above this level (or owners/admins) may force-approve a blocked review."
    />
  </div>
);

export const GeneralFields: React.FC<PolicyFieldsProps> = ({ draft, patch, readOnly }) => (
  <div className="space-y-4">
    <div>
      <label className="block text-sm font-medium text-slate-300 mb-1">Risk threshold percentile</label>
      <div className="flex items-center gap-3">
        <input
          type="range"
          min="0"
          max="100"
          className="flex-1"
          value={draft.risk_threshold_percentile ?? 20}
          onChange={(e) => patch({ risk_threshold_percentile: parseInt(e.target.value, 10) })}
          disabled={readOnly}
        />
        <span className="w-12 text-right text-sm text-white">P{draft.risk_threshold_percentile ?? 20}</span>
      </div>
    </div>
    <Input
      type="number"
      label="Required LLM confidence (0-10)"
      value={String(draft.require_llm_confidence ?? 8)}
      onChange={(e) => patch({ require_llm_confidence: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
    />
    <Input
      type="number"
      label="Landing delay (seconds)"
      value={String(draft.landing_delay_seconds ?? 0)}
      onChange={(e) => patch({ landing_delay_seconds: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
      helperText="Human-rejection window before the approval is posted."
    />
    <Input
      type="number"
      label="Daily approve cap (blank = unlimited)"
      value={draft.daily_approve_cap == null ? '' : String(draft.daily_approve_cap)}
      onChange={(e) => {
        const v = e.target.value === '' ? undefined : parseInt(e.target.value, 10);
        patch({ daily_approve_cap: v });
      }}
      disabled={readOnly}
    />
    <label className="flex items-center gap-3 text-sm text-slate-200">
      <Toggle
        checked={!!draft.restrict_outcome_reporting_to_admins}
        onChange={(v) => patch({ restrict_outcome_reporting_to_admins: v })}
        disabled={readOnly}
      />
      Restrict in-app outcome reporting to admins
    </label>
  </div>
);
