import React, { useEffect, useMemo, useState } from 'react';
import { Button, Card, Icons } from '../../components/UIPrimitives';
import promptsService from '../../services/prompts';
import type { CatalogEntry, VariablesResponse } from '../../types/prompts';
import PromptContextSelector, { PromptContext } from '../../components/PromptContextSelector';

const DEFAULT_PROMPT_KEY = 'code_review';

const PromptsPage: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [catalog, setCatalog] = useState<CatalogEntry[]>([]);
  const [promptKey, setPromptKey] = useState<string>(DEFAULT_PROMPT_KEY);
  const [ctx, setCtx] = useState<PromptContext>({});
  const [variables, setVariables] = useState<VariablesResponse | null>(null);
  const [styleGuide, setStyleGuide] = useState('');
  const [securityGuide, setSecurityGuide] = useState('');
  const [preview, setPreview] = useState<string>('');
  const [saving, setSaving] = useState<'style' | 'security' | null>(null);

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
    const res = await promptsService.getVariables(promptKey, ctx);
    setVariables(res);
    // Initialize editors from existing chunks (first chunk body if present)
    const sg = res.variables.find(v => v.name === 'style_guide');
    const sec = res.variables.find(v => v.name === 'security_guidelines');
    if (sg && sg.chunks && sg.chunks.length > 0) setStyleGuide(sg.chunks[0].body);
    if (sec && sec.chunks && sec.chunks.length > 0) setSecurityGuide(sec.chunks[0].body);
  };

  const refreshPreview = async () => {
    const res = await promptsService.renderPreview(promptKey, ctx);
    setPreview(res.prompt);
  };

  const loadAll = async () => {
    setLoading(true);
    try {
      await refreshCatalog();
      await refreshVariables();
      await refreshPreview();
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
        await refreshPreview();
      } finally {
        setLoading(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [promptKey, JSON.stringify(ctx)]);

  const saveChunk = async (kind: 'style' | 'security') => {
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
        // Context is inferred from headers + query by backend via app context resolution
        ai_connector_id: ctx.ai_connector_id,
        integration_token_id: ctx.integration_token_id,
        repository: ctx.repository,
      });
      await refreshVariables();
      await refreshPreview();
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
        <div className="mt-4">
          <PromptContextSelector value={ctx} onChange={setCtx} />
        </div>
      </Card>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <div className="flex items-center justify-between mb-2">
            <h3 className="text-slate-200 font-medium">Style guide</h3>
            {!hasStyleVar && (
              <span className="text-xs text-amber-400">Template doesn't declare this variable, but content will be ignored</span>
            )}
          </div>
          <textarea
            className="w-full h-48 bg-slate-800 border border-slate-700 rounded p-2 text-slate-200"
            placeholder="Add style guidance to help reviewers focus on consistency and clarity"
            value={styleGuide}
            onChange={(e) => setStyleGuide(e.target.value)}
          />
          <div className="mt-2 flex justify-end">
            <Button onClick={() => saveChunk('style')} isLoading={saving === 'style'}>
              Save
            </Button>
          </div>
        </Card>

        <Card>
          <div className="flex items-center justify-between mb-2">
            <h3 className="text-slate-200 font-medium">Security guidelines</h3>
            {!hasSecurityVar && (
              <span className="text-xs text-amber-400">Template doesn't declare this variable, but content will be ignored</span>
            )}
          </div>
          <textarea
            className="w-full h-48 bg-slate-800 border border-slate-700 rounded p-2 text-slate-200"
            placeholder="Add security checklists and policies to elevate review quality"
            value={securityGuide}
            onChange={(e) => setSecurityGuide(e.target.value)}
          />
          <div className="mt-2 flex justify-end">
            <Button onClick={() => saveChunk('security')} isLoading={saving === 'security'}>
              Save
            </Button>
          </div>
        </Card>
      </div>

      <Card>
        <div className="flex items-center justify-between">
          <h3 className="text-slate-200 font-medium">Render preview</h3>
          <Button variant="outline" onClick={refreshPreview}>
            <Icons.Refresh />
            Refresh Preview
          </Button>
        </div>
        <pre className="mt-3 p-3 bg-slate-900 border border-slate-700 rounded text-slate-200 whitespace-pre-wrap max-h-96 overflow-auto">
{preview}
        </pre>
      </Card>
    </div>
  );
};

export default PromptsPage;
