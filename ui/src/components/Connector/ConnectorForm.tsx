import React, { useState, useEffect } from 'react';
import { Card, Input, Select, Button, Icons, Alert, Avatar } from '../UIPrimitives';
import { ConnectorType } from '../../store/Connector/reducer';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import GitLabConnector from './GitLabConnector';
import DomainValidator from './DomainValidator';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import { isDuplicateConnector, normalizeUrl } from './checkConnectorDuplicate';
import { validateGitLabProfile } from '../../api/gitlabProfile';
import { createPATConnector } from '../../api/patConnector';
import { getConnectors } from '../../api/connectors';

type ConnectorFormProps = {
    onSubmit: (connector: ConnectorData) => void;
};

export type ConnectorData = {
    name: string;
    type: ConnectorType;
    url: string;
    apiKey: string;
    id?: string;
    createdAt?: number;
    metadata?: any;
};

export const ConnectorForm: React.FC<ConnectorFormProps> = ({ onSubmit }) => {
    const navigate = useNavigate();
    const location = useLocation();
    const { providerType } = useParams<{ providerType?: string }>();
    const [selectedConnectorType, setSelectedConnectorType] = useState<ConnectorType>('gitlab-com');
    const [showConnectorForm, setShowConnectorForm] = useState<boolean>(false);
    const [errorMessage, setErrorMessage] = useState<string | null>(null);
    const [tab, setTab] = useState<'automated' | 'manual'>('manual');
    // Manual form state
    const [manualForm, setManualForm] = useState({
        username: '',
        pat: '',
        url: '',
    });
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [saving, setSaving] = useState(false);
    // Get connectors from Redux state
    const connectors = useAppSelector((state) => state.Connector.connectors);

    useEffect(() => {
        if (providerType === 'gitlab-com') {
            setSelectedConnectorType('gitlab-com');
            setShowConnectorForm(true);
        } else if (providerType === 'gitlab-self-hosted') {
            setSelectedConnectorType('gitlab-self-hosted');
            setShowConnectorForm(true);
        } else {
            setShowConnectorForm(false);
        }
    }, [providerType]);

    const handleConnectorSelect = (type: ConnectorType) => {
    setSelectedConnectorType(type);
    setShowConnectorForm(true);
    navigate(`/git/${type}/step1`);
    };

    const dispatch = useDispatch();
    const handleGitLabSubmit = async (data: ConnectorData) => {
        setSaving(true);
        setErrorMessage(null);
        try {
            // Call backend API to save PAT connector
            await createPATConnector({
                name: data.name,
                type: data.type,
                url: data.url,
                pat_token: data.apiKey,
                metadata: data.metadata,
            });
            // Refresh connector list in frontend and update Redux state
            const updatedConnectorsRaw = await getConnectors();
            // Map backend response to frontend Redux shape
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
            setShowConnectorForm(false);
        } catch (err: any) {
            setErrorMessage(err.message || 'Failed to save connector');
        } finally {
            setSaving(false);
        }
    };

    const handleBackToSelection = () => {
        setShowConnectorForm(false);
        navigate('/git');
    };

    // Tab switcher UI
    const TabSwitcher = () => (
        <div className="flex space-x-2 mb-4">
            <Button
                variant={tab === 'manual' ? 'primary' : 'outline'}
                onClick={() => setTab('manual')}
            >
                Manual
            </Button>
            <Button
                variant={tab === 'automated' ? 'primary' : 'outline'}
                onClick={() => {
                    // Only check for duplicate connector when switching to Automated tab, and only for token_type 'Bearer'
                    if (selectedConnectorType === 'gitlab-com') {
                        const hasGitlabComConnector = connectors.some((connector: any) => {
                            const connectorUrl = normalizeUrl(connector.url || '');
                            const metadataProviderUrl = connector.metadata?.provider_url ? normalizeUrl(connector.metadata.provider_url) : '';
                            // Only consider connectors with token_type 'Bearer'
                            const tokenType = connector.metadata?.token_type || connector.token_type || '';
                            return (tokenType === 'Bearer') && (connectorUrl.includes('gitlab.com') || metadataProviderUrl.includes('gitlab.com'));
                        });
                        if (hasGitlabComConnector) {
                            setErrorMessage('You already have an Automated GitLab.com connection (token_type: Bearer)');
                            return;
                        }
                    }
                    if (selectedConnectorType === 'gitlab-self-hosted') {
                        const hasSelfHostedConnector = connectors.some((connector: any) => {
                            const connectorUrl = normalizeUrl(connector.url || '');
                            const metadataProviderUrl = connector.metadata?.provider_url ? normalizeUrl(connector.metadata.provider_url) : '';
                            const tokenType = connector.metadata?.token_type || connector.token_type || '';
                            // Only consider connectors with token_type 'Bearer' and not gitlab.com
                            return (tokenType === 'Bearer') && ((connectorUrl && !connectorUrl.includes('gitlab.com')) || (metadataProviderUrl && !metadataProviderUrl.includes('gitlab.com')));
                        });
                        if (hasSelfHostedConnector) {
                            setErrorMessage('You already have an Automated Self-Hosted GitLab connection (token_type: Bearer)');
                            return;
                        }
                    }
                    setErrorMessage(null); // Clear error on successful tab switch
                    setTab('automated');
                }}
            >
                Automated
            </Button>
        </div>
    );

    // Manual GitLab.com form
    const ManualGitLabForm = () => {
    const [username, setUsername] = useState('');
    const [pat, setPat] = useState('');
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const usernameHasLiveReview = username.toLowerCase().includes('livereview');
        return (
            <Card title="Manual GitLab.com Connector">
                <div className="mb-4 rounded-md bg-yellow-900 text-yellow-200 px-4 py-3 border border-yellow-400 text-base font-semibold">
                    <span className="font-bold">Recommended:</span> For best practice, create a dedicated GitLab user such as <span className="font-bold text-yellow-100">LiveReviewBot</span> and grant it access to all projects/groups where you want AI code reviews. This helps with security, auditability, and permission management.
                </div>
                {!profile && (
                    <form className="space-y-4" onSubmit={async e => {
                        e.preventDefault();
                        setProfileError(null);
                        setConfirming(true);
                        try {
                            const result = await validateGitLabProfile('https://gitlab.com', pat);
                            setProfile(result);
                        } catch (err: any) {
                            console.error('GitLab validation error:', err);
                            // Extract the actual error message from the server response
                            let errorMessage = 'Failed to validate GitLab credentials';
                            if (err.data && err.data.error) {
                                errorMessage = err.data.error;
                            } else if (err.message && !err.message.includes('API error') && !err.message.includes('Request failed')) {
                                errorMessage = err.message;
                            } else if (err.status === 400) {
                                errorMessage = 'Invalid GitLab URL or Personal Access Token. Please check your credentials and try again.';
                            } else if (err.status >= 500) {
                                errorMessage = 'GitLab server error. Please try again later.';
                            }
                            setProfileError(errorMessage);
                        } finally {
                            setConfirming(false);
                        }
                    }}>
                        <Input
                            id="manual-username"
                            label="Connector Name"
                            value={username}
                            onChange={e => setUsername(e.target.value)}
                            required
                            helperText="Tip: Give this connector a descriptive name for your reference."
                        />
                        <Input
                            id="manual-pat"
                            label="Personal Access Token (PAT)"
                            type="password"
                            value={pat}
                            onChange={e => setPat(e.target.value)}
                            required
                            helperText="Ensure this user has sufficient project/group access for all repositories where you want AI code reviews."
                        />
                        {profileError && (
                            <div className="rounded-md bg-red-900 border border-red-700 px-4 py-3">
                                <div className="flex items-start">
                                    <div className="ml-3 flex-1">
                                        <h3 className="text-sm font-medium text-red-200">GitLab Connection Failed</h3>
                                        <div className="mt-1 text-sm text-red-300">{profileError}</div>
                                        <div className="mt-2 text-xs text-red-400">
                                            Make sure your Personal Access Token has 'api' scope and the GitLab instance is accessible.
                                        </div>
                                    </div>
                                    <button
                                        type="button"
                                        className="ml-auto flex-shrink-0 text-red-400 hover:text-red-300 text-lg font-bold"
                                        onClick={() => setProfileError(null)}
                                    >
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
                            {confirming ? 'Validating GitLab Connection...' : 'Add Connector'}
                        </Button>
                    </form>
                )}
                {profile && (
                    <div className="space-y-6">
                        <div className="flex items-center space-x-5">
                            {profile.avatar_url && (<Avatar src={profile.avatar_url} size="xl" />)}
                            <div>
                                <div className="font-extrabold text-2xl text-white">{profile.name}</div>
                                <div className="text-base text-blue-300 font-semibold">@{profile.username}</div>
                                <div className="text-sm text-slate-400 mt-1">{profile.email}</div>
                            </div>
                        </div>
                        {!(profile.username?.toLowerCase().includes('livereview') || profile.name?.toLowerCase().includes('livereview')) ? (
                            <div className="rounded-md bg-yellow-900 text-yellow-300 px-4 py-2 text-sm mb-2 border border-yellow-400">
                                <strong>Recommended:</strong> For best security and auditability, create a dedicated GitLab user (e.g. <span className="font-bold text-yellow-200">LiveReviewBot</span>) with all required project/group access for AI code reviews. This helps you manage permissions and track review activity.
                            </div>
                        ) : (
                            <div className="rounded-md bg-green-900 text-green-300 px-4 py-2 text-sm mb-2 border border-green-400">
                                <strong>Good!</strong> Your GitLab user is correctly named for LiveReview integration.
                            </div>
                        )}
                        <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                            Please confirm this is your GitLab profile before saving the connector.
                        </div>
                        <div className="flex space-x-3 pt-2">
                            <Button 
                                variant="primary" 
                                size="lg" 
                                className="font-bold px-6 py-2" 
                                onClick={async () => {
                                    await handleGitLabSubmit({
                                        name: username,
                                        type: 'gitlab-com',
                                        url: 'https://gitlab.com',
                                        apiKey: pat,
                                        metadata: {
                                            manual: true,
                                            gitlabProfile: profile,
                                        },
                                    });
                                    setProfile(null);
                                }} 
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

    // Manual Self-Hosted GitLab form
    const ManualSelfHostedForm = () => {
    const [username, setUsername] = useState('');
    const [pat, setPat] = useState('');
    const [url, setUrl] = useState('');
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const usernameHasLiveReview = username.toLowerCase().includes('livereview');
        return (
            <Card title="Manual Self-Hosted GitLab Connector">
                <div className="mb-4 rounded-md bg-yellow-900 text-yellow-200 px-4 py-3 border border-yellow-400 text-base font-semibold">
                    <span className="font-bold">Recommended:</span> For best practice, create a dedicated GitLab user such as <span className="font-bold text-yellow-100">LiveReviewBot</span> and grant it access to all projects/groups where you want AI code reviews. This helps with security, auditability, and permission management.
                </div>
                {!profile && (
                    <form className="space-y-4" onSubmit={async e => {
                        e.preventDefault();
                        setProfileError(null);
                        setConfirming(true);
                        try {
                            const result = await validateGitLabProfile(url, pat);
                            setProfile(result);
                        } catch (err: any) {
                            console.error('GitLab validation error:', err);
                            // Extract the actual error message from the server response
                            let errorMessage = 'Failed to validate GitLab credentials';
                            if (err.data && err.data.error) {
                                errorMessage = err.data.error;
                            } else if (err.message && !err.message.includes('API error') && !err.message.includes('Request failed')) {
                                errorMessage = err.message;
                            } else if (err.status === 400) {
                                errorMessage = 'Invalid GitLab URL or Personal Access Token. Please verify the instance URL is correct and your PAT has api scope.';
                            } else if (err.status >= 500) {
                                errorMessage = 'GitLab server error. Please try again later.';
                            }
                            setProfileError(errorMessage);
                        } finally {
                            setConfirming(false);
                        }
                    }}>
                        <Input
                            id="manual-username"
                            label="Connector Name"
                            value={username}
                            onChange={e => setUsername(e.target.value)}
                            required
                            helperText="Tip: Give this connector a descriptive name for your reference."
                        />
                        <Input
                            id="manual-pat"
                            label="Personal Access Token (PAT)"
                            type="password"
                            value={pat}
                            onChange={e => setPat(e.target.value)}
                            required
                            helperText="Ensure this user has sufficient project/group access for all repositories where you want AI code reviews."
                        />
                        <Input
                            id="manual-url"
                            label="Instance URL"
                            value={url}
                            onChange={e => setUrl(e.target.value)}
                            placeholder="https://gitlab.mycompany.com"
                            required
                        />
                        {profileError && (
                            <div className="rounded-md bg-red-900 border border-red-700 px-4 py-3">
                                <div className="flex items-start">
                                    <div className="ml-3 flex-1">
                                        <h3 className="text-sm font-medium text-red-200">GitLab Connection Failed</h3>
                                        <div className="mt-1 text-sm text-red-300">{profileError}</div>
                                        <div className="mt-2 text-xs text-red-400">
                                            Verify your instance URL is correct (no trailing slash) and your PAT has 'api' scope.
                                        </div>
                                    </div>
                                    <button
                                        type="button"
                                        className="ml-auto flex-shrink-0 text-red-400 hover:text-red-300 text-lg font-bold"
                                        onClick={() => setProfileError(null)}
                                    >
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
                            {confirming ? 'Validating GitLab Connection...' : 'Add Connector'}
                        </Button>
                    </form>
                )}
                {profile && (
                    <div className="space-y-6">
                        <div className="flex items-center space-x-5">
                            {profile.avatar_url && (<Avatar src={profile.avatar_url} size="xl" />)}
                            <div>
                                <div className="font-extrabold text-2xl text-white">{profile.name}</div>
                                <div className="text-base text-blue-300 font-semibold">@{profile.username}</div>
                                <div className="text-sm text-slate-400 mt-1">{profile.email}</div>
                            </div>
                        </div>
                        {!(profile.username?.toLowerCase().includes('livereview') || profile.name?.toLowerCase().includes('livereview')) ? (
                            <div className="rounded-md bg-yellow-900 text-yellow-300 px-4 py-2 text-sm mb-2 border border-yellow-400">
                                <strong>Recommended:</strong> For best security and auditability, create a dedicated GitLab user (e.g. <span className="font-bold text-yellow-200">LiveReviewBot</span>) with all required project/group access for AI code reviews. This helps you manage permissions and track review activity.
                            </div>
                        ) : (
                            <div className="rounded-md bg-green-900 text-green-300 px-4 py-2 text-sm mb-2 border border-green-400">
                                <strong>Good!</strong> Your GitLab user is correctly named for LiveReview integration.
                            </div>
                        )}
                        <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                            Please confirm this is your GitLab profile before saving the connector.
                        </div>
                        <div className="flex space-x-3 pt-2">
                            <Button 
                                variant="primary" 
                                size="lg" 
                                className="font-bold px-6 py-2" 
                                onClick={async () => {
                                    await handleGitLabSubmit({
                                        name: username,
                                        type: 'gitlab-self-hosted',
                                        url,
                                        apiKey: pat,
                                        metadata: {
                                            manual: true,
                                            gitlabProfile: profile,
                                        },
                                    });
                                    setProfile(null);
                                }} 
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

    // Show connector selection screen
    if (!showConnectorForm) {
        return (
            <DomainValidator>
                <Card title="Create New Connector">
                    <div className="space-y-5">
                        <h3 className="text-lg font-medium text-white">Select Git Provider</h3>
                        <p className="text-slate-300 text-sm">Choose a Git provider to connect with LiveReview</p>
                        {errorMessage && (
                            <Alert variant="error" title="Connection Error" onClose={() => setErrorMessage(null)}>
                                {errorMessage}
                            </Alert>
                        )}
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                            <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => handleConnectorSelect('gitlab-com')}>
                                <span className="text-base mt-2">GitLab.com</span>
                            </Button>
                            <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => handleConnectorSelect('gitlab-self-hosted')}>
                                <span className="text-base mt-2">Self-Hosted GitLab</span>
                            </Button>
                            <Button variant="outline" icon={<Icons.GitHub />} className="h-24 flex-col" disabled>
                                <span className="text-base mt-2">GitHub</span>
                                <span className="text-xs mt-1">Coming Soon</span>
                            </Button>
                            <Button variant="outline" icon={<Icons.Git />} className="h-24 flex-col" disabled>
                                <span className="text-base mt-2">Custom</span>
                                <span className="text-xs mt-1">Coming Soon</span>
                            </Button>
                        </div>
                    </div>
                </Card>
            </DomainValidator>
        );
    }

    // Show connector form with tab switcher
    if (selectedConnectorType === 'gitlab-com' || selectedConnectorType === 'gitlab-self-hosted') {
        return (
            <div className="space-y-4">
                <div className="flex items-center">
                    <Button variant="ghost" icon={<Icons.Add />} onClick={handleBackToSelection} iconPosition="left" className="text-sm">Back to providers</Button>
                </div>
                <TabSwitcher />
                {tab === 'manual' && (
                    selectedConnectorType === 'gitlab-com' ? <ManualGitLabForm /> : <ManualSelfHostedForm />
                )}
                {tab === 'automated' && (
                    <GitLabConnector type={selectedConnectorType} onSubmit={handleGitLabSubmit} />
                )}
            </div>
        );
    }

    // Placeholder for other connector types (GitHub, Custom, etc.)
    return (
        <Card title="Coming Soon">
            <div className="space-y-4">
                <p className="text-slate-300">This connector type is not yet available.</p>
                <Button variant="primary" onClick={handleBackToSelection}>Back to Selection</Button>
            </div>
        </Card>
    );
};
