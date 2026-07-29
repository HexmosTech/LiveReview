import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Button, Alert } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';
import { isCloudMode } from '../../utils/deploymentMode';

interface SlackConfig {
    configured: boolean;
    id?: number;
    org_id?: number;
    team_id?: string;
    enabled?: boolean;
    created_at?: string;
    updated_at?: string;
}

interface TeamsConfig {
    configured: boolean;
    bot_app_id?: string;
    tenant_id?: string;
}

interface DiscordConfig {
    configured: boolean;
    guild_id?: string;
}

const IntegrationsTab: React.FC = () => {
    const { currentOrg } = useOrgContext();

    return (
        <div className="space-y-8">
            <div>
                <div className="flex items-center gap-2 mb-1">
                    <h3 className="text-lg font-medium text-white">Integrations</h3>
                    {isCloudMode() && (
                        <span className="rounded border border-amber-700/50 bg-amber-900/30 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-400">
                            Enterprise
                        </span>
                    )}
                </div>
                <p className="text-sm text-slate-300">
                    Connect LiveReview to external services for extended functionality.
                </p>
            </div>

            {isCloudMode() ? (
                <div className="rounded-lg border border-slate-700 bg-slate-800/50 p-6 text-center">
                    <p className="text-sm text-slate-300">
                        Slack, Microsoft Teams, and Discord integrations are an Enterprise feature and are not available on this plan.
                    </p>
                </div>
            ) : (
                <>
                    <SlackIntegration currentOrg={currentOrg} />
                    <TeamsIntegration currentOrg={currentOrg} />
                    <DiscordIntegration currentOrg={currentOrg} />
                </>
            )}
        </div>
    );
};

