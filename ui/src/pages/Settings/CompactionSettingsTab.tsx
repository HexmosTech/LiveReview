import React, { useState, useEffect } from 'react';
import { Button } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';
import CronBuilder from '../../components/reviews/cronbuilder/CronBuilder';
import { getLocalCronText, localTimeZoneName } from '../../components/reviews/cronbuilder/cronTimezone';

interface CompactionConfig {
    enabled: boolean;
    cron_expression: string;
    retention_days: number;
    schedule_human?: string;
}

const DEFAULT_CONFIG: CompactionConfig = {
    enabled: true,
    cron_expression: '0 2 * * *',
    retention_days: 30,
};

const CompactionSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<CompactionConfig>(DEFAULT_CONFIG);
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isRunning, setIsRunning] = useState(false);
    const [isDone, setIsDone] = useState(false);
    const [showAdvanced, setShowAdvanced] = useState(false);

    useEffect(() => {
        loadConfig();
    }, []);

    const loadConfig = async () => {
        setIsLoading(true);
        try {
            const configData = await apiClient.get<CompactionConfig>('/api/v1/admin/settings/compaction');
            if (configData) setSettings(configData);
        } catch {
            notify.error('Failed to load compaction settings');
        } finally {
            setIsLoading(false);
        }
    };

    const saveSettingsToBackend = async (silent = false) => {
        try {
            await apiClient.put('/api/v1/admin/settings/compaction', {
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
        } finally {
            setIsSaving(false);
        }
    };

    const handleRunNow = async () => {
        setIsRunning(true);
        setIsDone(false);
        try {
            await saveSettingsToBackend(true);
            await apiClient.post('/api/v1/admin/settings/compaction/run', {});
            notify.success('Cleanup started in the background!');
            setIsDone(true);
            setTimeout(() => setIsDone(false), 3000);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to start cleanup');
        } finally {
            setIsRunning(false);
        }
    };

    if (isLoading) {
        return (
            <div className="flex justify-center items-center p-12">
                <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    const cronTextObj = getLocalCronText(settings.cron_expression || '0 2 * * *');
    const humanSchedule = cronTextObj.status && cronTextObj.value ? cronTextObj.value : settings.cron_expression;
    const tzName = localTimeZoneName();

    return (
        <div className="space-y-5">
            {/* Header */}
            <div className="flex items-center space-x-3">
                <div className="p-2 bg-indigo-500/10 border border-indigo-500/20 rounded-lg text-indigo-400">
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                </div>
                <div>
                    <h3 className="text-lg font-semibold text-white">Event Log Cleanup</h3>
                    <p className="text-sm text-slate-400">Automated log retention and cleanup for historical reviews</p>
                </div>
            </div>

            {/* Primary View: 100% Static Read-Only Overview Graphic */}
            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-4">
                {/* Enable Switch */}
                <div className="flex items-center justify-between pb-4 border-b border-slate-700/60">
                    <div>
                        <div className="flex items-center space-x-2">
                            <span className="text-sm font-medium text-white">Enable Automatic Log Cleanup</span>
                            <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium">
                                Recommended
                            </span>
                        </div>
                        <p className="text-xs text-slate-400 mt-1">
                            Automatically removes old verbose streaming logs on schedule. AI findings, comments, and milestones are preserved.
                        </p>
                    </div>
                    <label className="relative inline-flex items-center cursor-pointer flex-shrink-0 ml-4">
                        <input
                            type="checkbox"
                            className="sr-only peer"
                            checked={settings.enabled}
                            onChange={(e) => setSettings(prev => ({ ...prev, enabled: e.target.checked }))}
                        />
                        <div className="w-11 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-indigo-600"></div>
                    </label>
                </div>

                {/* Read-Only Status Row */}
                {settings.enabled && (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div className="bg-slate-900/50 border border-slate-700/50 rounded-lg p-3.5">
                            <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Retention Window</span>
                            <div className="text-lg font-bold text-white mt-0.5">
                                {settings.retention_days} Days
                            </div>
                            <p className="text-xs text-slate-400 mt-0.5">
                                Prunes verbose debug logs from reviews older than {settings.retention_days} days
                            </p>
                        </div>

                        <div className="bg-slate-900/50 border border-slate-700/50 rounded-lg p-3.5">
                            <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Execution Schedule</span>
                            <div className="text-lg font-bold text-indigo-300 mt-0.5">
                                {humanSchedule}
                            </div>
                            <p className="text-xs text-slate-500 mt-0.5">
                                {tzName}
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
                            {/* Fully Symmetrical Editing Card */}
                            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-5">
                                {/* Top Row: Log Retention Period */}
                                <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-700/60">
                                    <div>
                                        <h4 className="text-sm font-semibold text-white">Log Retention Period</h4>
                                        <p className="text-xs text-slate-400 mt-0.5">
                                            Reviews older than this number of days are eligible for automatic log cleanup.
                                        </p>
                                    </div>
                                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                                        <span className="text-xs font-medium text-slate-300">Keep logs for</span>
                                        <input
                                            type="number"
                                            min={1}
                                            max={365}
                                            value={settings.retention_days}
                                            onChange={(e) => setSettings(prev => ({ ...prev, retention_days: Math.max(1, parseInt(e.target.value, 10) || 30) }))}
                                            className="w-16 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-indigo-500"
                                        />
                                        <span className="text-xs font-medium text-slate-300">days</span>
                                    </div>
                                </div>

                                {/* Bottom Row: Execution Schedule Builder */}
                                <div>
                                    <h4 className="text-sm font-semibold text-white mb-1">Execution Schedule</h4>
                                    <p className="text-xs text-slate-400 mb-4">
                                        Configure timing to run during low-traffic hours for your team.
                                    </p>
                                    <CronBuilder
                                        defaultValue={settings.cron_expression || '0 2 * * *'}
                                        onChange={(newCron) => {
                                            if (newCron) setSettings(prev => ({ ...prev, cron_expression: newCron }));
                                        }}
                                    />
                                </div>
                            </div>

                            {/* Run Now Trigger */}
                            <div className="flex items-center justify-between p-4 bg-slate-800/80 border border-slate-700/80 rounded-xl">
                                <div>
                                    <p className="text-sm font-medium text-white">Manual Trigger</p>
                                    <p className="text-xs text-slate-400 mt-0.5">Saves current settings and runs cleanup immediately in the background</p>
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

                            {/* Save Settings Action Footer */}
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

export default CompactionSettingsTab;
