import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '../../UIPrimitives';

const MOCK_TOOLS_LIST = [
  { name: 'ruff', useCase: 'Python lint/format', multiplier: 1.0, status: 'enabled', findings: 0 },
  { name: 'bandit', useCase: 'Python SAST', multiplier: 1.0, status: 'enabled', findings: 14 },
  { name: 'gitleaks', useCase: 'Secret detection', multiplier: 1.0, status: 'enabled', findings: 0 },
  { name: 'eslint', useCase: 'JavaScript/TypeScript SAST', multiplier: 2.0, status: 'enabled', findings: 5 },
  { name: 'semgrep', useCase: 'Multi-language SAST', multiplier: 3.0, status: 'enabled', findings: 8 },
  { name: 'hadolint', useCase: 'Dockerfile lint', multiplier: 0.5, status: 'enabled', findings: 0 },
  { name: 'actionlint', useCase: 'GitHub Actions lint', multiplier: 0.5, status: 'enabled', findings: 2 },
  { name: 'shellcheck', useCase: 'Shell script lint', multiplier: 0.5, status: 'enabled', findings: 0 },
  { name: 'trufflehog', useCase: 'Deep secret scanning', multiplier: 2.0, status: 'enabled', findings: 0 },
  { name: 'trivy', useCase: 'Container/IaC CVE scan', multiplier: 2.5, status: 'enabled', findings: 3 },
  { name: 'spectral', useCase: 'API spec lint', multiplier: 1.0, status: 'enabled', findings: 0 },
  { name: 'brakeman', useCase: 'Ruby SAST', multiplier: 1.5, status: 'enabled', findings: 1 },
  { name: 'kubescape', useCase: 'Kubernetes IaC', multiplier: 2.0, status: 'enabled', findings: 0 },
  { name: 'zizmor', useCase: 'GitHub Actions security', multiplier: 1.0, status: 'enabled', findings: 2 },
  { name: 'openapi', useCase: 'OpenAPI/YAML validation', multiplier: 0.5, status: 'enabled', findings: 0 },
];

export const ToolsUsageWidget: React.FC = () => {
    const navigate = useNavigate();

    return (
        <div className="h-full flex flex-col justify-between text-xs">
            <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-700/60">
                <div>
                    <span className="font-semibold text-slate-200 text-sm">Static Analysis Tools</span>
                    <p className="text-[11px] text-slate-400">15 tools enabled • 20.0× credit tier</p>
                </div>
                <Button
                    variant="ghost"
                    size="sm"
                    className="text-xs text-blue-400 hover:text-blue-300"
                    onClick={() => navigate('/settings#third-party-tools')}
                >
                    Configure →
                </Button>
            </div>

            <div className="h-44 overflow-y-auto space-y-1.5 pr-1">
                {MOCK_TOOLS_LIST.map((tool) => (
                    <div
                        key={tool.name}
                        className="flex items-center justify-between rounded bg-slate-900/60 border border-slate-800 px-2.5 py-1.5"
                    >
                        <div className="flex items-center gap-2 min-w-0">
                            <span className="font-mono font-semibold text-white">{tool.name}</span>
                            <span className="text-[11px] text-slate-400 truncate hidden sm:inline">({tool.useCase})</span>
                        </div>

                        <div className="flex items-center gap-2 shrink-0">
                            <span className="font-mono text-[10px] text-slate-400">{tool.multiplier}×</span>
                            <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded border ${
                                tool.findings === 0 
                                    ? 'bg-slate-800 text-slate-400 border-slate-700' 
                                    : 'bg-slate-800 text-slate-200 font-medium border-slate-600'
                            }`}>
                                {tool.findings === 0 ? 'Clean' : `${tool.findings} findings`}
                            </span>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
};