const SlackIntegration: React.FC<{ currentOrg: any }> = ({ currentOrg }) => {
    const [config, setConfig] = useState<SlackConfig | null>(null);
    const [loading, setLoading] = useState(true);
    const [connecting, setConnecting] = useState(false);
    const [disconnecting, setDisconnecting] = useState(false);
    const [showDisconnectModal, setShowDisconnectModal] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);

    const loadConfig = useCallback(async () => {
        if (!currentOrg) return;
        setLoading(true);
        setError(null);
        try {
            const response = await apiClient.get<SlackConfig>(`/orgs/${currentOrg.id}/slack-config`);
            setConfig(response);
        } catch {
            setConfig({ configured: false });
        } finally {
            setLoading(false);
        }
    }, [currentOrg?.id]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const handleConnect = async () => {
        if (!currentOrg) return;
        setConnecting(true);
        setError(null);
        try {
            const redirectTo = encodeURIComponent(window.location.origin + '/#/settings#integrations');
            const response = await apiClient.get<{ url: string }>(`/auth/slack/install?org_id=${currentOrg.id}&redirect_to=${redirectTo}`);
            window.location.href = response.url;
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to initiate Slack connection');
            setConnecting(false);
        }
    };

    const handleDisconnect = () => {
        if (!currentOrg || !config?.configured) return;
        setShowDisconnectModal(true);
    };

    const confirmDisconnect = async () => {
        if (!currentOrg || !config?.configured) return;
        setDisconnecting(true);
        setError(null);
        setSuccess(null);
        try {
            await apiClient.delete(`/orgs/${currentOrg.id}/slack-config`);
            setSuccess('Slack bot disconnected successfully.');
            setConfig({ configured: false });
            setShowDisconnectModal(false);
        } catch (err: any) {
            setError(err.message || 'Failed to disconnect Slack bot');
        } finally {
            setDisconnecting(false);
        }
    };

    return (
        <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
            <div className="p-5">
                <div className="flex items-start space-x-4">
                    <div className="flex-shrink-0 mt-1">
                        <img src="/assets/slack-logo.png" alt="Slack" className="w-10 h-10 rounded-lg" />
                    </div>
                    <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                            <div>
                                <h4 className="text-white font-medium">Slack</h4>
                                <p className="text-sm text-slate-400">
                                    Get code review insights and analytics in your Slack workspace
                                </p>
                            </div>
                            {!loading && (
                                <div className="flex-shrink-0 ml-4">
                                    {config?.configured ? (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900 text-green-200">
                                            Connected
                                        </span>
                                    ) : (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300">
                                            Not connected
                                        </span>
                                    )}
                                </div>
                            )}
                        </div>

                        {error && (
                            <div className="mt-3">
                                <Alert variant="error" onClose={() => setError(null)}>
                                    {error}
                                </Alert>
                            </div>
                        )}

                        {success && (
                            <div className="mt-3">
                                <Alert variant="success" onClose={() => setSuccess(null)}>
                                    {success}
                                </Alert>
                            </div>
                        )}

                        {loading ? (
                            <div className="mt-3">
                                <div className="w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
                            </div>
                        ) : config?.configured ? (
                            <div className="mt-3 flex items-center space-x-3">
                                {config.team_id && (
                                    <span className="text-xs text-slate-500">
                                        Workspace: <code className="bg-slate-700 px-1 rounded">{config.team_id}</code>
                                    </span>
                                )}
                                <Button size="sm" variant="ghost" className="text-red-400 hover:text-red-300" onClick={handleDisconnect}>
                                    Disconnect
                                </Button>
                            </div>
                        ) : (
                            <div className="mt-3">
                                <Button
                                    size="sm"
                                    variant="primary"
                                    onClick={handleConnect}
                                    disabled={connecting}
                                    isLoading={connecting}
                                >
                                    Connect
                                </Button>
                            </div>
                        )}
                    </div>
                </div>
            </div>
            {showDisconnectModal && createPortal(
                <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
                    <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setShowDisconnectModal(false)} />
                    <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-sm w-full">
                        <div className="flex items-center justify-between p-5 border-b border-slate-700">
                            <div className="flex items-center space-x-3">
                                <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
                                    <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                                    </svg>
                                </div>
                                <h2 className="text-lg font-semibold text-white">Disconnect Slack</h2>
                            </div>
                        </div>
                        <div className="p-5">
                            <p className="text-slate-300">Are you sure you want to disconnect the Slack bot from this workspace?</p>
                        </div>
                        <div className="flex items-center justify-end space-x-3 p-5 bg-slate-900/50 border-t border-slate-700 rounded-b-lg">
                            <button
                                type="button"
                                onClick={confirmDisconnect}
                                disabled={disconnecting}
                                className="px-4 py-2 text-sm font-medium text-red-300 bg-red-500/10 border border-red-500/40 hover:bg-red-500/20 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
                            >
                                {disconnecting ? (
                                    <>
                                        <div className="w-4 h-4 border-2 border-red-300 border-t-transparent rounded-full animate-spin"></div>
                                        <span>Disconnecting...</span>
                                    </>
                                ) : (
                                    <>
                                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                        </svg>
                                        <span>Disconnect</span>
                                    </>
                                )}
                            </button>
                            <button
                                type="button"
                                onClick={() => setShowDisconnectModal(false)}
                                disabled={disconnecting}
                                autoFocus
                                className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                Cancel
                            </button>
                        </div>
                    </div>
                </div>,
                document.body
            )}
        </div>
    );
};

