import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Button, Alert, Icons } from '../../components/UIPrimitives';
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
    application_id?: string;
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
    const [saving, setSaving] = useState(false);
    const [disconnecting, setDisconnecting] = useState(false);
    const [showDisconnectModal, setShowDisconnectModal] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [editMode, setEditMode] = useState(false);
    const [botToken, setBotToken] = useState('');
    const [appToken, setAppToken] = useState('');
    const [manifestCopied, setManifestCopied] = useState(false);

    const slackManifest = `display_information:
  name: Livi
  description: LiveReview Bot
  background_color: "#001c5e"
features:
  bot_user:
    display_name: Livi
    always_online: true
oauth_config:
  scopes:
    bot:
      - channels:read
      - app_mentions:read
      - channels:history
      - chat:write
      - files:write
      - groups:history
      - im:history
      - im:read
      - im:write
      - users:read
settings:
  event_subscriptions:
    bot_events:
      - app_mention
      - message.im
  interactivity:
    is_enabled: true
  socket_mode_enabled: true
  token_rotation_enabled: false`;

    const copyManifest = () => {
        navigator.clipboard.writeText(slackManifest);
        setManifestCopied(true);
        setTimeout(() => setManifestCopied(false), 2000);
    };

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

    const handleSave = async () => {
        if (!currentOrg) return;
        if (!botToken) {
            setError('Bot token is required');
            return;
        }
        if (!appToken) {
            setError('App-level token is required');
            return;
        }
        setSaving(true);
        setError(null);
        setSuccess(null);
        try {
            const response = await apiClient.put<SlackConfig>(
                `/orgs/${currentOrg.id}/slack-config`,
                { bot_token: botToken, app_token: appToken }
            );
            setConfig(response);
            setEditMode(false);
            setBotToken('');
            setAppToken('');
            setSuccess('Slack bot configured successfully.');
        } catch (err: any) {
            setError(err.message || 'Failed to save Slack configuration');
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
            await apiClient.delete(`/orgs/${currentOrg.id}/slack-config`);
            setSuccess('Slack bot disconnected successfully.');
            setConfig({ configured: false });
            setEditMode(false);
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
                        ) : config?.configured && !editMode ? (
                            <div className="mt-3 flex items-center justify-between gap-3 flex-wrap">
                                <div className="flex items-center space-x-3 flex-wrap gap-y-2">
                                    {config.team_id && (
                                        <span className="text-xs text-slate-500">
                                            Workspace: <code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">{config.team_id}</code>
                                        </span>
                                    )}
                                </div>
                                <div className="flex items-center space-x-3">
                                    <Button size="sm" variant="outline" className="!px-2 !py-1 border-blue-500/60 text-blue-300 hover:border-blue-400 hover:bg-blue-500/10 hover:text-blue-200" onClick={() => setEditMode(true)}>
                                        Edit
                                    </Button>
                                    <Button size="sm" variant="outline" className="!px-2 !py-1 border-red-500/60 text-red-300 hover:border-red-400 hover:bg-red-500/10 hover:text-red-200" onClick={handleDisconnect}>
                                        Disconnect
                                    </Button>
                                </div>
                            </div>
                        ) : (
                            <div className="mt-3 space-y-3">
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">App-Level Token</label>
                                    <input
                                        type="password"
                                        value={appToken}
                                        onChange={(e) => setAppToken(e.target.value)}
                                        placeholder="xapp-1-xxxxxxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                </div>
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">Bot Token</label>
                                    <input
                                        type="password"
                                        value={botToken}
                                        onChange={(e) => setBotToken(e.target.value)}
                                        placeholder="xoxb-xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                </div>
                                <div className="text-xs text-slate-400 space-y-3">
                                    <ol className="list-decimal list-inside space-y-1.5">
                                        <li>
                                            In{' '}
                                            <a href="https://api.slack.com/apps" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300 underline">
                                                Slack API Apps
                                            </a>
                                            , click <strong className="text-slate-200">Create New App</strong>, choose <strong className="text-slate-200">From manifest</strong>.
                                        </li>
                                        <li>
                                            Paste the manifest below and choose your workspace
                                        </li>

                                    </ol>
                                    <div className="relative">
                                        <pre className="bg-slate-900 rounded p-3 text-xs text-slate-200 font-mono overflow-x-auto whitespace-pre">{slackManifest}</pre>
                                        <Button
                                            size="sm"
                                            variant="ghost"
                                            className="absolute top-2 right-2"
                                            onClick={copyManifest}
                                            icon={<Icons.Copy />}
                                        >
                                            {manifestCopied ? 'Copied!' : 'Copy'}
                                        </Button>
                                    </div>
                                    <ol className="list-decimal list-inside space-y-1.5" start={3}>
                                        <li>
                                            Click <strong className="text-slate-200">"Next" &gt; "Create and Install"</strong>.
                                        </li>
                                        <li>
                                            When prompted <strong className="text-slate-200">Allow the "Livi" app to access Slack</strong>, click <strong className="text-slate-200">Allow</strong>.
                                        </li>
                                        <li>
                                            Go to <strong className="text-slate-200">Basic Information &gt; App-Level Tokens</strong>, click <strong className="text-slate-200">Generate Token and Scopes</strong>, add the <strong className="text-slate-200">connections:write</strong> scope, then click <strong className="text-slate-200">Generate</strong>. Copy it into the <strong className="text-slate-200">App-Level Token</strong> field above.
                                        </li>
                                        <li>
                                            Go to <strong className="text-slate-200">Install App</strong> in the sidebar and click <strong className="text-slate-200">Install to Workspace</strong> to reveal the <strong className="text-slate-200">Bot Token</strong>. Copy it into the field above.
                                        </li>
                                        <li>
                                            Click <strong className="text-slate-200">Save</strong>.
                                        </li>
                                    </ol>
                                </div>
                                <div className="flex items-center space-x-3">
                                    <Button
                                        size="sm"
                                        variant="primary"
                                        onClick={handleSave}
                                        disabled={saving || !botToken || !appToken}
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
                                                setAppToken('');
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
                            <div className="mt-3 flex items-center justify-between gap-3 flex-wrap">
                                <div className="flex items-center space-x-3 flex-wrap gap-y-2">
                                    {config.bot_app_id && (
                                        <span className="text-xs text-slate-500">
                                            App ID: <code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">{config.bot_app_id}</code>
                                        </span>
                                    )}
                                </div>
                                <div className="flex items-center space-x-3">
                                    <Button size="sm" variant="outline" className="!px-2 !py-1 border-blue-500/60 text-blue-300 hover:border-blue-400 hover:bg-blue-500/10 hover:text-blue-200" onClick={() => setEditMode(true)}>
                                        Edit
                                    </Button>
                                    <Button size="sm" variant="outline" className="!px-2 !py-1 border-red-500/60 text-red-300 hover:border-red-400 hover:bg-red-500/10 hover:text-red-200" onClick={handleDisconnect}>
                                        Disconnect
                                    </Button>
                                </div>
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
                                    Set the messaging endpoint to <code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">{window.location.origin}/api/messages</code>
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

// Discord bot permission integer (OAuth2 permissions bitfield). This is the
// bitwise OR of the Discord permission bitflags LiveReview needs, so it can be
// recalculated if the bot's permissions ever change:
//
//	VIEW_CHANNEL            = 1024
//	SEND_MESSAGES           = 2048
//	EMBED_LINKS             = 16384
//	ATTACH_FILES            = 32768
//	READ_MESSAGE_HISTORY    = 65536
//	SEND_MESSAGES_IN_THREADS= 274877906944
//
//	1024 | 2048 | 16384 | 32768 | 65536 | 274877906944 = 274878024704
//
// Used in the generated OAuth2 invite URL (permissions=<integer>).
const DISCORD_BOT_PERMISSIONS = 274878024704;

// Base URL template for the OAuth2 bot invite link. {client_id} is replaced
// with the user's Discord Application ID.
const DISCORD_INVITE_URL = (clientId: string) =>
    `https://discord.com/oauth2/authorize?client_id=${clientId}&scope=bot+applications.commands&permissions=${DISCORD_BOT_PERMISSIONS}`;

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
    const [appId, setAppId] = useState('');

    const loadConfig = useCallback(async () => {
        if (!currentOrg) return;
        setLoading(true);
        setError(null);
        try {
            const response = await apiClient.get<DiscordConfig>(`/orgs/${currentOrg.id}/discord-config`);
            setConfig(response);
        } catch {
            // Treat load failures as "not configured" but surface the error so
            // network/server problems aren't silently masked.
            setConfig({ configured: false });
            setError('Failed to load Discord configuration.');
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
                { bot_token: botToken, application_id: appId }
            );
            setConfig(response);
            setEditMode(false);
            setBotToken('');
            setAppId('');
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
                            <div className="mt-3 space-y-3">
                                <div className="mt-3 flex items-center justify-between gap-3 flex-wrap">
                                    <div className="flex items-center space-x-3 flex-wrap gap-y-2">
                                        {config.application_id && (
                                            <span className="text-xs text-slate-500">
                                                Application ID: <code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">{config.application_id}</code>
                                            </span>
                                        )}
                                    </div>
                                    <div className="flex items-center space-x-3">
                                        <Button size="sm" variant="outline" className="!px-2 !py-1 border-blue-500/60 text-blue-300 hover:border-blue-400 hover:bg-blue-500/10 hover:text-blue-200" onClick={() => {
                                            setEditMode(true);
                                            setAppId(config.application_id || '');
                                            setBotToken('');
                                        }}>
                                            Edit
                                        </Button>
                                        <Button size="sm" variant="outline" className="!px-2 !py-1 border-red-500/60 text-red-300 hover:border-red-400 hover:bg-red-500/10 hover:text-red-200" onClick={handleDisconnect}>
                                            Disconnect
                                        </Button>
                                    </div>
                                </div>
                                {config.application_id && (
                                    <div className="space-y-2">
                                        <a
                                            href={DISCORD_INVITE_URL(config.application_id)}
                                            target="_blank"
                                            rel="noopener noreferrer"
                                            className="inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors"
                                        >
                                            Invite bot to your server
                                        </a>
                                    </div>
                                )}
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
                                <div>
                                    <label className="block text-xs font-medium text-slate-400 mb-1">Application ID</label>
                                    <input
                                        type="text"
                                        value={appId}
                                        onChange={(e) => setAppId(e.target.value)}
                                        placeholder="e.g. 123456789012345678"
                                        className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                                    />
                                    <p className="text-xs text-slate-500 mt-1">
                                        Found on the app's <strong className="text-slate-300">General Information</strong> tab in the Discord Developer Portal.
                                    </p>
                                </div>
                                <div className="text-xs text-slate-400 space-y-2">
                                    <p className="font-medium text-slate-300">Step-by-step setup:</p>
                                    <ol className="list-decimal list-inside space-y-1.5">
                                        <li>
                                            Go to the{' '}
                                            <a href="https://discord.com/developers/applications" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300 underline">
                                                Discord Developer Portal
                                            </a>
                                            {' '}and click <strong className="text-slate-200">New Application</strong>, Name it <strong className="text-slate-200">Livi</strong>, then click Create.
                                        </li>
                                        <li>
                                            Click <strong className="text-slate-200">Bot</strong> on the left sidebar.
                                        </li>
                                        <li>
                                            Under the <strong className="text-slate-200">Privileged Gateway Intents</strong> section, enable the below and press save changes:
                                            <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5 text-slate-500">
                                                <li><code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">MESSAGE CONTENT INTENT</code> — required to read message content</li>
                                                <li><code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">SERVER MEMBERS INTENT</code> — required to see guild members</li>
                                            </ul>
                                        </li>
                                        <li>
                                            Click <strong className="text-slate-200">Reset Token</strong>, then paste the bot token above and click <strong className="text-slate-200">Save</strong>.
                                        </li>
                                        <li>
                                            Copy your <strong className="text-slate-200">Application ID</strong> from the <strong className="text-slate-200">General Information</strong> tab and paste it above.
                                        </li>
                                        <li>
                                            After saving, click <strong className="text-slate-200">Invite bot to your server</strong>, choose your server, and authorize.
                                        </li>
                                        <li>
                                            DM the bot or mention it with <code className="px-1.5 py-0.5 bg-slate-900/60 border border-slate-600 text-slate-100 rounded">@YourBotName your question</code> in a channel to start.
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
                                                setAppId('');
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
