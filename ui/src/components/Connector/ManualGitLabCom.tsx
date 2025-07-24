import React, { useState } from 'react';
import { Card, Input, Button, Avatar } from '../UIPrimitives';
import { validateGitLabProfile } from '../../api/gitlabProfile';
import { createPATConnector } from '../../api/patConnector';
import { getConnectors } from '../../api/connectors';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { useNavigate } from 'react-router-dom';

const ManualGitLabCom: React.FC = () => {
    const dispatch = useDispatch();
    const navigate = useNavigate();
    const [username, setUsername] = useState('');
    const [pat, setPat] = useState('');
    const [profile, setProfile] = useState<any | null>(null);
    const [profileError, setProfileError] = useState<string | null>(null);
    const [confirming, setConfirming] = useState(false);
    const [saving, setSaving] = useState(false);

    const handleSaveConnector = async () => {
        setSaving(true);
        try {
            // Call backend API to save PAT connector
            await createPATConnector({
                name: username,
                type: 'gitlab-com',
                url: 'https://gitlab.com',
                pat_token: pat,
                metadata: {
                    manual: true,
                    gitlabProfile: profile,
                },
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
            // Navigate back to Git providers list
            navigate('/git');
        } catch (err: any) {
            console.error('Failed to save connector:', err);
            // You might want to show an error message to the user here
        } finally {
            setSaving(false);
        }
    };

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
                        setProfileError('Failed to validate GitLab credentials');
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
                    <div className="rounded-md bg-slate-800 text-slate-300 px-4 py-2 text-sm mb-2" style={{border: '1px solid #334155'}}>
                        Please confirm this is your GitLab profile before saving the connector.
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

export default ManualGitLabCom;