const TeamsIntegration: React.FC<{ currentOrg: any }> = ({ currentOrg }) => {
    const [config, setConfig] = useState<TeamsConfig | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [disconnecting, setDisconnecting] = useState(false);
    const [showDisconnectModal, setShowDisconnectModal] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [editMode, setEditMode] = useState(false);
    const [form, setForm] = useState({ bot_app_id: '', bot_password: '' });

    const loadConfig = useCallback(async () => {
        if (!currentOrg) return;
        setLoading(true);
        setError(null);
        try {
            const response = await apiClient.get<TeamsConfig>(`/orgs/${currentOrg.id}/teams-config`);
            setConfig(response);
            if (response.configured) {
                setForm({ bot_app_id: response.bot_app_id || '', bot_password: '' });
            }
        } catch {
            setConfig({ configured: false });
        } finally {
            setLoading(false);
        }
    }, [currentOrg?.id]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const handleSave = async () => {
        if (!currentOrg) return;
        if (!form.bot_app_id || !form.bot_password) {
            setError('Both App ID and Password are required');
            return;
        }
        setSaving(true);
        setError(null);
        setSuccess(null);
        try {
            const response = await apiClient.put<TeamsConfig>(
                `/orgs/${currentOrg.id}/teams-config`,
                { bot_app_id: form.bot_app_id, bot_password: form.bot_password }
            );
            setConfig(response);
            setEditMode(false);
            setForm({ bot_app_id: response.bot_app_id || '', bot_password: '' });
            setSuccess('Teams bot configured successfully.');
        } catch (err: any) {
            setError(err.message || 'Failed to save Teams configuration');
        } finally {
            setSaving(false);
        }
    };

    const handleDisconnect = () => {
        if (!currentOrg || !config?.configured) return;
        setShowDisconnectModal(true);
    };

    const confirmDisconnect = async () => {
        if (!currentOrg || !config?.configured) return;
        setDisconnecting(true);
        setError(null);
        setSuccess(null);
        try {
            await apiClient.delete(`/orgs/${currentOrg.id}/teams-config`);
            setSuccess('Teams bot disconnected successfully.');
            setConfig({ configured: false });
            setEditMode(false);
            setShowDisconnectModal(false);
        } catch (err: any) {
            setError(err.message || 'Failed to disconnect Teams bot');
        } finally {
            setDisconnecting(false);
        }
    };

    return (
        <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
            <div className="p-5">
                <div className="flex items-start space-x-4">
                    <div className="flex-shrink-0 mt-1">
                        <img src="/assets/teams-logo.svg" alt="Microsoft Teams" className="w-10 h-10 rounded-lg" />
                    </div>
                    <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                            <div>
                                <h4 className="text-white font-medium">Microsoft Teams</h4>
                                <p className="text-sm text-slate-400">
                                    Receive code review insights directly in your Microsoft Teams channels
                                </p>
                            </div>
                            {!loading && (
                                <div className="flex-shrink-0 ml-4">
                                    {config?.configured ? (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900 text-green-200">
                                            Connected
                                        </span>
                                    ) : (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300">
                                            Not connected
                                        </span>
                                    )}
                                </div>
                            )}
                        </div>

                        {error && (
                            <div className="mt-3">
                                <Alert variant="error" onClose={() => setError(null)}>
                                    {error}
                                </Alert>
                            </div>
                        )}

                        {success && (
                            <div className="mt-3">
                                <Alert variant="success" onClose={() => setSuccess(null)}>
                                    {success}
                                </Alert>
                            </div>
                        )}

                        {loading ? (
                            <div className="mt-3">
                                <div className="w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
                            </div>
                        ) : config?.configured && !editMode ? (
                            <div className="mt-3 flex items-center space-x-3">
                                {config.bot_app_id && (
                                    <span className="text-xs text-slate-500">
                                        App ID: <code className="bg-slate-700 px-1 rounded">{config.bot_app_id}</code>
                                    </span>
                                )}
                                <Button size="sm" variant="ghost" className="text-blue-400 hover:text-blue-300" onClick={() => setEditMode(true)}>
                                    Edit
                                </Button>
                                <Button size="sm" variant="ghost" className="text-red-400 hover:text-red-300" onClick={handleDisconnect}>
                                    Disconnect
                                </Button>
                            </div>
                        ) : (
                            <div className="mt-3 space-y-3">
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">Bot App ID</label>
                                    <input
                                        type="text"
                                        value={form.bot_app_id}
                                        onChange={(e) => setForm({ ...form, bot_app_id: e.target.value })}
                                        placeholder="e.g. 12345678-1234-1234-1234-123456789012"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                </div>
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">Bot Password (Client Secret)</label>
                                    <input
                                        type="password"
                                        value={form.bot_password}
                                        onChange={(e) => setForm({ ...form, bot_password: e.target.value })}
                                        placeholder="Enter your bot client secret"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                </div>
                                <p className="text-xs text-slate-500">
                                    Create an Azure Bot in the Azure Portal, then paste the App ID and Client Secret here.
                                    Set the messaging endpoint to <code className="bg-slate-700 px-1 rounded">{window.location.origin}/api/messages</code>
                                </p>
                                <div className="flex items-center space-x-3">
                                    <Button
                                        size="sm"
                                        variant="primary"
                                        onClick={handleSave}
                                        disabled={saving || !form.bot_app_id || !form.bot_password}
                                        isLoading={saving}
                                    >
                                        Save
                                    </Button>
                                    {editMode && (
                                        <Button
                                            size="sm"
                                            variant="ghost"
                                            onClick={() => {
                                                setEditMode(false);
                                                if (config?.configured) {
                                                    setForm({ bot_app_id: config.bot_app_id || '', bot_password: '' });
                                                } else {
                                                    setForm({ bot_app_id: '', bot_password: '' });
                                                }
                                            }}
                                        >
                                            Cancel
                                        </Button>
                                    )}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            </div>
            {showDisconnectModal && createPortal(
                <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
                    <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setShowDisconnectModal(false)} />
                    <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-sm w-full">
                        <div className="flex items-center justify-between p-5 border-b border-slate-700">
                            <div className="flex items-center space-x-3">
                                <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
                                    <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                                    </svg>
                                </div>
                                <h2 className="text-lg font-semibold text-white">Disconnect Microsoft Teams</h2>
                            </div>
                        </div>
                        <div className="p-5">
                            <p className="text-slate-300">Are you sure you want to disconnect the Teams bot from this workspace?</p>
                        </div>
                        <div className="flex items-center justify-end space-x-3 p-5 bg-slate-900/50 border-t border-slate-700 rounded-b-lg">
                            <button
                                type="button"
                                onClick={confirmDisconnect}
                                disabled={disconnecting}
                                className="px-4 py-2 text-sm font-medium text-red-300 bg-red-500/10 border border-red-500/40 hover:bg-red-500/20 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
                            >
                                {disconnecting ? (
                                    <>
                                        <div className="w-4 h-4 border-2 border-red-300 border-t-transparent rounded-full animate-spin"></div>
                                        <span>Disconnecting...</span>
                                    </>
                                ) : (
                                    <>
                                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                        </svg>
                                        <span>Disconnect</span>
                                    </>
                                )}
                            </button>
                            <button
                                type="button"
                                onClick={() => setShowDisconnectModal(false)}
                                disabled={disconnecting}
                                autoFocus
                                className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                Cancel
                            </button>
                        </div>
                    </div>
                </div>,
                document.body
            )}
        </div>
    );
};

const DiscordIntegration: React.FC<{ currentOrg: any }> = ({ currentOrg }) => {
    const [config, setConfig] = useState<DiscordConfig | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [disconnecting, setDisconnecting] = useState(false);
    const [showDisconnectModal, setShowDisconnectModal] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [editMode, setEditMode] = useState(false);
    const [botToken, setBotToken] = useState('');

    const loadConfig = useCallback(async () => {
        if (!currentOrg) return;
        setLoading(true);
        setError(null);
        try {
            const response = await apiClient.get<DiscordConfig>(`/orgs/${currentOrg.id}/discord-config`);
            setConfig(response);
        } catch {
            setConfig({ configured: false });
        } finally {
            setLoading(false);
        }
    }, [currentOrg?.id]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const handleSave = async () => {
        if (!currentOrg) return;
        if (!botToken) {
            setError('Bot token is required');
            return;
        }
        setSaving(true);
        setError(null);
        setSuccess(null);
        try {
            const response = await apiClient.put<DiscordConfig>(
                `/orgs/${currentOrg.id}/discord-config`,
                { bot_token: botToken }
            );
            setConfig(response);
            setEditMode(false);
            setBotToken('');
            setSuccess('Discord bot configured successfully.');
        } catch (err: any) {
            setError(err.message || 'Failed to save Discord configuration');
        } finally {
            setSaving(false);
        }
    };

    const handleDisconnect = () => {
        if (!currentOrg || !config?.configured) return;
        setShowDisconnectModal(true);
    };

    const confirmDisconnect = async () => {
        if (!currentOrg || !config?.configured) return;
        setDisconnecting(true);
        setError(null);
        setSuccess(null);
        try {
            await apiClient.delete(`/orgs/${currentOrg.id}/discord-config`);
            setSuccess('Discord bot disconnected successfully.');
            setConfig({ configured: false });
            setEditMode(false);
            setShowDisconnectModal(false);
        } catch (err: any) {
            setError(err.message || 'Failed to disconnect Discord bot');
        } finally {
            setDisconnecting(false);
        }
    };

    return (
        <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
            <div className="p-5">
                <div className="flex items-start space-x-4">
                    <div className="flex-shrink-0 mt-1">
                        <img src="/assets/discord-logo.svg" alt="Discord" className="w-10 h-10 rounded-lg" />
                    </div>
                    <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                            <div>
                                <h4 className="text-white font-medium">Discord</h4>
                                <p className="text-sm text-slate-400">
                                    Interact with LiveReview directly from your Discord server
                                </p>
                            </div>
                            {!loading && (
                                <div className="flex-shrink-0 ml-4">
                                    {config?.configured ? (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900 text-green-200">
                                            Connected
                                        </span>
                                    ) : (
                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700 text-slate-300">
                                            Not connected
                                        </span>
                                    )}
                                </div>
                            )}
                        </div>

                        {error && (
                            <div className="mt-3">
                                <Alert variant="error" onClose={() => setError(null)}>
                                    {error}
                                </Alert>
                            </div>
                        )}

                        {success && (
                            <div className="mt-3">
                                <Alert variant="success" onClose={() => setSuccess(null)}>
                                    {success}
                                </Alert>
                            </div>
                        )}

                        {loading ? (
                            <div className="mt-3">
                                <div className="w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
                            </div>
                        ) : config?.configured && !editMode ? (
                            <div className="mt-3 flex items-center space-x-3">
                                {config.guild_id && (
                                    <span className="text-xs text-slate-500">
                                        Server ID: <code className="bg-slate-700 px-1 rounded">{config.guild_id}</code>
                                    </span>
                                )}
                                <Button size="sm" variant="ghost" className="text-blue-400 hover:text-blue-300" onClick={() => setEditMode(true)}>
                                    Edit
                                </Button>
                                <Button size="sm" variant="ghost" className="text-red-400 hover:text-red-300" onClick={handleDisconnect}>
                                    Disconnect
                                </Button>
                            </div>
                        ) : (
                            <div className="mt-3 space-y-3">
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">Bot Token</label>
                                    <input
                                        type="password"
                                        value={botToken}
                                        onChange={(e) => setBotToken(e.target.value)}
                                        placeholder="Enter your Discord bot token"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                </div>
                                <div className="text-xs text-slate-400 space-y-2">
                                    <p className="font-medium text-slate-300">Step-by-step setup:</p>
                                    <ol className="list-decimal list-inside space-y-1.5">
                                        <li>
                                            Go to the{' '}
                                            <a href="https://discord.com/developers/applications" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300 underline">
                                                Discord Developer Portal
                                            </a>
                                            {' '}and click <strong className="text-slate-200">New Application</strong>.
                                        </li>
                                        <li>
                                            Under <strong className="text-slate-200">Bot</strong> on the left sidebar, click <strong className="text-slate-200">Add Bot</strong> and confirm.
                                        </li>
                                        <li>
                                            Under the <strong className="text-slate-200">Privileged Gateway Intents</strong> section, enable all three:
                                            <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5 text-slate-500">
                                                <li><code className="bg-slate-700 px-1 rounded">MESSAGE CONTENT INTENT</code> — required to read message content</li>
                                                <li><code className="bg-slate-700 px-1 rounded">SERVER MEMBERS INTENT</code> — required to see guild members</li>
                                                <li><code className="bg-slate-700 px-1 rounded">GUILD INTENTS</code> (usually enabled by default)</li>
                                            </ul>
                                        </li>
                                        <li>
                                            Click <strong className="text-slate-200">Reset Token</strong> (or copy the existing one), then paste the bot token below and click <strong className="text-slate-200">Save</strong>.
                                        </li>
                                        <li>
                                            Go to <strong className="text-slate-200">OAuth2 &gt; URL Generator</strong> on the left sidebar. Under <strong className="text-slate-200">Scopes</strong>, select:
                                            <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5 text-slate-500">
                                                <li><code className="bg-slate-700 px-1 rounded">bot</code></li>
                                                <li><code className="bg-slate-700 px-1 rounded">applications.commands</code></li>
                                            </ul>
                                        </li>
                                        <li>
                                            Under <strong className="text-slate-200">Bot Permissions</strong>, select:
                                            <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5 text-slate-500">
                                                <li><code className="bg-slate-700 px-1 rounded">Send Messages</code></li>
                                                <li><code className="bg-slate-700 px-1 rounded">Send Messages in Threads</code></li>
                                                <li><code className="bg-slate-700 px-1 rounded">Read Message History</code></li>
                                                <li><code className="bg-slate-700 px-1 rounded">Read Messages / View Channels</code></li>
                                                <li><code className="bg-slate-700 px-1 rounded">Attach Files</code> — for chart images</li>
                                                <li><code className="bg-slate-700 px-1 rounded">Embed Links</code></li>
                                            </ul>
                                        </li>
                                        <li>
                                            Use the generated URL at the bottom to invite the bot to your server.
                                        </li>
                                        <li>
                                            DM the bot or mention it with <code className="bg-slate-700 px-1 rounded">@YourBotName your question</code> in a channel to start.
                                        </li>
                                    </ol>
                                </div>
                                <div className="flex items-center space-x-3">
                                    <Button
                                        size="sm"
                                        variant="primary"
                                        onClick={handleSave}
                                        disabled={saving || !botToken}
                                        isLoading={saving}
                                    >
                                        Save
                                    </Button>
                                    {editMode && (
                                        <Button
                                            size="sm"
                                            variant="ghost"
                                            onClick={() => {
                                                setEditMode(false);
                                                setBotToken('');
                                            }}
                                        >
                                            Cancel
                                        </Button>
                                    )}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            </div>
            {showDisconnectModal && createPortal(
                <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
                    <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setShowDisconnectModal(false)} />
                    <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-sm w-full">
                        <div className="flex items-center justify-between p-5 border-b border-slate-700">
                            <div className="flex items-center space-x-3">
                                <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
                                    <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                                    </svg>
                                </div>
                                <h2 className="text-lg font-semibold text-white">Disconnect Discord</h2>
                            </div>
                        </div>
                        <div className="p-5">
                            <p className="text-slate-300">Are you sure you want to disconnect the Discord bot from this server?</p>
                        </div>
                        <div className="flex items-center justify-end space-x-3 p-5 bg-slate-900/50 border-t border-slate-700 rounded-b-lg">
                            <button
                                type="button"
                                onClick={confirmDisconnect}
                                disabled={disconnecting}
                                className="px-4 py-2 text-sm font-medium text-red-300 bg-red-500/10 border border-red-500/40 hover:bg-red-500/20 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center space-x-2"
                            >
                                {disconnecting ? (
                                    <>
                                        <div className="w-4 h-4 border-2 border-red-300 border-t-transparent rounded-full animate-spin"></div>
                                        <span>Disconnecting...</span>
                                    </>
                                ) : (
                                    <>
                                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                        </svg>
                                        <span>Disconnect</span>
                                    </>
                                )}
                            </button>
                            <button
                                type="button"
                                onClick={() => setShowDisconnectModal(false)}
                                disabled={disconnecting}
                                autoFocus
                                className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                Cancel
                            </button>
                        </div>
                    </div>
                </div>,
                document.body
            )}
        </div>
    );
};

export default IntegrationsTab;
