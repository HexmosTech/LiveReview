import React, { useEffect, useState } from 'react';
import { Button } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';

interface AIReviewPerformanceSettings {
    max_concurrent_batches_ceiling: number;
    min_review_timeout_floor_seconds: number;
    max_review_timeout_ceiling_seconds: number;
}

const ENDPOINT = '/api/v1/admin/settings/ai-review-performance';

// Instance-wide safety bounds for AI review concurrency and timeout. These
// are ceilings/floors, not targets: per-review concurrency is actually
// governed by a self-tuning limiter (see internal/llm/adaptivelimiter) that
// discovers the AI provider's real rate limit on its own. This page just
// sets the outer bounds that limiter - and any org's own soft preference in
// AI Providers → Review AI Settings - can never exceed.
const AIReviewPerformanceSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<AIReviewPerformanceSettings | null>(null);
    const [isSaving, setIsSaving] = useState(false);

    const load = async () => {
        try {
            setSettings(await apiClient.get<AIReviewPerformanceSettings>(ENDPOINT));
        } catch (error: any) {
            notify.error(error?.message || 'Failed to load AI review performance settings');
        }
    };

    useEffect(() => {
        load();
    }, []);

    const handleSave = async () => {
        if (!settings) return;
        setIsSaving(true);
        try {
            await apiClient.put(ENDPOINT, {
                max_concurrent_batches_ceiling: Number(settings.max_concurrent_batches_ceiling),
                min_review_timeout_floor_seconds: Number(settings.min_review_timeout_floor_seconds),
                max_review_timeout_ceiling_seconds: Number(settings.max_review_timeout_ceiling_seconds),
            });
            notify.success('Settings saved successfully!');
            await load();
        } catch (error: any) {
            notify.error(error?.message || 'Failed to save AI review performance settings');
        } finally {
            setIsSaving(false);
        }
    };

    if (!settings) {
        return (
            <div className="flex justify-center items-center p-12">
                <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    const timeoutRangeInvalid = settings.max_review_timeout_ceiling_seconds < settings.min_review_timeout_floor_seconds;
    const canSubmit = settings.max_concurrent_batches_ceiling > 0 && settings.min_review_timeout_floor_seconds > 0 && !timeoutRangeInvalid;

    return (
        <div className="space-y-5">
            <div className="flex items-center space-x-3">
                <div className="p-2 bg-indigo-500/10 border border-indigo-500/20 rounded-lg text-indigo-400">
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                </div>
                <div>
                    <h3 className="text-lg font-semibold text-white">AI Review Performance</h3>
                    <p className="text-sm text-slate-400">
                        Instance-wide safety bounds for review concurrency and timeout. Any organization's own preference (AI Providers → Review AI Settings) is always clamped within these.
                    </p>
                </div>
            </div>

            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-4">
                <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-700/60">
                    <div>
                        <h4 className="text-sm font-semibold text-white">Max concurrent batches ceiling</h4>
                        <p className="text-xs text-slate-400 mt-0.5">
                            Highest number of a review's batches allowed to run at once. A self-tuning limiter discovers the actual safe number for your AI provider at runtime - this is just the safety cap it can never exceed.
                        </p>
                    </div>
                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                        <input
                            type="number"
                            min={1}
                            value={settings.max_concurrent_batches_ceiling}
                            onChange={(e) => setSettings({ ...settings, max_concurrent_batches_ceiling: parseInt(e.target.value, 10) || 0 })}
                            aria-label="Max concurrent batches ceiling"
                            className="w-20 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-indigo-500"
                        />
                        <span className="text-xs font-medium text-slate-300">batches</span>
                    </div>
                </div>

                <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-700/60">
                    <div>
                        <h4 className="text-sm font-semibold text-white">Min review timeout</h4>
                        <p className="text-xs text-slate-400 mt-0.5">
                            Floor on how short a review's overall timeout can be set, instance-wide or per-org.
                        </p>
                    </div>
                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                        <input
                            type="number"
                            min={1}
                            value={settings.min_review_timeout_floor_seconds}
                            onChange={(e) => setSettings({ ...settings, min_review_timeout_floor_seconds: parseInt(e.target.value, 10) || 0 })}
                            aria-label="Min review timeout in seconds"
                            className="w-20 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-indigo-500"
                        />
                        <span className="text-xs font-medium text-slate-300">seconds</span>
                    </div>
                </div>

                <div className="flex flex-wrap items-center justify-between gap-4">
                    <div>
                        <h4 className="text-sm font-semibold text-white">Max review timeout ceiling</h4>
                        <p className="text-xs text-slate-400 mt-0.5">
                            Highest the overall review timeout can be set to, instance-wide or per-org.
                        </p>
                    </div>
                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                        <input
                            type="number"
                            min={settings.min_review_timeout_floor_seconds}
                            value={settings.max_review_timeout_ceiling_seconds}
                            onChange={(e) => setSettings({ ...settings, max_review_timeout_ceiling_seconds: parseInt(e.target.value, 10) || 0 })}
                            aria-label="Max review timeout ceiling in seconds"
                            className="w-20 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-indigo-500"
                        />
                        <span className="text-xs font-medium text-slate-300">seconds</span>
                    </div>
                </div>
                {timeoutRangeInvalid && (
                    <p className="text-xs text-red-400">Max review timeout ceiling must be at or above the min review timeout.</p>
                )}
            </div>

            <div className="flex justify-end pt-2 border-t border-slate-700/80">
                <Button variant="primary" onClick={handleSave} isLoading={isSaving} disabled={isSaving || !canSubmit}>
                    Save Settings
                </Button>
            </div>
        </div>
    );
};

export default AIReviewPerformanceSettingsTab;
