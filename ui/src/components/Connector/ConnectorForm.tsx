import React, { useState, useEffect } from 'react';
import { Card, Input, Select, Button, Icons, Alert, Avatar } from '../UIPrimitives';
import { ConnectorType } from '../../store/Connector/reducer';
import GitLabConnector from './GitLabConnector';
import DomainValidator from './DomainValidator';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import { isDuplicateConnector, normalizeUrl } from './checkConnectorDuplicate';
import { validateGitLabProfile } from '../../api/gitlabProfile';

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
        if (type === 'gitlab-com') {
            const hasGitlabComConnector = connectors.some((connector: any) => {
                const connectorUrl = normalizeUrl(connector.url || '');
                const metadataProviderUrl = connector.metadata?.provider_url ? normalizeUrl(connector.metadata.provider_url) : '';
                return connectorUrl.includes('gitlab.com') || metadataProviderUrl.includes('gitlab.com');
            });
            if (hasGitlabComConnector) {
                setErrorMessage('You already have a GitLab.com connection');
                return;
            }
            setErrorMessage(null);
        }
        setSelectedConnectorType(type);
        setShowConnectorForm(true);
        navigate(`/git/${type}/step1`);
    };

    const handleGitLabSubmit = (data: ConnectorData) => {
        const connectorWithMeta = {
            ...data,
            id: `connector-${Date.now()}`,
            createdAt: Date.now(),
        };
        onSubmit(connectorWithMeta);
        setShowConnectorForm(false);
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
                onClick={() => setTab('automated')}
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
        return (
            <Card title="Manual GitLab.com Connector">
                {!profile && !confirming && (
                    <form className="space-y-4" onSubmit={async e => {
                        e.preventDefault();
                        setProfileError(null);
                        setConfirming(true);
                        try {
                            const result = await validateGitLabProfile('https://gitlab.com', pat);
                            setProfile(result);
                        } catch (err: any) {
                            setProfileError(err.message);
                        } finally {
                            setConfirming(false);
                        }
                    }}>
                        <Input id="manual-username" label="Username" value={username} onChange={e => setUsername(e.target.value)} required />
                        <Input id="manual-pat" label="Personal Access Token (PAT)" type="password" value={pat} onChange={e => setPat(e.target.value)} required />
                        {profileError && (
                            <Alert variant="error" title="Validation Error" onClose={() => setProfileError(null)}>{profileError}</Alert>
                        )}
                        <Button variant="primary" type="submit" disabled={confirming}>{confirming ? 'Validating...' : 'Add Connector'}</Button>
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
                        <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                            Please confirm this is your GitLab profile before saving the connector.
                        </div>
                        <div className="flex space-x-3 pt-2">
                            <Button variant="primary" size="lg" className="font-bold px-6 py-2" onClick={() => {
                                handleGitLabSubmit({
                                    name: 'GitLab.com (Manual)',
                                    type: 'gitlab-com',
                                    url: 'https://gitlab.com',
                                    apiKey: pat,
                                    metadata: {
                                        username,
                                        manual: true,
                                        gitlabProfile: profile,
                                    },
                                });
                                setProfile(null);
                            }}>Confirm & Save</Button>
                            <Button variant="outline" size="lg" className="px-6 py-2" onClick={() => setProfile(null)}>Cancel</Button>
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
        return (
            <Card title="Manual Self-Hosted GitLab Connector">
                {!profile && !confirming && (
                    <form className="space-y-4" onSubmit={async e => {
                        e.preventDefault();
                        setProfileError(null);
                        setConfirming(true);
                        try {
                            const result = await validateGitLabProfile(url, pat);
                            setProfile(result);
                        } catch (err: any) {
                            setProfileError(err.message);
                        } finally {
                            setConfirming(false);
                        }
                    }}>
                        <Input id="manual-username" label="Username" value={username} onChange={e => setUsername(e.target.value)} required />
                        <Input id="manual-pat" label="Personal Access Token (PAT)" type="password" value={pat} onChange={e => setPat(e.target.value)} required />
                        <Input id="manual-url" label="Instance URL" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://gitlab.mycompany.com" required />
                        {profileError && (
                            <Alert variant="error" title="Validation Error" onClose={() => setProfileError(null)}>{profileError}</Alert>
                        )}
                        <Button variant="primary" type="submit" disabled={confirming}>{confirming ? 'Validating...' : 'Add Connector'}</Button>
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
                        <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                            Please confirm this is your GitLab profile before saving the connector.
                        </div>
                        <div className="flex space-x-3 pt-2">
                            <Button variant="primary" size="lg" className="font-bold px-6 py-2" onClick={() => {
                                handleGitLabSubmit({
                                    name: 'Self-Hosted GitLab (Manual)',
                                    type: 'gitlab-self-hosted',
                                    url,
                                    apiKey: pat,
                                    metadata: {
                                        username,
                                        manual: true,
                                        gitlabProfile: profile,
                                    },
                                });
                                setProfile(null);
                            }}>Confirm & Save</Button>
                            <Button variant="outline" size="lg" className="px-6 py-2" onClick={() => setProfile(null)}>Cancel</Button>
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
