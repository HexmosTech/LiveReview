import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Icons, EmptyState } from '../../UIPrimitives';
import { HumanizedTimestamp } from '../../HumanizedTimestamp/HumanizedTimestamp';
import { useSystemOverview } from './SystemOverviewData';
import { ChartSkeleton } from './ChartSkeleton';

const ICON_MAP: Record<string, React.FC> = {
    github: Icons.GitHub,
    gitlab: Icons.GitLab,
    'gitlab-com': Icons.GitLab,
    'gitlab-self-hosted': Icons.GitLab,
    bitbucket: Icons.Bitbucket,
    gitea: Icons.Gitea,
    azuredevops: Icons.AzureDevOps,
    openai: Icons.OpenAI,
    claude: Icons.Claude,
    anthropic: Icons.Claude,
    'anthropic-compatible': Icons.AnthropicCompatible,
    google: Icons.Google,
    gemini: Icons.Google,
    'gemini-enterprise': Icons.Google,
    ollama: Icons.Ollama,
    deepseek: Icons.DeepSeek,
    cohere: Icons.Cohere,
    atlas: Icons.AtlasCloud,
    bedrock: Icons.Aws,
};

// Not period-scoped — always the current connection state, regardless of the Day/Week/Month/All selector.
export const ConnectedProviders: React.FC = () => {
    const navigate = useNavigate();
    const { systemOverview, loading } = useSystemOverview();

    if (loading) {
        return <ChartSkeleton />;
    }

    const providers = systemOverview?.connected_providers ?? [];
    if (providers.length === 0) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="Nothing connected yet"
                description="Connected git hosts and AI providers will appear here once you connect them."
            />
        );
    }

    return (
        <div className="h-full overflow-y-auto space-y-2 pr-1">
            {providers.map((provider) => {
                const Icon = ICON_MAP[provider.provider.toLowerCase()];
                return (
                    <button
                        key={provider.id}
                        type="button"
                        onClick={() => navigate(provider.kind === 'git' ? '/git' : '/ai')}
                        className="w-full flex items-center justify-between gap-2 rounded-lg bg-slate-900/40 border border-slate-700/50 hover:border-slate-600 hover:bg-slate-900/60 transition-colors px-3 py-2 text-left"
                    >
                        <div className="flex items-center gap-2.5 min-w-0">
                            <span className="text-slate-300 shrink-0">{Icon ? <Icon /> : null}</span>
                            <div className="min-w-0">
                                <p className="text-sm text-slate-100 truncate">{provider.name}</p>
                                <p className="text-[11px] text-slate-500">
                                    Synced <HumanizedTimestamp timestamp={provider.last_synced_at} className="text-slate-500 text-[11px]" />
                                </p>
                            </div>
                        </div>
                        <div className="shrink-0 flex items-center gap-1.5">
                            <span
                                className={`text-[11px] font-medium px-2 py-0.5 rounded-full border ${
                                    provider.kind === 'git'
                                        ? 'bg-sky-500/15 text-sky-300 border-sky-500/30'
                                        : 'bg-amber-500/15 text-amber-300 border-amber-500/30'
                                }`}
                            >
                                {provider.kind === 'git' ? 'Git' : 'AI'}
                            </span>
                            <span className="text-[11px] font-medium px-2 py-0.5 rounded-full bg-green-500/15 text-green-300 border border-green-500/30">
                                Connected
                            </span>
                        </div>
                    </button>
                );
            })}
        </div>
    );
};
