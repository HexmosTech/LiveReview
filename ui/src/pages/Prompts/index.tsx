import React, { useEffect, useMemo, useState } from 'react';
import { Button, Card } from '../../components/UIPrimitives';
import promptsService from '../../services/prompts';
import type { CatalogEntry, VariablesResponse } from '../../types/prompts';
import LicenseUpgradeDialog from '../../components/License/LicenseUpgradeDialog';
import { useHasLicenseFor } from '../../hooks/useLicenseTier';

const DEFAULT_PROMPT_KEY = 'code_review';

const PromptsPage: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [catalog, setCatalog] = useState<CatalogEntry[]>([]);
  const [promptKey, setPromptKey] = useState<string>(DEFAULT_PROMPT_KEY);
  // Context filters removed for simplified global configuration (org-level only)
  const [variables, setVariables] = useState<VariablesResponse | null>(null);
  const [styleGuide, setStyleGuide] = useState('');
  const [securityGuide, setSecurityGuide] = useState('');
  const [saving, setSaving] = useState<'style' | 'security' | null>(null);
  const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
  const hasTeamLicense = useHasLicenseFor('team');

  const hasStyleVar = useMemo(() => variables?.variables.some(v => v.name === 'style_guide'), [variables]);
  const hasSecurityVar = useMemo(() => variables?.variables.some(v => v.name === 'security_guidelines'), [variables]);

  const refreshCatalog = async () => {
    const res = await promptsService.getCatalog();
    setCatalog(res.catalog);
    if (!res.catalog.find(c => c.prompt_key === promptKey)) {
      setPromptKey(res.catalog[0]?.prompt_key || DEFAULT_PROMPT_KEY);
    }
  };

  const refreshVariables = async () => {
    const res = await promptsService.getVariables(promptKey);
    setVariables(res);
    // Initialize editors from existing chunks (first chunk body if present)
    const sg = res.variables.find(v => v.name === 'style_guide');
    const sec = res.variables.find(v => v.name === 'security_guidelines');
    if (sg && sg.chunks && sg.chunks.length > 0) setStyleGuide(sg.chunks[0].body);
    if (sec && sec.chunks && sec.chunks.length > 0) setSecurityGuide(sec.chunks[0].body);
  };

  const loadAll = async () => {
    setLoading(true);
    try {
      await refreshCatalog();
      await refreshVariables();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    // When promptKey or context changes, refresh variables and preview
    (async () => {
      setLoading(true);
      try {
        await refreshVariables();
      } finally {
        setLoading(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [promptKey]);

  const saveChunk = async (kind: 'style' | 'security') => {
    // Check for Team license before saving (self-hosted mode only)
    if (!hasTeamLicense) {
      setShowUpgradeDialog(true);
      return;
    }
    
    try {
      setSaving(kind);
      const varName = kind === 'style' ? 'style_guide' : 'security_guidelines';
      const title = kind === 'style' ? 'Style Guide' : 'Security Guidelines';
      const body = kind === 'style' ? styleGuide : securityGuide;
      if (!body.trim()) {
        // No-op if empty; in MVP we don't delete existing chunk automatically
        return;
      }
      await promptsService.createChunk(promptKey, varName, {
        type: 'user',
        title,
        body,
      });
      await refreshVariables();
    } finally {
      setSaving(null);
    }
  };

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">

      <Card>
        <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
          <div className="flex items-center gap-3">
            <span className="text-slate-300 text-sm">Prompt</span>
            <select
              className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-200"
              value={promptKey}
              onChange={(e) => setPromptKey(e.target.value)}
            >
              {catalog.map((c) => (
                <option key={`${c.prompt_key}:${c.provider}`} value={c.prompt_key}>
                  {c.prompt_key}
                </option>
              ))}
            </select>
          </div>
          <div className="text-sm text-slate-400">{loading ? 'Loading…' : 'Ready'}</div>
        </div>
      </Card>

      {(!hasStyleVar && !hasSecurityVar) && (
        <Card>
          <div className="text-slate-300 text-sm">No customizations available for this template yet.</div>
        </Card>
      )}

      <div className="space-y-6">
        {hasStyleVar && (
          <Card>
            <div className="mb-3">
              <h3 className="text-slate-200 font-medium">Style guide</h3>
              <p className="text-xs text-slate-400 mt-1">Guidance appended to code review prompts to enforce consistency and clarity.</p>
            </div>
            <textarea
              className="w-full min-h-[12rem] bg-slate-800 border border-slate-700 rounded p-3 text-slate-200 resize-y"
              placeholder="Add style guidance to help reviewers focus on consistency and clarity"
              value={styleGuide}
              onChange={(e) => setStyleGuide(e.target.value)}
            />
            <div className="mt-3 flex justify-end">
              <Button onClick={() => saveChunk('style')} isLoading={saving === 'style'}>
                Save
              </Button>
            </div>
          </Card>
        )}

        {hasSecurityVar && (
          <Card>
            <div className="mb-3">
              <h3 className="text-slate-200 font-medium">Security guidelines</h3>
              <p className="text-xs text-slate-400 mt-1">Security checklist injected into prompts to improve vulnerability detection.</p>
            </div>
            <textarea
              className="w-full min-h-[12rem] bg-slate-800 border border-slate-700 rounded p-3 text-slate-200 resize-y"
              placeholder="Add security checklists and policies to elevate review quality"
              value={securityGuide}
              onChange={(e) => setSecurityGuide(e.target.value)}
            />
            <div className="mt-3 flex justify-end">
              <Button onClick={() => saveChunk('security')} isLoading={saving === 'security'}>
                Save
              </Button>
            </div>
          </Card>
        )}
      </div>

      {/* Render preview removed per simplification requirements */}

      {/* License Upgrade Dialog */}
      <LicenseUpgradeDialog
        open={showUpgradeDialog}
        onClose={() => setShowUpgradeDialog(false)}
        requiredTier="team"
        featureName="Prompt Customization"
        featureDescription="Customize your AI review prompts with style guides and security guidelines to get reviews tailored to your team's standards."
      />
    </div>
  );
};

export default PromptsPage;
