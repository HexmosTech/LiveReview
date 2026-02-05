import React, { useState, useEffect } from 'react';
import { Button, Icons, Input, Alert, Badge } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseUpgradeDialog from '../../components/License/LicenseUpgradeDialog';
import { useHasLicenseFor, COMMUNITY_TIER_LIMITS } from '../../hooks/useLicenseTier';
import { getApiUrl } from '../../utils/apiUrl';

export interface APIKey {
    id: number;
    key_prefix: string;
    label: string;
    scopes: string[];
    last_used_at: string | null;
    created_at: string;
    expires_at: string | null;
}

interface ListKeysResponse {
    keys: APIKey[];
}

interface CreateKeyResponse {
    api_key: APIKey;
    plain_key: string;
}

const APIKeysTab: React.FC = () => {
    const { currentOrg } = useOrgContext();
    const [keys, setKeys] = useState<APIKey[]>([]);
    const [loading, setLoading] = useState(true);
    const [showCreateForm, setShowCreateForm] = useState(false);
    const [newKeyLabel, setNewKeyLabel] = useState('');
    const [newKeyScopes, setNewKeyScopes] = useState<string[]>([]);
    const [createdKey, setCreatedKey] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
    const hasTeamLicense = useHasLicenseFor('team');

    // Check if adding a new API key requires Team license
    const requiresLicenseForNewKey = () => {
        if (keys.length >= COMMUNITY_TIER_LIMITS.MAX_API_KEYS && !hasTeamLicense) {
            setShowUpgradeDialog(true);
            return true;
        }
        return false;
    };

    useEffect(() => {
        loadKeys();
    }, [currentOrg?.id]);

    const loadKeys = async () => {
        if (!currentOrg) return;
        
        setLoading(true);
        setError(null);
        try {
            const response = await apiClient.get<ListKeysResponse>(`/orgs/${currentOrg.id}/api-keys`);
            setKeys(response.keys || []);
        } catch (err: any) {
            setError(err.message || 'Failed to load API keys');
        } finally {
            setLoading(false);
        }
    };

    const handleCreateKey = async () => {
        if (!currentOrg) return;
        if (!newKeyLabel.trim()) {
            setError('Please enter a label for the API key');
            return;
        }

        if (requiresLicenseForNewKey()) return;

        setError(null);
        setLoading(true);

        try {
            const response = await apiClient.post<CreateKeyResponse>(`/orgs/${currentOrg.id}/api-keys`, {
                label: newKeyLabel,
                scopes: newKeyScopes,
            });

            setCreatedKey(response.plain_key);
            setSuccess('API key created successfully!');
            setShowCreateForm(false);
            setNewKeyLabel('');
            setNewKeyScopes([]);
            await loadKeys();
        } catch (err: any) {
            setError(err.message || 'Failed to create API key');
        } finally {
            setLoading(false);
        }
    };

    const handleRevokeKey = async (keyId: number) => {
        if (!currentOrg) return;
        if (!confirm('Are you sure you want to revoke this API key? This action cannot be undone.')) {
            return;
        }

        setError(null);
        try {
            await apiClient.post(`/orgs/${currentOrg.id}/api-keys/${keyId}/revoke`, {});
            setSuccess('API key revoked successfully');
            await loadKeys();
        } catch (err: any) {
            setError(err.message || 'Failed to revoke API key');
        }
    };

    const handleDeleteKey = async (keyId: number) => {
        if (!currentOrg) return;
        if (!confirm('Are you sure you want to permanently delete this API key?')) {
            return;
        }

        setError(null);
        try {
            await apiClient.delete(`/orgs/${currentOrg.id}/api-keys/${keyId}`);
            setSuccess('API key deleted successfully');
            await loadKeys();
        } catch (err: any) {
            setError(err.message || 'Failed to delete API key');
        }
    };

    const formatDate = (dateString: string | null) => {
        if (!dateString) return 'Never';
        return new Date(dateString).toLocaleString();
    };

    const copyToClipboard = (text: string) => {
        navigator.clipboard.writeText(text);
        setSuccess('API key copied to clipboard!');
        setTimeout(() => setSuccess(null), 2000);
    };

    if (!currentOrg) {
        return (
            <div className="p-4">
                <Alert variant="warning">
                    Please select an organization to manage API keys.
                </Alert>
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <div>
                <h3 className="text-lg font-medium text-white mb-2">API Keys</h3>
                <p className="text-sm text-slate-300 mb-4">
                    Manage personal API keys for programmatic access to LiveReview. Use these keys with the <code className="bg-slate-700 px-1 rounded">lrc</code> CLI tool.
                </p>
            </div>

            {error && (
                <Alert variant="error" onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            {success && (
                <Alert variant="success" onClose={() => setSuccess(null)}>
                    {success}
                </Alert>
            )}

            {createdKey && (
                <div className="bg-green-900 bg-opacity-50 border border-green-600 rounded-lg p-4">
                    <div className="flex items-start space-x-3 mb-3">
                        <div className="text-green-400 mt-0.5">
                            <Icons.Success />
                        </div>
                        <div className="flex-1">
                            <h4 className="text-green-300 font-medium mb-2">API Key Created</h4>
                            <p className="text-green-100 text-sm mb-3">
                                Copy this key now - it won't be shown again!
                            </p>
                            <div className="flex items-center space-x-2 bg-slate-900 rounded p-3 mb-4">
                                <code className="text-green-400 font-mono text-sm flex-1 break-all">
                                    {createdKey}
                                </code>
                                <Button
                                    size="sm"
                                    variant="ghost"
                                    onClick={() => copyToClipboard(createdKey)}
                                >
                                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                                    </svg>
                                </Button>
                            </div>
                            <div className="border-t border-green-700 pt-3 mt-3">
                                <h5 className="text-green-200 font-medium text-sm mb-2">Install the lrc CLI</h5>
                                <p className="text-green-100 text-xs mb-2">
                                    Run this command to install the CLI with your credentials:
                                </p>
                                <div className="space-y-2">
                                    <div>
                                        <p className="text-green-200 text-xs mb-1">Linux/Mac:</p>
                                        <div className="flex items-center space-x-2 bg-slate-900 rounded p-2">
                                            <code className="text-green-300 font-mono text-xs flex-1 break-all">
                                                {`curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${createdKey}" LRC_API_URL="${getApiUrl()}" bash`}
                                            </code>
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                onClick={() => {
                                                    const cmd = `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${createdKey}" LRC_API_URL="${getApiUrl()}" bash`;
                                                    navigator.clipboard.writeText(cmd);
                                                    setSuccess('Command copied to clipboard!');
                                                    setTimeout(() => setSuccess(null), 2000);
                                                }}
                                            >
                                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                                                </svg>
                                            </Button>
                                        </div>
                                    </div>
                                    <div>
                                        <p className="text-green-200 text-xs mb-1">Windows PowerShell:</p>
                                        <div className="flex items-center space-x-2 bg-slate-900 rounded p-2">
                                            <code className="text-green-300 font-mono text-xs flex-1 break-all">
                                                {`$env:LRC_API_KEY="${createdKey}"; $env:LRC_API_URL="${getApiUrl()}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`}
                                            </code>
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                onClick={() => {
                                                    const cmd = `$env:LRC_API_KEY="${createdKey}"; $env:LRC_API_URL="${getApiUrl()}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`;
                                                    navigator.clipboard.writeText(cmd);
                                                    setSuccess('Command copied to clipboard!');
                                                    setTimeout(() => setSuccess(null), 2000);
                                                }}
                                            >
                                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                                                </svg>
                                            </Button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                    <div className="flex justify-end">
                        <Button
                            size="sm"
                            variant="outline"
                            onClick={() => setCreatedKey(null)}
                        >
                            Close
                        </Button>
                    </div>
                </div>
            )}

            <div className="flex justify-between items-center">
                <h4 className="text-md font-medium text-slate-300">Your API Keys</h4>
                <Button
                    variant="primary"
                    size="sm"
                    icon={<Icons.Add />}
                    onClick={() => {
                        if (requiresLicenseForNewKey()) return;
                        setShowCreateForm(!showCreateForm);
                    }}
                >
                    Create New Key
                </Button>
            </div>

            {showCreateForm && (
                <div className="bg-slate-800 rounded-lg p-4 border border-slate-700">
                    <h5 className="text-white font-medium mb-4">Create New API Key</h5>
                    <div className="space-y-4">
                        <Input
                            label="Label"
                            placeholder="e.g., My Dev Machine, CI/CD Pipeline"
                            value={newKeyLabel}
                            onChange={(e) => setNewKeyLabel(e.target.value)}
                            helperText="A descriptive name to identify this key"
                        />
                        <div className="flex space-x-2">
                            <Button
                                variant="primary"
                                onClick={handleCreateKey}
                                disabled={loading || !newKeyLabel.trim()}
                                isLoading={loading}
                            >
                                Create Key
                            </Button>
                            <Button
                                variant="ghost"
                                onClick={() => {
                                    setShowCreateForm(false);
                                    setNewKeyLabel('');
                                    setError(null);
                                }}
                            >
                                Cancel
                            </Button>
                        </div>
                    </div>
                </div>
            )}

            {loading && !showCreateForm ? (
                <div className="flex items-center justify-center py-8">
                    <div className="text-center">
                        <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-2"></div>
                        <p className="text-slate-400">Loading API keys...</p>
                    </div>
                </div>
            ) : keys.length === 0 ? (
                <div className="text-center py-8 bg-slate-800 rounded-lg border border-slate-700">
                    <div className="text-slate-400 mb-2">
                        <svg className="w-12 h-12 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                        </svg>
                    </div>
                    <p className="text-slate-300 mb-1">No API keys yet</p>
                    <p className="text-sm text-slate-400">Create your first API key to get started with the CLI</p>
                </div>
            ) : (
                <div className="space-y-3">
                    {keys.map((key) => (
                        <div
                            key={key.id}
                            className="bg-slate-800 rounded-lg p-4 border border-slate-700 hover:border-slate-600 transition-colors"
                        >
                            <div className="flex items-start justify-between">
                                <div className="flex-1">
                                    <div className="flex items-center space-x-3 mb-2">
                                        <h5 className="text-white font-medium">{key.label}</h5>
                                        <code className="text-xs bg-slate-900 text-slate-400 px-2 py-1 rounded font-mono">
                                            {key.key_prefix}...
                                        </code>
                                    </div>
                                    <div className="grid grid-cols-2 gap-4 text-sm">
                                        <div>
                                            <span className="text-slate-400">Created:</span>
                                            <span className="text-slate-300 ml-2">{formatDate(key.created_at)}</span>
                                        </div>
                                        <div>
                                            <span className="text-slate-400">Last Used:</span>
                                            <span className="text-slate-300 ml-2">{formatDate(key.last_used_at)}</span>
                                        </div>
                                    </div>
                                    {key.expires_at && (
                                        <div className="mt-2">
                                            <Badge variant={new Date(key.expires_at) > new Date() ? 'info' : 'danger'}>
                                                Expires: {formatDate(key.expires_at)}
                                            </Badge>
                                        </div>
                                    )}
                                </div>
                                <div className="flex space-x-2 ml-4">
                                    <Button
                                        size="sm"
                                        variant="ghost"
                                        onClick={() => handleRevokeKey(key.id)}
                                        className="text-amber-400 hover:text-amber-300"
                                    >
                                        Revoke
                                    </Button>
                                    <Button
                                        size="sm"
                                        variant="ghost"
                                        onClick={() => handleDeleteKey(key.id)}
                                        className="text-red-400 hover:text-red-300"
                                    >
                                        <Icons.Delete />
                                    </Button>
                                </div>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            <div className="bg-blue-900 bg-opacity-30 border border-blue-600 rounded-lg p-4 mt-6">
                <div className="flex items-start space-x-3">
                    <div className="text-blue-400 mt-0.5">
                        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                            <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                        </svg>
                    </div>
                    <div>
                        <h5 className="text-blue-300 font-medium mb-2">Using the lrc CLI</h5>
                        <p className="text-blue-100 text-sm mb-2">
                            Use your API key with the <code className="bg-blue-800 px-1 rounded">lrc</code> command-line tool:
                        </p>
                        <pre className="bg-slate-900 text-slate-300 p-3 rounded text-xs overflow-x-auto">
export LRC_API_KEY="your-api-key-here"{'\n'}
lrc --diff-source staged
                        </pre>
                        <p className="text-blue-100 text-sm mt-2">
                            See the <a href="https://github.com/your-repo/lrc" className="underline text-blue-300 hover:text-blue-200">lrc documentation</a> for more details.
                        </p>
                    </div>
                </div>
            </div>

            {/* License Upgrade Dialog */}
            <LicenseUpgradeDialog
                open={showUpgradeDialog}
                onClose={() => setShowUpgradeDialog(false)}
                requiredTier="team"
                featureName="Multiple API Keys"
                featureDescription="Create additional API keys for different environments, CI/CD pipelines, and team members. Community tier includes 1 API key."
            />
        </div>
    );
};

export default APIKeysTab;
