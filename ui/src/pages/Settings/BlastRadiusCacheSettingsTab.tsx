import React, { useEffect, useState } from 'react';
import { Button } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';

interface BlastRadiusCacheSettings {
    enabled: boolean;
    max_gb: number;
    min_gb: number;
    used_bytes: number;
    repos: number;
}

const ENDPOINT = '/api/v1/admin/settings/blast-radius-cache';
const GB = 1024 ** 3;

// Server-side blast radius keeps a shallow clone + graph index of each reviewed repo on the
// lrdata volume; this section sets the cache size (least-recently-used repos are evicted).
const BlastRadiusCacheSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<BlastRadiusCacheSettings | null>(null);
    const [isSaving, setIsSaving] = useState(false);

    const load = async () => {
        try {
            setSettings(await apiClient.get<BlastRadiusCacheSettings>(ENDPOINT));
        } catch (error: any) {
            notify.error(error?.message || 'Failed to load blast radius cache settings');
        }
    };

    useEffect(() => {
        load();
    }, []);

    const handleSave = async () => {
        if (!settings) return;
        setIsSaving(true);
        try {
            await apiClient.put(ENDPOINT, { enabled: settings.enabled, max_gb: Number(settings.max_gb) });
            notify.success('Settings saved successfully!');
            await load();
        } catch (error: any) {
            notify.error(error?.message || 'Failed to save settings');
        } finally {
            setIsSaving(false);
        }
    };

    if (!settings) {
        return (
            <div className="flex justify-center items-center p-12">
                <div className="w-8 h-8 border-2 border-violet-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    const usedGB = settings.used_bytes / GB;
    const usedPct = Math.min(100, (usedGB / Math.max(settings.max_gb, 1)) * 100);
    const tooSmall = settings.max_gb < settings.min_gb;

    return (
        <div className="space-y-5">
            <div className="flex items-center space-x-3">
                <div className="p-2 bg-violet-500/10 border border-violet-500/20 rounded-lg text-violet-400">
                    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
                    </svg>
                </div>
                <div>
                    <h3 className="text-lg font-semibold text-white">Repo Cache</h3>
                    <p className="text-sm text-slate-400">
                        Keeps a shallow clone and code graph of each reviewed repo so PR reviews get blast-radius risk scores. Least-recently-used repos are removed when the cache is full.
                    </p>
                </div>
            </div>

            <div className="bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 space-y-4">
                <div className="flex items-center justify-between pb-4 border-b border-slate-700/60">
                    <div>
                        <span className="text-sm font-medium text-white">Enable server-side blast radius</span>
                        <p className="text-xs text-slate-400 mt-1">
                            Scores PR reviews started from LiveReview.
                        </p>
                    </div>
                    <label className="relative inline-flex items-center cursor-pointer flex-shrink-0 ml-4">
                        <input
                            type="checkbox"
                            className="sr-only peer"
                            checked={settings.enabled}
                            onChange={(e) => setSettings({ ...settings, enabled: e.target.checked })}
                            aria-label="Enable server-side blast radius"
                        />
                        <div className="w-11 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-violet-600"></div>
                    </label>
                </div>

                <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-700/60">
                    <div>
                        <h4 className="text-sm font-semibold text-white">Cache size</h4>
                        <p className="text-xs text-slate-400 mt-0.5">
                            Minimum {settings.min_gb} GB. Raise it to keep more repos warm.
                        </p>
                    </div>
                    <div className="flex items-center space-x-3 bg-slate-900/80 border border-slate-700 rounded-lg px-4 py-2">
                        <input
                            type="number"
                            min={settings.min_gb}
                            max={10000}
                            value={settings.max_gb}
                            onChange={(e) => setSettings({ ...settings, max_gb: parseInt(e.target.value, 10) || 0 })}
                            aria-label="Cache size in GB"
                            className="w-20 bg-slate-800 border border-slate-600 rounded px-2 py-1 text-white font-bold text-sm text-center focus:outline-none focus:border-violet-500"
                        />
                        <span className="text-xs font-medium text-slate-300">GB</span>
                    </div>
                </div>

                <div>
                    <div className="flex justify-between text-xs text-slate-400 mb-1.5">
                        <span>
                            Used {usedGB.toFixed(1)} GB of {settings.max_gb} GB · {settings.repos} {settings.repos === 1 ? 'repo' : 'repos'} cached
                        </span>
                        <span>{usedPct.toFixed(0)}%</span>
                    </div>
                    <div className="h-2 bg-slate-700 rounded-full overflow-hidden">
                        <div className="h-full bg-violet-500" style={{ width: `${usedPct}%` }} />
                    </div>
                </div>
            </div>

            <div className="flex justify-end pt-2 border-t border-slate-700/80">
                <Button variant="primary" onClick={handleSave} isLoading={isSaving} disabled={isSaving || tooSmall}>
                    Save Settings
                </Button>
            </div>
        </div>
    );
};

export default BlastRadiusCacheSettingsTab;
