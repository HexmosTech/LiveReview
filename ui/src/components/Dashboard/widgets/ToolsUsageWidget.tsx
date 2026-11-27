import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import apiClient from '../../../api/apiClient';
import { Button } from '../../UIPrimitives';
import { useOrgContext } from '../../../hooks/useOrgContext';
import { isCloudMode } from '../../../utils/deploymentMode';

interface OrgTool {
  id: number;
  name: string;
  use_case?: string;
  category?: string;
  multiplier: number;
  enabled: boolean;
}

export const ToolsUsageWidget: React.FC = () => {
    const navigate = useNavigate();
    const { currentOrg, isPaidPlan } = useOrgContext();
    const [tools, setTools] = useState<OrgTool[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        if (!isCloudMode() || !isPaidPlan || !currentOrg?.id) return;
        let cancelled = false;
        setLoading(true);
        apiClient
            .get<{ tools?: OrgTool[] }>(`/orgs/${currentOrg.id}/tools`)
            .then((res) => {
                if (!cancelled) {
                    setTools(res.tools || []);
                }
            })
            .catch(() => {
                if (!cancelled) {
                    setTools([]);
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setLoading(false);
                }
            });
        return () => {
            cancelled = true;
        };
    }, [currentOrg?.id, isPaidPlan]);

    if (!isCloudMode() || !isPaidPlan) {
        return null;
    }

    const enabledTools = tools.filter((t) => t.enabled);
    const totalMultiplier = enabledTools.reduce((acc, t) => acc + Number(t.multiplier || 0), 0);

    return (
        <div className="h-full flex flex-col justify-between text-xs">
            <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-700/60">
                <div>
                    <span className="font-semibold text-slate-200 text-sm">SAST Tools</span>
                    <p className="text-[11px] text-slate-400">
                        {loading
                            ? 'Loading tool configuration...'
                            : `${enabledTools.length} of ${tools.length} tools enabled • ${totalMultiplier.toFixed(1)}× credit tier`}
                    </p>
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
                {!loading && enabledTools.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-slate-400 text-center px-3">
                        {tools.length === 0
                            ? 'No static analysis tools registered yet.'
                            : 'No static analysis tools enabled for this organization.'}
                    </div>
                ) : (
                    enabledTools.map((tool) => (
                        <div
                            key={tool.id || tool.name}
                            className="flex items-center justify-between rounded bg-slate-900/60 border border-slate-800 px-2.5 py-1.5"
                        >
                            <div className="flex items-center gap-2 min-w-0">
                                <span className="font-semibold text-white">{tool.name}</span>
                                {(tool.category || tool.use_case) && (
                                    <span className="text-[11px] text-slate-400 truncate hidden sm:inline">
                                        ({tool.category || tool.use_case})
                                    </span>
                                )}
                            </div>

                            <div className="flex items-center gap-2 shrink-0">
                                <span className="text-[10px] text-slate-400 tabular-nums">{Number(tool.multiplier || 0).toFixed(1)}×</span>
                                <span className="text-[10px] font-medium px-1.5 py-0.5 rounded border bg-slate-800 text-emerald-300 border-emerald-700/50">
                                    Enabled
                                </span>
                            </div>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
};
