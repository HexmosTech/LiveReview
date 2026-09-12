import React, { useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import apiClient from '../../api/apiClient';
import toast from 'react-hot-toast';
import {
    PageHeader,
    Card,
    Button,
    Spinner,
    Icons,
} from '../../components/UIPrimitives';
import {
    Table,
    TableHead,
    TableHeaderCell,
    TableBody,
    TableRow,
    TableCell,
} from '../../components/DataTable/SimpleTable';
import {
    CIRuleset,
    ProviderKey,
    PROVIDER_SHA_VARS,
    PROVIDER_SECRET_INSTRUCTIONS,
    GATE_SECRETS,
    buildCurlSnippet,
    buildGithubActionsWorkflow,
    buildGitlabCIWorkflow,
    Breadcrumb,
} from './shared';

const PROVIDERS: ProviderKey[] = [
    'curl',
    'github',
    'gitlab',
    'bitbucket',
    'azure',
    'generic',
];

const CopyIconButton: React.FC<{
    text: string;
    label: string;
    onCopy: (text: string, label: string) => void;
}> = ({ text, label, onCopy }) => (
    <Button
        size="sm"
        variant="ghost"
        title={`Copy ${label}`}
        onClick={() => onCopy(text, label)}
        className="!px-1.5 !py-1"
    >
        <Icons.Copy />
    </Button>
);

const CiRulesetIntegration: React.FC = () => {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();

    const [ruleset, setRuleset] = useState<CIRuleset | null>(null);
    const [loading, setLoading] = useState(true);
    const [instructionsOpen, setInstructionsOpen] = useState(false);

    const providerParam = searchParams.get('provider') as ProviderKey | null;
    const provider: ProviderKey =
        providerParam && PROVIDERS.includes(providerParam)
            ? providerParam
            : 'github';
    const setProvider = (p: ProviderKey) =>
        setSearchParams(
            (prev) => {
                const next = new URLSearchParams(prev);
                next.set('provider', p);
                return next;
            },
            { replace: true }
        );

    useEffect(() => {
        if (!id) return;
        apiClient
            .get<{ ruleset: CIRuleset }>(`/ci-rulesets/${id}`)
            .then((res) => setRuleset(res.ruleset))
            .catch((err: any) => {
                toast.error(err?.message || 'Failed to load ruleset');
                navigate('/ci-rulesets');
            })
            .finally(() => setLoading(false));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);

    const copyToClipboard = (text: string, label: string) => {
        navigator.clipboard?.writeText(text).then(
            () => toast.success(`${label} copied to clipboard`),
            () => toast.error('Copy failed')
        );
    };

    if (loading || !ruleset) {
        return (
            <div className="container mx-auto px-4 py-6">
                <div className="flex justify-center py-24">
                    <Spinner />
                </div>
            </div>
        );
    }

    const baseUrl = window.location.origin;
    const curlSnippet = buildCurlSnippet(
        baseUrl,
        ruleset.id,
        ruleset.org_id,
        provider
    );
    const workflowYaml = buildGithubActionsWorkflow(
        baseUrl,
        ruleset.id,
        ruleset.org_id
    );
    const gitlabYaml = buildGitlabCIWorkflow(
        baseUrl,
        ruleset.id,
        ruleset.org_id
    );

    return (
        <div className="container mx-auto px-4 py-6 space-y-6">
            <Breadcrumb
                items={[
                    { label: 'CI/CD Gates', to: '/ci-rulesets' },
                    {
                        label: ruleset.name,
                        to: `/ci-rulesets/${ruleset.id}/edit`,
                    },
                    { label: 'Integration code' },
                ]}
            />
            <PageHeader
                title={`Integration code: ${ruleset.name}`}
                description="Drop this into your CI/CD job. The exit code alone is the whole gate."
            />

            <div className="border-b border-slate-700">
                <nav
                    className="flex flex-wrap gap-x-6"
                    aria-label="CI/CD platform"
                >
                    {PROVIDERS.map((p) => (
                        <button
                            key={p}
                            type="button"
                            onClick={() => setProvider(p)}
                            className={`py-3 px-1 border-b-2 text-sm font-medium transition-colors ${
                                provider === p
                                    ? 'border-blue-500 text-white'
                                    : 'border-transparent text-slate-400 hover:text-slate-200 hover:border-slate-600'
                            }`}
                        >
                            {PROVIDER_SHA_VARS[p].label}
                        </button>
                    ))}
                </nav>
            </div>

            {provider === 'curl' && (
                <Card
                    title="cURL / Bash snippet"
                    subtitle="Drop this into any CI step that runs bash"
                >
                    <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-200 overflow-x-auto">
                        {curlSnippet}
                    </pre>
                    <div className="flex justify-end mt-2">
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                                copyToClipboard(curlSnippet, 'Snippet')
                            }
                        >
                            Copy snippet
                        </Button>
                    </div>
                    <p className="text-xs text-slate-500 mt-2">
                        Set <code>LIVEREVIEW_API_KEY</code> as a CI secret.
                        Requires <code>jq</code> to be installed on the CI
                        runner.
                    </p>
                </Card>
            )}

            {provider !== 'curl' && (
                <Card
                    title="Environment variables"
                    subtitle={`Secrets to add in ${PROVIDER_SHA_VARS[provider].label}`}
                >
                    <button
                        type="button"
                        onClick={() => setInstructionsOpen((v) => !v)}
                        className={`w-full flex items-center justify-between text-left text-sm font-medium text-slate-300 hover:text-white bg-slate-900/40 hover:bg-slate-900/60 border border-slate-700 px-4 py-3 transition-colors ${
                            instructionsOpen ? 'rounded-t-md' : 'rounded-md'
                        }`}
                    >
                        <span>
                            Instructions to add CI/CD Secrets to your repository
                        </span>
                        <span
                            className={`text-slate-400 inline-flex transition-transform ${
                                instructionsOpen ? 'rotate-180' : ''
                            }`}
                        >
                            <Icons.ChevronDown />
                        </span>
                    </button>
                    {instructionsOpen && (
                        <div className="bg-slate-900/40 border border-t-0 border-slate-700 rounded-b-md px-4 py-3">
                            <ol className="list-decimal list-inside text-sm text-slate-300 space-y-1.5">
                                {PROVIDER_SECRET_INSTRUCTIONS[provider].map(
                                    (step, i) => (
                                        <li key={i}>{step}</li>
                                    )
                                )}
                            </ol>
                        </div>
                    )}
                    <div className="overflow-x-auto rounded border border-slate-700 mt-4 inline-block">
                        <Table style={{ width: 'auto' }}>
                            <TableHead divided={false}>
                                <TableHeaderCell className="!pr-2">
                                    Key
                                </TableHeaderCell>
                                <TableHeaderCell className="!pl-0 !pr-2" />
                                <TableHeaderCell className="!pr-2">
                                    Value
                                </TableHeaderCell>
                                <TableHeaderCell className="!pl-0 !pr-2" />
                            </TableHead>
                            <TableBody>
                                {GATE_SECRETS.map((secret) => {
                                    const value = secret.describe(
                                        baseUrl,
                                        ruleset.id,
                                        ruleset.org_id
                                    );
                                    const href = secret.linkHref?.(baseUrl);
                                    return (
                                        <TableRow key={secret.key}>
                                            <TableCell className="!pr-2">
                                                <span className="text-xs font-medium text-slate-300">
                                                    {secret.key}
                                                </span>
                                            </TableCell>
                                            <TableCell className="!pl-0 !pr-2">
                                                <CopyIconButton
                                                    text={secret.key}
                                                    label={`${secret.key} key`}
                                                    onCopy={copyToClipboard}
                                                />
                                            </TableCell>
                                            <TableCell className="!pr-2">
                                                {href ? (
                                                    <span className="text-sm text-slate-300">
                                                        {secret.linkPrefix}
                                                        <a
                                                            href={href}
                                                            target="_blank"
                                                            rel="noreferrer"
                                                            className="text-blue-400 hover:underline"
                                                        >
                                                            {secret.linkLabel}
                                                        </a>
                                                        {secret.linkSuffix}
                                                    </span>
                                                ) : (
                                                    <code className="text-green-400 font-mono text-sm">
                                                        {value}
                                                    </code>
                                                )}
                                            </TableCell>
                                            <TableCell className="!pl-0 !pr-2">
                                                {!href && (
                                                    <CopyIconButton
                                                        text={value}
                                                        label={`${secret.key} value`}
                                                        onCopy={
                                                            copyToClipboard
                                                        }
                                                    />
                                                )}
                                            </TableCell>
                                        </TableRow>
                                    );
                                })}
                            </TableBody>
                        </Table>
                    </div>
                </Card>
            )}

            {provider === 'github' && (
                <Card
                    title="Full GitHub Actions workflow"
                    subtitle="Drop this at .github/workflows/livereview-gate.yml"
                >
                    <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-200 overflow-x-auto">
                        {workflowYaml}
                    </pre>
                    <div className="flex justify-end mt-2">
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                                copyToClipboard(workflowYaml, 'Workflow')
                            }
                        >
                            Copy workflow
                        </Button>
                    </div>
                </Card>
            )}

            {provider === 'gitlab' && (
                <Card
                    title="Full GitLab CI pipeline"
                    subtitle="Drop this job into your .gitlab-ci.yml"
                >
                    <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-200 overflow-x-auto">
                        {gitlabYaml}
                    </pre>
                    <div className="flex justify-end mt-2">
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                                copyToClipboard(gitlabYaml, 'Pipeline')
                            }
                        >
                            Copy pipeline
                        </Button>
                    </div>
                </Card>
            )}

            {(provider === 'bitbucket' ||
                provider === 'azure' ||
                provider === 'generic') && (
                <Card
                    title={`Full ${PROVIDER_SHA_VARS[provider].label} config`}
                    subtitle="Not available as a ready-made template yet"
                >
                    <p className="text-sm text-slate-300">
                        We don't have a native{' '}
                        {PROVIDER_SHA_VARS[provider].label} config template
                        yet -- use the{' '}
                        <button
                            type="button"
                            onClick={() => setProvider('curl')}
                            className="text-blue-400 hover:underline"
                        >
                            cURL / Bash snippet
                        </button>{' '}
                        instead and drop it into your pipeline's script step.
                    </p>
                    <p className="text-sm text-slate-300 mt-2">
                        Want a native {PROVIDER_SHA_VARS[provider].label}{' '}
                        template added? Email{' '}
                        <a
                            href="mailto:info@hexmos.com"
                            className="text-blue-400 hover:underline"
                        >
                            info@hexmos.com
                        </a>{' '}
                        and we'll add it.
                    </p>
                </Card>
            )}

            <div className="flex justify-end">
                <Button
                    variant="ghost"
                    onClick={() => navigate('/ci-rulesets')}
                >
                    Back to rulesets
                </Button>
            </div>
        </div>
    );
};

export default CiRulesetIntegration;
