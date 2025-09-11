import React, { useState } from 'react';
import { Card, Input, Button, Avatar, Popover, Icons } from '../UIPrimitives';
import { validateBitbucketProfile } from '../../api/bitbucketProfile';
import { createPATConnector } from '../../api/patConnector';
import { getConnectors } from '../../api/connectors';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { useNavigate } from 'react-router-dom';

const ManualBitbucketConnector: React.FC = () => {
    const dispatch = useDispatch();
    const navigate = useNavigate();
    const [connectorName, setConnectorName] = useState('');
    const [email, setEmail] = useState('');
    const [apiToken, setApiToken] = useState('');
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [saving, setSaving] = useState(false);

    const handleSaveConnector = async () => {
        setSaving(true);
        try {
            await createPATConnector({
                name: connectorName,
                type: 'bitbucket',
                url: 'https://bitbucket.org',
                pat_token: apiToken,
                metadata: {
                    manual: true,
                    email: email,
                    bitbucketProfile: profile,
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

    const checkForLiveReviewInName = (name: string | null | undefined): boolean => {
        if (!name) return false;
        return name.toLowerCase().includes('livereview');
    };

    return (
        <Card title="Manual Bitbucket Connector">
            <div className="mb-4 rounded-md bg-blue-900 text-blue-200 px-4 py-3 border border-blue-400 text-sm">
                <span className="font-semibold">Note:</span> Atlassian is transitioning from App Passwords to API Tokens. For best practice, create a dedicated user (e.g., LiveReviewBot) with repository access. Generate tokens at{' '}
                <a 
                    href="https://id.atlassian.com/manage-profile/security/api-tokens" 
                    target="_blank" 
                    rel="noopener noreferrer"
                    className="text-blue-100 hover:text-white underline font-medium"
                >
                    id.atlassian.com
                </a>.
            </div>

            {!profile && (
                                <form className="space-y-4" onSubmit={async e => {
                    e.preventDefault();
                    setProfileError(null);
                    setConfirming(true);
                    try {
                        const result = await validateBitbucketProfile(email, apiToken);
                        setProfile(result);
                    } catch (err: any) {
                        setProfileError('Failed to validate Bitbucket credentials');
                    } finally {
                        setConfirming(false);
                    }
                }}>
                    <Input
                        id="manual-connector-name"
                        label="Connector Name"
                        value={connectorName}
                        onChange={e => setConnectorName(e.target.value)}
                        required
                        helperText="Tip: Give this connector a descriptive name for your reference."
                    />
                    <Input
                        id="manual-email"
                        label="Atlassian Account Email"
                        type="email"
                        value={email}
                        onChange={e => setEmail(e.target.value)}
                        required
                        helperText="Your Atlassian account email address (e.g., john@example.com)."
                    />
                    <div>
                        <div className="flex items-center space-x-3 mb-2">
                            <label className="block text-sm font-medium text-slate-300">Atlassian API Token</label>
                            <div className="flex items-center space-x-2 bg-blue-600 hover:bg-blue-500 px-3 py-1.5 rounded-lg transition-colors cursor-pointer">
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
                                        <p className="text-slate-200 font-medium text-sm">Bitbucket API Token Setup</p>
                                        <p className="text-xs text-slate-400 leading-relaxed">
                                            Generate an API Token from your Atlassian account (replaces App Passwords).
                                        </p>
                                        <ul className="text-xs text-slate-300 list-disc pl-4 space-y-1">
                                            <li>Generate at: <code className="text-green-400 bg-slate-700 px-1 rounded text-xs">id.atlassian.com</code></li>
                                            <li>Grant repo read + pull request read permissions</li>
                                            <li>Use a dedicated service user (recommended)</li>
                                        </ul>
                                        <div className="pt-1">
                                            <a
                                                href="https://github.com/HexmosTech/LiveReview/wiki/BitBucket"
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="text-blue-400 hover:text-blue-300 underline text-xs font-medium"
                                            >
                                                Open full guide ↗
                                            </a>
                                        </div>
                                    </div>
                                </Popover>
                            </div>
                        </div>
                        <Input
                            id="manual-api-token"
                            type="password"
                            value={apiToken}
                            onChange={e => setApiToken(e.target.value)}
                            required
                            helperText="Generate an API Token at https://id.atlassian.com/manage-profile/security/api-tokens"
                        />
                    </div>
                    {profileError && (
                        <div className="rounded-md bg-red-900 border border-red-700 px-4 py-3">
                            <div className="flex items-start">
                                <div className="ml-3 flex-1">
                                    <h3 className="text-sm font-medium text-red-200">Bitbucket Connection Failed</h3>
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
                        {confirming ? 'Validating Bitbucket Connection...' : 'Add Connector'}
                    </Button>
                </form>
            )}
            {profile && (
                <div className="space-y-6">
                    <div className="flex items-center space-x-5">
                        {profile.links?.avatar?.href && (<Avatar src={profile.links.avatar.href} size="xl" />)}
                        <div>
                            <div className="font-extrabold text-2xl text-white">{profile.display_name || profile.nickname}</div>
                            <div className="text-base text-blue-300 font-semibold">@{profile.nickname}</div>
                            {profile.account_id && (
                                <div className="text-sm text-slate-400 mt-1">Account ID: {profile.account_id}</div>
                            )}
                            {profile.website && (
                                <div className="text-sm text-slate-400">{profile.website}</div>
                            )}
                            {profile.location && (
                                <div className="text-sm text-slate-400">{profile.location}</div>
                            )}
                        </div>
                    </div>
                    {checkForLiveReviewInName(profile.display_name) && (
                        <div className="rounded-md bg-green-900 border border-green-700 px-4 py-3">
                            <div className="flex items-start">
                                <div className="ml-3 flex-1">
                                    <h3 className="text-sm font-medium text-green-200">Great Choice! 🎉</h3>
                                    <div className="mt-1 text-sm text-green-300">
                                        We notice "LiveReview" in the profile name. This suggests you're following best practices by using a dedicated account for code reviews.
                                    </div>
                                </div>
                            </div>
                        </div>
                    )}
                    <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                        Please confirm this is your Bitbucket profile before saving the connector.
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

export default ManualBitbucketConnector;
