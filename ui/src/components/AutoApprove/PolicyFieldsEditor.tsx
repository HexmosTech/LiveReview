import React from 'react';
import { Input, Toggle } from '../UIPrimitives';
import { AutoApprovePolicyUpsert } from '../../api/riskPolicy';
import { CIRuleset } from '../../pages/CiRulesets/shared';
import { FieldLabel, InfoHint } from './InfoHint';
import { HELP } from './helpText';

// Shared field groups for the auto-approve policy editor, used both by the
// org-wide Settings tab (AutoApprovePolicyTab.tsx) and a repository's
// "Customize" override (RepoAutoApproveSection.tsx), so the two never drift
// out of sync — only the save target (org default vs. repository_id) differs.

// Named presets for the risk threshold; the stored value stays a plain percentile.
export const THRESHOLD_PRESETS = [
  { value: 20, name: 'Conservative' },
  { value: 50, name: 'Balanced' },
  { value: 80, name: 'Aggressive' },
];

export const thresholdLabel = (v: number) =>
  `${THRESHOLD_PRESETS.find((p) => p.value === v)?.name ?? 'Custom'} (safest ${v}%)`;

const ThresholdPicker: React.FC<{ value: number; onChange: (v: number) => void; readOnly: boolean }> = ({ value, onChange, readOnly }) => {
  const [custom, setCustom] = React.useState(!THRESHOLD_PRESETS.some((p) => p.value === value));
  const btn = (active: boolean) =>
    `px-3 py-1.5 rounded-md text-sm border ${active ? 'bg-blue-600 border-blue-500 text-white' : 'border-slate-600 text-slate-300 hover:bg-slate-700'} disabled:opacity-50`;
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {THRESHOLD_PRESETS.map((p) => (
          <button
            key={p.value}
            type="button"
            disabled={readOnly}
            className={btn(!custom && value === p.value)}
            onClick={() => { setCustom(false); onChange(p.value); }}
          >
            {p.name}
          </button>
        ))}
        <button type="button" disabled={readOnly} className={btn(custom)} onClick={() => setCustom(true)}>
          Custom
        </button>
      </div>
      <p className="text-sm text-slate-200 border-l-2 border-blue-500 pl-3">
        Approves the safest <span className="font-semibold text-white">{value}</span> out of 100 PRs.
      </p>
      {custom && (
        <div className="flex items-start gap-3">
          <div className="flex-1">
            <input
              type="range"
              min="0"
              max="100"
              list="threshold-presets"
              className="w-full"
              value={value}
              onChange={(e) => onChange(parseInt(e.target.value, 10))}
              disabled={readOnly}
            />
            <datalist id="threshold-presets">
              {THRESHOLD_PRESETS.map((p) => <option key={p.value} value={p.value} />)}
            </datalist>
            {/* Preset names under the slider, placed at their percent positions. */}
            <div className="relative h-4 text-[11px] text-slate-400">
              {THRESHOLD_PRESETS.map((p) => (
                <button
                  key={p.value}
                  type="button"
                  disabled={readOnly}
                  className="absolute -translate-x-1/2 hover:text-white"
                  style={{ left: `${p.value}%` }}
                  onClick={() => onChange(p.value)}
                >
                  {p.name}
                </button>
              ))}
            </div>
          </div>
          <span className="w-12 text-right text-sm text-white">P{value}</span>
        </div>
      )}
    </div>
  );
};

export interface PolicyFieldsProps {
  draft: AutoApprovePolicyUpsert;
  patch: (partial: Partial<AutoApprovePolicyUpsert>) => void;
  readOnly: boolean;
}

export const StaticRulesFields: React.FC<PolicyFieldsProps & { rulesets: CIRuleset[] }> = ({ draft, patch, readOnly, rulesets }) => (
  <div className="space-y-2">
    <p className="text-sm text-slate-400 flex items-start gap-1.5">
      <span>
        Blocking rulesets always run (even under "always approve"). Manage them under{' '}
        <a href="#/ci-rulesets" className="text-blue-400">CI Rulesets</a>.
      </span>
      <InfoHint text={HELP.staticRules} />
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
      label={<FieldLabel label="Minimum author review level" hint={HELP.minAuthorLevel} />}
      value={String(draft.min_author_level ?? 1)}
      onChange={(e) => patch({ min_author_level: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
      helperText="Unmapped authors default to level 1; bots to level 0."
    />
    <Input
      type="number"
      label={<FieldLabel label="Force-approve minimum level" hint={HELP.forceApproveMinLevel} />}
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
      <label className="block text-sm font-medium text-slate-300 mb-1">
        <FieldLabel label="Risk threshold" hint={HELP.riskThresholdPercentile} />
      </label>
      <ThresholdPicker value={draft.risk_threshold_percentile ?? 20} onChange={(v) => patch({ risk_threshold_percentile: v })} readOnly={readOnly} />
    </div>
    <Input
      type="number"
      label={<FieldLabel label="Required LLM confidence (0-10)" hint={HELP.requireLLMConfidence} />}
      value={String(draft.require_llm_confidence ?? 8)}
      onChange={(e) => patch({ require_llm_confidence: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
    />
    <Input
      type="number"
      label={<FieldLabel label="Landing delay (seconds)" hint={HELP.landingDelaySeconds} />}
      value={String(draft.landing_delay_seconds ?? 0)}
      onChange={(e) => patch({ landing_delay_seconds: parseInt(e.target.value, 10) || 0 })}
      disabled={readOnly}
      helperText="Human-rejection window before the approval is posted."
    />
    <Input
      type="number"
      label={<FieldLabel label="Daily approve cap (blank = unlimited)" hint={HELP.dailyApproveCap} />}
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
      <FieldLabel label="Restrict in-app outcome reporting to admins" hint={HELP.restrictOutcomeReporting} />
    </label>
  </div>
);
