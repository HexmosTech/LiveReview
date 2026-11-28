import React from 'react';
import { ReviewAccounting } from '../../types/reviews';
import { RelativeTime } from '../UIPrimitives';
import {
    formatCurrency,
    formatInt,
    formatStageLabel,
    getStageExecutionText,
    getStageRouteText,
    StageAIConfig,
} from '../../pages/Reviews/reviewDetailHelpers';

export interface ReviewAccountingPanelProps {
    accounting: ReviewAccounting | null;
    accountingError: string | null;
    accountingBannerClass: string;
    showToolsDropdown: boolean;
    reviewMetadata?: Record<string, any>;
    active: boolean;
}

export const ReviewAccountingPanel: React.FC<ReviewAccountingPanelProps> = ({
    accounting,
    accountingError,
    accountingBannerClass,
    showToolsDropdown,
    reviewMetadata,
    active,
}) => {
    const leaderAI: StageAIConfig = {
        mode: typeof reviewMetadata?.leader_ai_execution_mode === 'string' ? reviewMetadata.leader_ai_execution_mode : '',
        source: typeof reviewMetadata?.leader_ai_execution_source === 'string' ? reviewMetadata.leader_ai_execution_source : '',
        provider: typeof reviewMetadata?.leader_ai_provider_name === 'string' ? reviewMetadata.leader_ai_provider_name : '',
        connector: typeof reviewMetadata?.leader_ai_connector_name === 'string' ? reviewMetadata.leader_ai_connector_name : '',
    };

    const helperAI: StageAIConfig = {
        mode: typeof reviewMetadata?.helper_ai_execution_mode === 'string' ? reviewMetadata.helper_ai_execution_mode : '',
        source: typeof reviewMetadata?.helper_ai_execution_source === 'string' ? reviewMetadata.helper_ai_execution_source : '',
        provider: typeof reviewMetadata?.helper_ai_provider_name === 'string' ? reviewMetadata.helper_ai_provider_name : '',
        connector: typeof reviewMetadata?.helper_ai_connector_name === 'string' ? reviewMetadata.helper_ai_connector_name : '',
    };

    const helperEnabled = !!accounting?.helperEnabled;
    const stageBreakdown = accounting?.stageBreakdown || [];

    return (
        <div className={`bg-slate-800 rounded-lg p-4 border border-slate-700 mb-6 ${active ? '' : 'hidden'}`}>
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-white">Accounting</h2>
                {accounting?.lastAccountedAt ? (
                    <span className="text-slate-500">
                        Last accounted <RelativeTime timestamp={accounting.lastAccountedAt} />
                    </span>
                ) : (
                    <span className="text-slate-500">Auto-refresh every 15s</span>
                )}
            </div>
            {accountingError && (
                <div className={accountingBannerClass}>
                    {accountingError}
                </div>
            )}
            <div className={`grid grid-cols-2 ${showToolsDropdown ? 'md:grid-cols-7' : 'md:grid-cols-6'} gap-2 text-xs mb-3`}>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Total LOC</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.totalBillableLoc || 0).toLocaleString()}</p>
                </div>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Input Tokens</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{formatInt(accounting?.totalInputTokens)}</p>
                </div>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Output Tokens</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{formatInt(accounting?.totalOutputTokens)}</p>
                </div>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Total Cost (USD)</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{formatCurrency(accounting?.totalCostUsd)}</p>
                </div>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Accounted Ops</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.accountedOperations || 0).toLocaleString()}</p>
                </div>
                <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                    <p className="text-slate-500">Token-tracked Ops</p>
                    <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.tokenTrackedOperations || 0).toLocaleString()}</p>
                </div>
                {showToolsDropdown && (
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Tool Credits</p>
                        <p className="text-white font-semibold text-sm mt-0.5">
                            {accounting?.toolCreditsDeducted != null && !isNaN(Number(accounting.toolCreditsDeducted))
                                ? Number(accounting.toolCreditsDeducted).toFixed(1)
                                : <span className="text-slate-600">—</span>}
                        </p>
                    </div>
                )}
            </div>

            {!!stageBreakdown.length && (
                <div className="mt-3 pt-3 border-t border-slate-700/60">
                    <div className="mb-2 flex items-center justify-between">
                        <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">Model Breakdown</h3>
                        <span className="text-[11px] text-slate-500">
                            {helperEnabled ? 'Leader & Helper stages' : 'Single-stage review'}
                        </span>
                    </div>
                    <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
                        {stageBreakdown.map((stage, idx) => {
                            const routeText = getStageRouteText(stage, leaderAI, helperAI);
                            const executionText = getStageExecutionText(stage, leaderAI, helperAI);
                            return (
                                <div
                                    key={`${stage.stage}-${stage.provider || 'unknown'}-${stage.model || 'unknown'}-${stage.pricingVersion || ''}-${idx}`}
                                    className="rounded border border-slate-800 bg-slate-900/90 p-2.5"
                                >
                                    <div className="mb-2 flex items-start justify-between gap-3">
                                        <div>
                                            <p className="text-xs font-semibold text-white">{formatStageLabel(stage.stage)}</p>
                                            <p className="text-[11px] text-slate-400">
                                                {(stage.provider || 'unknown provider')} / {(stage.model || 'unknown model')}
                                            </p>
                                        </div>
                                        {stage.pricingVersion && (
                                            <span className="rounded border border-slate-700 px-2 py-0.5 text-[10px] text-slate-400">
                                                {stage.pricingVersion}
                                            </span>
                                        )}
                                    </div>
                                    <div className="grid grid-cols-3 gap-2 text-xs">
                                        <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                            <p className="text-[10px] text-slate-500">Input</p>
                                            <p className="text-xs font-medium text-white">{formatInt(stage.inputTokens)}</p>
                                        </div>
                                        <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                            <p className="text-[10px] text-slate-500">Output</p>
                                            <p className="text-xs font-medium text-white">{formatInt(stage.outputTokens)}</p>
                                        </div>
                                        <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                            <p className="text-[10px] text-slate-500">Cost</p>
                                            <p className="text-xs font-medium text-white">{formatCurrency(stage.costUsd)}</p>
                                        </div>
                                    </div>
                                    {(executionText || routeText) && (
                                        <div className="mt-2 space-y-1 text-xs text-slate-300">
                                            {executionText && (
                                                <p><span className="text-slate-500">Execution:</span> {executionText}</p>
                                            )}
                                            {routeText && (
                                                <p><span className="text-slate-500">Route:</span> {routeText}</p>
                                            )}
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>
                </div>
            )}

            {accounting?.latestOperation && (
                <div className="bg-slate-900 rounded-md p-3 border border-slate-700 text-xs">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-y-2 gap-x-4">
                        <p className="text-slate-300"><span className="text-slate-500">Latest operation:</span> {accounting.latestOperation.operationType || '—'}</p>
                        <p className="text-slate-300"><span className="text-slate-500">Trigger:</span> {accounting.latestOperation.triggerSource || '—'}</p>
                        <p className="text-slate-300"><span className="text-slate-500">Provider/Model:</span> {(accounting.latestOperation.provider || 'unknown')} / {(accounting.latestOperation.model || 'unknown')}</p>
                        <p className="text-slate-300"><span className="text-slate-500">Pricing version:</span> {accounting.latestOperation.pricingVersion || 'unknown'}</p>
                        <p className="text-slate-300"><span className="text-slate-500">Operation ID:</span> {accounting.latestOperation.operationId || '—'}</p>
                        <p className="text-slate-300"><span className="text-slate-500">Idempotency key:</span> {accounting.latestOperation.idempotencyKey || '—'}</p>
                        {(leaderAI.mode || leaderAI.source) && (
                            <p className="text-slate-300"><span className="text-slate-500">Leader execution:</span> {(leaderAI.mode || 'unknown')} via {(leaderAI.source || 'unknown')}</p>
                        )}
                        {(leaderAI.provider || leaderAI.connector) && (
                            <p className="text-slate-300"><span className="text-slate-500">Leader route:</span> {(leaderAI.provider || 'unknown')} / {(leaderAI.connector || 'unknown')}</p>
                        )}
                        {helperEnabled && (helperAI.mode || helperAI.source) && (
                            <p className="text-slate-300"><span className="text-slate-500">Helper execution:</span> {(helperAI.mode || 'unknown')} via {(helperAI.source || 'unknown')}</p>
                        )}
                        {helperEnabled && (helperAI.provider || helperAI.connector) && (
                            <p className="text-slate-300"><span className="text-slate-500">Helper route:</span> {(helperAI.provider || 'unknown')} / {(helperAI.connector || 'unknown')}</p>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
};

