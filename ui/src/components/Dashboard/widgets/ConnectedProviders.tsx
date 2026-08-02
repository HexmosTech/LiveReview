import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Icons } from '../../UIPrimitives';
import { MOCK_CONNECTED_PROVIDERS } from './mockData';

const ICON_MAP: Record<string, React.FC> = {
    GitHub: Icons.GitHub,
    GitLab: Icons.GitLab,
    Bitbucket: Icons.Bitbucket,
    Gitea: Icons.Gitea,
    OpenAI: Icons.OpenAI,
    Claude: Icons.Claude,
    Google: Icons.Google,
    Ollama: Icons.Ollama,
    DeepSeek: Icons.DeepSeek,
};

const formatLastSync = (minutesAgo: number): string => {
    if (minutesAgo < 60) return `${minutesAgo}m ago`;
    return `${Math.round(minutesAgo / 60)}h ago`;
};

// Most orgs only have a couple of git hosts and AI backends connected — this is
// meant to read as a short, simple "what's plugged in" list, not a monitoring board.
export const ConnectedProviders: React.FC = () => {
    const navigate = useNavigate();

    return (
        <div className="h-full overflow-y-auto space-y-2 pr-1">
            {MOCK_CONNECTED_PROVIDERS.map((provider) => {
                const Icon = ICON_MAP[provider.iconKey];
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
                                <p className="text-[11px] text-slate-500">Synced {formatLastSync(provider.lastSyncMinutesAgo)}</p>
                            </div>
                        </div>
                        <span className="shrink-0 text-[11px] font-medium px-2 py-0.5 rounded-full bg-green-500/15 text-green-300 border border-green-500/30">
                            Connected
                        </span>
                    </button>
                );
            })}
        </div>
    );
};
