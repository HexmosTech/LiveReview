import React, { useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import apiClient from '../../api/apiClient';
import { PageHeader, Card, Button, Spinner } from '../../components/UIPrimitives';
import { useToast } from '../../components/NotificationToast';
import { CIRuleset, ProviderKey, PROVIDER_SHA_VARS, buildCurlSnippet, buildGithubActionsWorkflow, Breadcrumb } from './shared';

const PROVIDERS: ProviderKey[] = ['github', 'gitlab', 'bitbucket', 'azure', 'generic'];

const CiRulesetIntegration: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { showToast, ToastContainer } = useToast();

  const [ruleset, setRuleset] = useState<CIRuleset | null>(null);
  const [loading, setLoading] = useState(true);

  const providerParam = searchParams.get('provider') as ProviderKey | null;
  const provider: ProviderKey = providerParam && PROVIDERS.includes(providerParam) ? providerParam : 'github';
  const setProvider = (p: ProviderKey) => setSearchParams((prev) => {
    const next = new URLSearchParams(prev);
    next.set('provider', p);
    return next;
  }, { replace: true });

  useEffect(() => {
    if (!id) return;
    apiClient.get<{ ruleset: CIRuleset }>(`/ci-rulesets/${id}`)
      .then((res) => setRuleset(res.ruleset))
      .catch((err: any) => {
        showToast(err?.message || 'Failed to load ruleset', 'error');
        navigate('/ci-rulesets');
      })
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard?.writeText(text).then(
      () => showToast(`${label} copied to clipboard`, 'success'),
      () => showToast('Copy failed', 'error')
    );
  };

  if (loading || !ruleset) {
    return (
      <div className="container mx-auto px-4 py-6">
        <div className="flex justify-center py-24"><Spinner /></div>
      </div>
    );
  }

  const baseUrl = window.location.origin;
  const curlSnippet = buildCurlSnippet(baseUrl, ruleset.id, ruleset.org_id, provider);
  const workflowYaml = buildGithubActionsWorkflow(baseUrl, ruleset.id, ruleset.org_id);

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <ToastContainer />
      <Breadcrumb
        items={[
          { label: 'CI/CD Gates', to: '/ci-rulesets' },
          { label: ruleset.name, to: `/ci-rulesets/${ruleset.id}/edit` },
          { label: 'Integration code' },
        ]}
      />
      <PageHeader title={`Integration code: ${ruleset.name}`} description="Drop this into your CI/CD job. The exit code alone is the whole gate." />

      <Card>
        <div className="flex flex-wrap gap-2 mb-3">
          {PROVIDERS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setProvider(p)}
              className={`text-xs px-2.5 py-1.5 rounded-md border ${provider === p ? 'bg-blue-600 border-blue-500 text-white' : 'bg-slate-700/70 border-slate-600 text-slate-200 hover:bg-slate-600'}`}
            >
              {PROVIDER_SHA_VARS[p].label}
            </button>
          ))}
        </div>
        <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-200 overflow-x-auto">{curlSnippet}</pre>
        <div className="flex justify-end mt-2">
          <Button size="sm" variant="outline" onClick={() => copyToClipboard(curlSnippet, 'Snippet')}>Copy snippet</Button>
        </div>
        <p className="text-xs text-slate-500 mt-2">
          Set <code>LIVEREVIEW_API_KEY</code> as a CI secret (Settings → API Keys). Requires <code>jq</code> to be installed on the CI runner.
        </p>
      </Card>

      {provider === 'github' && (
        <Card title="Full GitHub Actions workflow" subtitle="Drop this at .github/workflows/livereview-gate.yml">
          <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-200 overflow-x-auto">{workflowYaml}</pre>
          <div className="flex justify-end mt-2">
            <Button size="sm" variant="outline" onClick={() => copyToClipboard(workflowYaml, 'Workflow')}>Copy workflow</Button>
          </div>
        </Card>
      )}

      <div className="flex justify-end">
        <Button variant="ghost" onClick={() => navigate('/ci-rulesets')}>Back to rulesets</Button>
      </div>
    </div>
  );
};

export default CiRulesetIntegration;
