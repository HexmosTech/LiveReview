import React, { useState, useEffect } from 'react';
import { Button } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';
import CronBuilder from '../../components/reviews/cronbuilder/CronBuilder';
import { getCronText } from '../../components/reviews/cronbuilder/cronUtils';

interface PreloadedChangesArchivalConfig {
    enabled: boolean;
    cron_expression: string;
    retention_days: number;
    schedule_human?: string;
}

const PreloadedChangesArchivalSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<PreloadedChangesArchivalConfig | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isRunning, setIsRunning] = useState(false);
    const [isDone, setIsDone] = useState(false);
    const [showAdvanced, setShowAdvanced] = useState(false);
    // null = loading, true = configured, false = not configured
    const [blobConfigured, setBlobConfigured] = useState<boolean | null>(null);

    useEffect(() => {
        loadConfig();
        checkBlobStorage();
    }, []);

    const loadConfig = async () => {
        setIsLoading(true);
        try {
            const configData = await apiClient.get<PreloadedChangesArchivalConfig>('/api/v1/admin/settings/preloaded-changes-archival');
            if (configData) setSettings(configData);
        } catch {
            notify.error('Failed to load preloaded changes archival settings');
        } finally {
            setIsLoading(false);
        }
    };

    const checkBlobStorage = async () => {
        try {
            // Reuse the existing storage settings endpoint — if it returns a non-local type, blob is configured.
            const data = await apiClient.get<{ storage_type?: string }>('/api/v1/admin/settings/storage');
            const storageType = data?.storage_type ?? 'local_fs';
            setBlobConfigured(storageType !== 'local_fs' && storageType !== '');
        } catch {
            setBlobConfigured(false);
        }
    };

    const saveSettingsToBackend = async (silent = false) => {
        if (!settings) return;
        try {
            await apiClient.put('/api/v1/admin/settings/preloaded-changes-archival', {
                enabled: settings.enabled,
                cron_expression: settings.cron_expression,
                retention_days: Number(settings.retention_days),
            });
            if (!silent) notify.success('Settings saved successfully!');
        } catch (error: any) {
            if (!silent) notify.error(error?.message || 'Failed to save settings');
            throw error;
        }
    };

    const handleSave = async () => {
        setIsSaving(true);
        try {
            await saveSettingsToBackend(false);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to save settings');
        } finally {
            setIsSaving(false);
        }
    };

    const handleRunNow = async () => {
        setIsRunning(true);
        setIsDone(false);
        try {
            await saveSettingsToBackend(true);
            await apiClient.post('/api/v1/admin/settings/preloaded-changes-archival/run', {});
            notify.success('Preloaded changes archival started in the background!');
            setIsDone(true);
            setTimeout(() => setIsDone(false), 3000);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to start archival');
        } finally {
            setIsRunning(false);
        }
    };

    if (isLoading) {
        return (
            <div className="flex justify-center items-center p-12">
                <div className="w-8 h-8 border-2 border-violet-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    if (!settings) return null;

    const cronTextObj = getCronText(settings.cron_expression || '30 21 * * *');
    const humanSchedule = cronTextObj.status && cronTextObj.value ? cronTextObj.value : (settings.schedule_human || settings.cron_expression);

    return (
        <div className="space-y-5">
            {/* Header */}
            <div className="flex items-center space-x-3">
                <div className="p-2 bg-violet-500/10 border border-violet-500/20 rounded-lg text-violet-400">
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
                    </svg>
                </div>
                <div>
                    <h3 className="text-lg font-semibold text-white">Preloaded Changes Archival</h3>
                    <p className="text-sm text-slate-400">Moves code diffs (preloaded changes) older than the retention window from PostgreSQL to Blob Storage</p>
                </div>
            </div>

            {/* Blob Storage Warning Banner */}
            {blobConfigured === false && (
                <div className="flex items-start space-x-3 p-4 bg-amber-500/10 border border-amber-500/30 rounded-xl">
                    <div className="flex-shrink-0 text-amber-400 mt-0.5">
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                        </svg>
                    </div>
                    <div>
                        <p className="text-sm font-medium text-amber-300">Blob Storage not configured</p>
                        <p className="text-xs text-amber-400/80 mt-0.5">
                            Archival requires an external Blob Storage (S3, Backblaze B2, GCS, or Azure). Without it, the archival job will skip every run and diffs will remain in PostgreSQL.
                            Configure storage first under <span className="font-semibold text-amber-300">Settings → Storage</span>.
                        </p>
                    </div>
                </div>
            )}

            {/* Primary View: Read-Only Overview */}
            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-4">
                {/* Enable Switch */}
                <div className="flex items-center justify-between pb-4 border-b border-slate-700/60">
                    <div>
                        <div className="flex items-center space-x-2">
                            <span className="text-sm font-medium text-white">Enable Automatic Preloaded Changes Archival</span>
                            <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium">
                                Recommended
                            </span>
                        </div>
                        <p className="text-xs text-slate-400 mt-1">
                            Automatically moves code diffs from PostgreSQL to Blob Storage on schedule. Diffs remain readable via the API at all times.
                        </p>
                    </div>
                    <label className="relative inline-flex items-center cursor-pointer flex-shrink-0 ml-4">
                        <input
                            type="checkbox"
                            className="sr-only peer"
                            checked={settings.enabled}
                            onChange={(e) => setSettings(prev => prev ? ({ ...prev, enabled: e.target.checked }) : null)}
                        />
                        <div className="w-11 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-violet-600"></div>
                    </label>
                </div>

                {/* Read-Only Status Cards */}
                {settings.enabled && (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div className="bg-slate-900/50 border border-slate-700/50 rounded-lg p-3.5">
                            <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Retention Window</span>
                            <div className="text-lg font-bold text-white mt-0.5">
                                {settings.retention_days} Days
                            </div>
                            <p className="text-xs text-slate-400 mt-0.5">
                                Reviews older than {settings.retention_days} days are eligible for archival
                            </p>
                        </div>

                        <div className="bg-slate-900/50 border border-slate-700/50 rounded-lg p-3.5">
                            <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Execution Schedule</span>
                            <div className="text-lg font-bold text-violet-300 mt-0.5">
                                {humanSchedule}
                            </div>
                            <p className="text-xs text-slate-500 mt-0.5">
                                UTC Standard Time
                            </p>
                        </div>
                    </div>
                )}
            </div>

            {/* Advanced Section (Editing Controls) */}
            {settings.enabled && (
                <div className="space-y-4">
                    <button
                        type="button"
                        onClick={() => setShowAdvanced(prev => !prev)}
                        className="flex items-center space-x-2 text-sm font-medium text-slate-300 hover:text-white transition-colors"
                    >
                        <svg
                            className={`w-4 h-4 transition-transform duration-200 ${showAdvanced ? 'rotate-90' : ''}`}
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                        >
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                        </svg>
                        <span>Advanced (Edit Retention &amp; Schedule)</span>
                    </button>

                    {showAdvanced && (
                        <div className="space-y-4 pt-1">
                            {/* Editing Card */}
                            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-5">
                                {/* Retention Period */}
                                <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-700/60">
                                    <div>
                                        <h4 className="text-sm font-semibold text-white">Diff Retention Period</h4>
                                        <p className="text-xs text-slate-400 mt-0.5">
                                            Reviews older than this many days are eligible for automatic archival to Blob Storage.
                                        </p>
                                    </div>
                                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                                        <span className="text-xs font-medium text-slate-300">Archive after</span>
                                        <input
                                            type="number"
                                            min={1}
                                            max={365}
                                            value={settings.retention_days}
                                            onChange={(e) =>
                                                setSettings(prev => prev ? ({
                                                    ...prev,
                                                    retention_days: Math.max(1, parseInt(e.target.value, 10) || 30),
                                                }) : null)
                                            }
                                            className="w-16 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-violet-500"
                                        />
                                        <span className="text-xs font-medium text-slate-300">days</span>
                                    </div>
                                </div>

                                {/* Schedule Builder */}
                                <div>
                                    <h4 className="text-sm font-semibold text-white mb-1">Execution Schedule</h4>
                                    <p className="text-xs text-slate-400 mb-4">
                                        Schedule archival to run during low-traffic hours to minimise DB load.
                                    </p>
                                    <CronBuilder
                                        defaultValue={settings.cron_expression || '30 21 * * *'}
                                        onChange={(newCron) => {
                                            if (newCron) setSettings(prev => prev ? ({ ...prev, cron_expression: newCron }) : null);
                                        }}
                                    />
                                </div>
                            </div>

                            {/* Manual Trigger */}
                            <div className="flex items-center justify-between p-4 bg-slate-800/80 border border-slate-700/80 rounded-xl">
                                <div>
                                    <p className="text-sm font-medium text-white">Manual Trigger</p>
                                    <p className="text-xs text-slate-400 mt-0.5">Saves current settings and runs archival immediately in the background</p>
                                </div>
                                <Button
                                    variant="outline"
                                    onClick={handleRunNow}
                                    isLoading={isRunning}
                                    disabled={isSaving || isRunning}
                                    className={isDone
                                        ? 'border-emerald-600/70 text-emerald-300 hover:bg-emerald-900/30'
                                        : 'border-amber-600/70 text-amber-300 hover:bg-amber-900/30'}
                                >
                                    {isDone ? (
                                        <>
                                            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                                            </svg>
                                            Done!
                                        </>
                                    ) : (
                                        <>
                                            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                            </svg>
                                            Run Now
                                        </>
                                    )}
                                </Button>
                            </div>

                            {/* Save Footer */}
                            <div className="flex justify-end pt-2 border-t border-slate-700/80">
                                <Button
                                    variant="primary"
                                    onClick={handleSave}
                                    isLoading={isSaving}
                                    disabled={isSaving || isRunning}
                                >
                                    Save Settings
                                </Button>
                            </div>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};

export default PreloadedChangesArchivalSettingsTab;
