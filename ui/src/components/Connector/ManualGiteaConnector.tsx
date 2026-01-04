import React, { useState } from 'react';
import { Card, Input, Button, Avatar, Popover, Icons } from '../UIPrimitives';
import { validateGiteaProfile } from '../../api/giteaProfile';
import { createPATConnector } from '../../api/patConnector';
import { getConnectors } from '../../api/connectors';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { useNavigate } from 'react-router-dom';

const ManualGiteaConnector: React.FC = () => {
    const dispatch = useDispatch();
    const navigate = useNavigate();
    const [connectorName, setConnectorName] = useState('');
    const [baseURL, setBaseURL] = useState('');
    const [pat, setPat] = useState('');
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [saving, setSaving] = useState(false);

    const normalizeBaseURL = (url: string) => url.trim().replace(/\/+$/, '');

    const handleSaveConnector = async () => {
        setSaving(true);
        try {
            const normalizedURL = normalizeBaseURL(baseURL);
            await createPATConnector({
                name: connectorName || profile?.full_name || profile?.login || 'Gitea Connector',
                type: 'gitea',
                url: normalizedURL,
                pat_token: pat,
                metadata: {
                    manual: true,
                    giteaProfile: profile,
                },
            });
            const updatedConnectorsRaw = await getConnectors();
            const updatedConnectors = updatedConnectorsRaw.map((c: any) => ({
                id: c.id?.toString() || '',
                name: c.connection_name || '',
                type: c.provider || '',
                url: c.provider_url || '',
                apiKey: c.provider_app_id || '',
                createdAt: c.created_at || '',
                metadata: c.metadata || {},
            }));
            dispatch(setConnectors(updatedConnectors));
            navigate('/git');
        } catch (err: any) {
            console.error('Failed to save connector:', err);
        } finally {
            setSaving(false);
        }
    };

    return (
        <Card title="Manual Gitea Connector">
            <div className="mb-4 rounded-md bg-emerald-900 text-emerald-100 px-4 py-3 border border-emerald-500 text-sm">
                <span className="font-semibold">Heads up:</span> Use your Gitea instance URL (e.g., https://gitea.mycompany.com) and a PAT with repository read access. A dedicated service account (e.g., livereview-bot) is recommended.
            </div>

            {!profile && (
                <form className="space-y-4" onSubmit={async e => {
                    e.preventDefault();
                    setProfileError(null);
                    setConfirming(true);
                    try {
                        const normalizedURL = normalizeBaseURL(baseURL);
                        const result = await validateGiteaProfile(normalizedURL, pat);
                        setProfile(result);
                    } catch (err: any) {
                        const message = err?.message || 'Failed to validate Gitea credentials';
                        setProfileError(message);
                    } finally {
                        setConfirming(false);
                    }
                }}>
                    <Input
                        id="manual-gitea-connector-name"
                        label="Connector Name"
                        value={connectorName}
                        onChange={e => setConnectorName(e.target.value)}
                        required
                        helperText="Tip: Give this connector a descriptive name for your reference."
                    />
                    <Input
                        id="manual-gitea-base-url"
                        label="Gitea Base URL"
                        value={baseURL}
                        onChange={e => setBaseURL(e.target.value)}
                        required
                        placeholder="https://gitea.mycompany.com"
                        helperText="Use the full URL of your Gitea instance. Trailing slashes are removed automatically."
                    />
                    <div>
                        <div className="flex items-center space-x-3 mb-2">
                            <label className="block text-sm font-medium text-slate-300">Personal Access Token (PAT)</label>
                            <div className="flex items-center space-x-2 bg-emerald-700 hover:bg-emerald-600 px-3 py-1.5 rounded-lg transition-colors cursor-pointer">
                                <Icons.Info />
                                <Popover
                                    hover
                                    trigger={
                                        <span className="text-white font-semibold text-sm">
                                            📋 Setup Guide
                                        </span>
                                    }
                                >
                                    <div className="space-y-2">
                                        <p className="text-slate-200 font-medium text-sm">Gitea PAT Setup</p>
                                        <p className="text-xs text-slate-400 leading-relaxed">
                                            Create a Personal Access Token with repo read permissions on your Gitea instance.
                                        </p>
                                        <ul className="text-xs text-slate-300 list-disc pl-4 space-y-1">
                                            <li>Generate under: <code className="text-green-400 bg-slate-700 px-1 rounded">Settings → Applications</code></li>
                                            <li>Scopes: repository read (and PR read if available)</li>
                                            <li>Use a dedicated service user (recommended)</li>
                                        </ul>
                                    </div>
                                </Popover>
                            </div>
                        </div>
                        <Input
                            id="manual-gitea-pat"
                            type="password"
                            value={pat}
                            onChange={e => setPat(e.target.value)}
                            required
                            helperText="Ensure the PAT has access to all target repositories."
                        />
                    </div>
                    {profileError && (
                        <div className="rounded-md bg-red-900 border border-red-700 px-4 py-3">
                            <div className="flex items-start">
                                <div className="ml-3 flex-1">
                                    <h3 className="text-sm font-medium text-red-200">Gitea Connection Failed</h3>
                                    <div className="mt-1 text-sm text-red-300">{profileError}</div>
                                </div>
                                <button type="button" className="ml-auto flex-shrink-0 text-red-400 hover:text-red-300 text-lg font-bold" onClick={() => setProfileError(null)}>
                                    ×
                                </button>
                            </div>
                        </div>
                    )}
                    <Button 
                        variant="primary" 
                        type="submit" 
                        disabled={confirming}
                        isLoading={confirming}
                    >
                        {confirming ? 'Validating Gitea Connection...' : 'Add Connector'}
                    </Button>
                </form>
            )}
            {profile && (
                <div className="space-y-6">
                    <div className="flex items-center space-x-5">
                        {profile.avatar_url && (<Avatar src={profile.avatar_url} size="xl" />)}
                        <div>
                            <div className="font-extrabold text-2xl text-white">{profile.full_name || profile.login}</div>
                            <div className="text-base text-emerald-300 font-semibold">@{profile.login}</div>
                            {profile.email && (
                                <div className="text-sm text-slate-400 mt-1">{profile.email}</div>
                            )}
                        </div>
                    </div>
                    <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                        Please confirm this is your Gitea profile before saving the connector.
                    </div>
                    <div className="flex space-x-3 pt-2">
                        <Button 
                            variant="primary" 
                            size="lg" 
                            className="font-bold px-6 py-2" 
                            onClick={handleSaveConnector} 
                            disabled={saving}
                            isLoading={saving}
                        >
                            {saving ? 'Saving Connector...' : 'Confirm & Save'}
                        </Button>
                        <Button variant="outline" size="lg" className="px-6 py-2" onClick={() => setProfile(null)} disabled={saving}>Cancel</Button>
                    </div>
                </div>
            )}
        </Card>
    );
};

export default ManualGiteaConnector;
