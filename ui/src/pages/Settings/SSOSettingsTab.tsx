import React, { useEffect, useState } from 'react';
import { Button, Input, Icons } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';
import { useOrgContext } from '../../hooks/useOrgContext';

type ProviderType = 'oidc' | 'saml';

type ReconciliationInterval = 'hourly' | 'daily' | 'weekly';

interface SSOConfigResponse {
    provider_type: ProviderType;
    display_name: string;
    enabled: boolean;
    oidc_issuer?: string;
    oidc_client_id?: string;
    oidc_client_secret_set: boolean;
    oidc_scopes?: string;
    saml_idp_metadata_url?: string;
    saml_idp_entity_id?: string;
    saml_metadata_set: boolean;
    reconciliation_enabled?: boolean;
    reconciliation_interval?: ReconciliationInterval;
    last_reconciled_at?: string;
}

interface FormState {
    display_name: string;
    enabled: boolean;
    oidc_issuer: string;
    oidc_client_id: string;
    oidc_client_secret: string;
    oidc_scopes: string;
    saml_idp_metadata_url: string;
    saml_idp_metadata_xml: string;
    reconciliation_enabled: boolean;
    reconciliation_interval: ReconciliationInterval;
}

const emptyForm: FormState = {
    display_name: '',
    enabled: false,
    oidc_issuer: '',
    oidc_client_id: '',
    oidc_client_secret: '',
    oidc_scopes: '',
    saml_idp_metadata_url: '',
    saml_idp_metadata_xml: '',
    reconciliation_enabled: false,
    reconciliation_interval: 'daily',
};

const ResultBanner: React.FC<{ type: 'success' | 'error'; message: string }> = ({ type, message }) => (
    <div className={`mb-4 p-4 rounded-lg ${type === 'success' ? 'bg-green-900/30 border border-green-600 text-green-200' : 'bg-red-900/30 border border-red-600 text-red-200'}`}>
        {message}
    </div>
);

const ProviderPanel: React.FC<{
    provider: ProviderType;
    orgId: number;
    existing: SSOConfigResponse | undefined;
    onSaved: () => void;
}> = ({ provider, orgId, existing, onSaved }) => {
    const [form, setForm] = useState<FormState>(emptyForm);
    const [isSaving, setIsSaving] = useState(false);
    const [isTesting, setIsTesting] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);
    const [result, setResult] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

    useEffect(() => {
        if (existing) {
            setForm({
                display_name: existing.display_name,
                enabled: existing.enabled,
                oidc_issuer: existing.oidc_issuer || '',
                oidc_client_id: existing.oidc_client_id || '',
                oidc_client_secret: '', // never prefilled - left blank keeps the stored secret
                oidc_scopes: existing.oidc_scopes || '',
                saml_idp_metadata_url: existing.saml_idp_metadata_url || '',
                saml_idp_metadata_xml: '', // never round-tripped - left blank keeps the stored metadata
                reconciliation_enabled: existing.reconciliation_enabled || false,
                reconciliation_interval: existing.reconciliation_interval || 'daily',
            });
        }
    }, [existing]);

    const handleChange = (field: keyof FormState, value: string | boolean) => {
        setForm(prev => ({ ...prev, [field]: value }));
    };

    const requestBody = () => {
        const body: Record<string, unknown> = {
            display_name: form.display_name,
            enabled: form.enabled,
        };
        if (provider === 'oidc') {
            body.oidc_issuer = form.oidc_issuer;
            body.oidc_client_id = form.oidc_client_id;
            body.oidc_scopes = form.oidc_scopes;
            if (form.oidc_client_secret !== '') {
                body.oidc_client_secret = form.oidc_client_secret;
            }
            body.reconciliation_enabled = form.reconciliation_enabled;
            body.reconciliation_interval = form.reconciliation_interval;
        } else {
            body.saml_idp_metadata_url = form.saml_idp_metadata_url;
            if (form.saml_idp_metadata_xml !== '') {
                body.saml_idp_metadata_xml = form.saml_idp_metadata_xml;
            }
        }
        return body;
    };

    const handleSave = async () => {
        if (!form.display_name) {
            notify.error('Please enter a display name (shown on the login page)');
            return;
        }
        setIsSaving(true);
        setResult(null);
        try {
            await apiClient.put(`/api/v1/orgs/${orgId}/sso-config/${provider}`, requestBody());
            setResult({ type: 'success', message: 'SSO configuration saved successfully!' });
            notify.success('SSO configuration saved successfully!');
            onSaved();
        } catch (error: any) {
            const message = error?.message || 'Failed to save SSO configuration';
            setResult({ type: 'error', message });
            notify.error(message);
        } finally {
            setIsSaving(false);
        }
    };

    const handleTest = async () => {
        setIsTesting(true);
        setResult(null);
        try {
            const response = await apiClient.post<{ message: string }>(`/api/v1/orgs/${orgId}/sso-config/${provider}/test`, requestBody());
            const message = response?.message || 'Connection test succeeded';
            setResult({ type: 'success', message });
            notify.success(message);
        } catch (error: any) {
            const message = error?.message || 'Connection test failed';
            setResult({ type: 'error', message });
            notify.error(message);
        } finally {
            setIsTesting(false);
        }
    };

    const handleDelete = async () => {
        if (!window.confirm(`Delete the ${provider.toUpperCase()} SSO configuration? Users will no longer be able to sign in via this provider.`)) {
            return;
        }
        setIsDeleting(true);
        setResult(null);
        try {
            await apiClient.delete(`/api/v1/orgs/${orgId}/sso-config/${provider}`);
            notify.success('SSO configuration deleted');
            setForm(emptyForm);
            onSaved();
        } catch (error: any) {
            const message = error?.message || 'Failed to delete SSO configuration';
            setResult({ type: 'error', message });
            notify.error(message);
        } finally {
            setIsDeleting(false);
        }
    };

    const metadataURL = `${window.location.origin}/api/v1/auth/sso/saml/${orgId}/metadata`;
    const acsURL = `${window.location.origin}/api/v1/auth/sso/saml/${orgId}/acs`;

    return (
        <div className="space-y-4">
            {result && <ResultBanner type={result.type} message={result.message} />}

            <Input
                label="Display Name"
                placeholder={provider === 'oidc' ? 'Company SSO (OIDC)' : 'Company SSO (SAML)'}
                value={form.display_name}
                onChange={(e) => handleChange('display_name', e.target.value)}
            />

            {provider === 'oidc' ? (
                <>
                    <Input
                        label="Issuer URL"
                        placeholder="https://your-idp.example.com/realms/your-realm"
                        value={form.oidc_issuer}
                        onChange={(e) => handleChange('oidc_issuer', e.target.value)}
                    />
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <Input
                            label="Client ID"
                            value={form.oidc_client_id}
                            onChange={(e) => handleChange('oidc_client_id', e.target.value)}
                        />
                        <Input
                            label="Client Secret"
                            type="password"
                            placeholder={existing?.oidc_client_secret_set ? '•••••••• (unchanged)' : ''}
                            value={form.oidc_client_secret}
                            onChange={(e) => handleChange('oidc_client_secret', e.target.value)}
                        />
                    </div>
                    <Input
                        label="Scopes (space-separated, optional)"
                        placeholder="openid email profile"
                        value={form.oidc_scopes}
                        onChange={(e) => handleChange('oidc_scopes', e.target.value)}
                    />
                    <p className="text-sm text-slate-400">
                        Redirect URI to register with your identity provider:{' '}
                        <code className="text-slate-200">{window.location.origin}/api/v1/auth/sso/oidc/callback</code>
                    </p>

                    <div className="bg-slate-800 p-4 rounded-lg space-y-3">
                        <div className="flex items-center space-x-2">
                            <input
                                type="checkbox"
                                id="reconciliation-enabled"
                                checked={form.reconciliation_enabled}
                                onChange={(e) => handleChange('reconciliation_enabled', e.target.checked)}
                                className="w-4 h-4 text-indigo-600 bg-slate-700 border-slate-600 rounded focus:ring-indigo-500 focus:ring-2"
                            />
                            <label htmlFor="reconciliation-enabled" className="text-sm font-medium text-slate-300">
                                Automatic deprovisioning check
                            </label>
                        </div>
                        <p className="text-sm text-slate-400">
                            Fallback for orgs that don't configure SCIM below: periodically re-checks each signed-in
                            user against the identity provider and deactivates them here if the account was disabled
                            or removed there. SCIM (if configured) is real-time; this is a backstop, bounded by the
                            interval you pick.
                        </p>
                        {form.reconciliation_enabled && (
                            <div>
                                <label className="block text-sm font-medium text-slate-300 mb-1">Check interval</label>
                                <select
                                    value={form.reconciliation_interval}
                                    onChange={(e) => handleChange('reconciliation_interval', e.target.value)}
                                    className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-sm text-slate-200"
                                >
                                    <option value="hourly">Hourly</option>
                                    <option value="daily">Daily</option>
                                    <option value="weekly">Weekly</option>
                                </select>
                                {existing?.last_reconciled_at && (
                                    <p className="text-xs text-slate-500 mt-1">
                                        Last checked: {new Date(existing.last_reconciled_at).toLocaleString()}
                                    </p>
                                )}
                            </div>
                        )}
                    </div>
                </>
            ) : (
                <>
                    <Input
                        label="IdP Metadata URL"
                        placeholder="https://your-idp.example.com/metadata"
                        value={form.saml_idp_metadata_url}
                        onChange={(e) => handleChange('saml_idp_metadata_url', e.target.value)}
                    />
                    <div>
                        <label className="block text-sm font-medium text-slate-300 mb-1">
                            Or paste IdP Metadata XML directly (optional - overrides the URL above)
                        </label>
                        <textarea
                            className="w-full h-32 bg-slate-800 border border-slate-700 rounded-lg p-3 text-sm text-slate-200 font-mono"
                            placeholder={existing?.saml_metadata_set ? '(metadata already stored - leave blank to keep it)' : '<EntityDescriptor ...>'}
                            value={form.saml_idp_metadata_xml}
                            onChange={(e) => handleChange('saml_idp_metadata_xml', e.target.value)}
                        />
                    </div>
                    <p className="text-sm text-slate-400">
                        Give your identity provider these URLs: SP Entity ID / Metadata URL{' '}
                        <code className="text-slate-200">{metadataURL}</code>, ACS URL{' '}
                        <code className="text-slate-200">{acsURL}</code>
                    </p>
                </>
            )}

            <div className="flex items-center space-x-2 bg-slate-800 p-4 rounded-lg">
                <input
                    type="checkbox"
                    id={`sso-enabled-${provider}`}
                    checked={form.enabled}
                    onChange={(e) => handleChange('enabled', e.target.checked)}
                    className="w-4 h-4 text-indigo-600 bg-slate-700 border-slate-600 rounded focus:ring-indigo-500 focus:ring-2"
                />
                <label htmlFor={`sso-enabled-${provider}`} className="text-sm font-medium text-slate-300">
                    Enabled (users can sign in with this provider immediately)
                </label>
            </div>

            <div className="flex justify-end space-x-3 pt-4 border-t border-slate-700">
                {existing && (
                    <Button variant="outline" onClick={handleDelete} isLoading={isDeleting} disabled={isSaving || isTesting || isDeleting}>
                        Delete
                    </Button>
                )}
                <Button variant="outline" onClick={handleTest} isLoading={isTesting} disabled={isSaving || isTesting || isDeleting}>
                    Test Connection
                </Button>
                <Button variant="primary" onClick={handleSave} isLoading={isSaving} disabled={isSaving || isTesting || isDeleting || !form.display_name}>
                    Save
                </Button>
            </div>
        </div>
    );
};

interface ScimTokenResponse {
    exists?: boolean;
    token_prefix?: string;
    created_at?: string;
    token?: string; // only present immediately after create/rotate
}

const ScimProvisioningPanel: React.FC<{ orgId: number }> = ({ orgId }) => {
    const [tokenInfo, setTokenInfo] = useState<ScimTokenResponse | null>(null);
    const [newToken, setNewToken] = useState<string | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [isWorking, setIsWorking] = useState(false);

    const fetchToken = async () => {
        try {
            const data = await apiClient.get<ScimTokenResponse>(`/api/v1/orgs/${orgId}/scim-token`);
            setTokenInfo(data);
        } catch (error) {
            console.error('Failed to fetch SCIM token status:', error);
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => {
        if (orgId) {
            fetchToken();
        }
    }, [orgId]);

    const handleGenerate = async () => {
        const verb = tokenInfo?.token_prefix ? 'rotate' : 'generate';
        if (tokenInfo?.token_prefix && !window.confirm('Rotating the SCIM token immediately invalidates the old one. Your identity provider will need the new token. Continue?')) {
            return;
        }
        setIsWorking(true);
        try {
            const data = await apiClient.post<ScimTokenResponse>(`/api/v1/orgs/${orgId}/scim-token`, {});
            setTokenInfo(data);
            setNewToken(data.token || null);
            notify.success(`SCIM token ${verb}d`);
        } catch (error: any) {
            notify.error(error?.message || `Failed to ${verb} SCIM token`);
        } finally {
            setIsWorking(false);
        }
    };

    const handleRevoke = async () => {
        if (!window.confirm('Revoke the SCIM token? Your identity provider will no longer be able to sync users until a new token is generated.')) {
            return;
        }
        setIsWorking(true);
        try {
            await apiClient.delete(`/api/v1/orgs/${orgId}/scim-token`);
            setTokenInfo({ exists: false });
            setNewToken(null);
            notify.success('SCIM token revoked');
        } catch (error: any) {
            notify.error(error?.message || 'Failed to revoke SCIM token');
        } finally {
            setIsWorking(false);
        }
    };

    if (isLoading) {
        return null;
    }

    const scimBaseURL = `${window.location.origin}/api/v1/scim/v2`;

    return (
        <div className="mt-8 pt-8 border-t border-slate-700">
            <div className="flex items-center mb-4">
                <div className="text-indigo-400 mr-3">
                    <Icons.Settings />
                </div>
                <div>
                    <h3 className="font-medium text-white">SCIM Provisioning</h3>
                    <p className="text-sm text-slate-300">
                        Let Okta, Azure AD, or Google Workspace automatically create and deactivate users here when
                        they're added or removed at the identity provider - the real-time counterpart to the
                        reconciliation check above.
                    </p>
                </div>
            </div>

            {newToken && (
                <div className="mb-4 p-4 rounded-lg bg-amber-900/30 border border-amber-600 text-amber-200">
                    <p className="text-sm font-medium mb-2">
                        Copy this token now - it won't be shown again.
                    </p>
                    <code className="block bg-slate-900 p-2 rounded text-xs break-all text-amber-100">{newToken}</code>
                </div>
            )}

            <p className="text-sm text-slate-400 mb-3">
                SCIM base URL: <code className="text-slate-200">{scimBaseURL}</code>
            </p>

            {tokenInfo?.token_prefix ? (
                <p className="text-sm text-slate-400 mb-3">
                    Current token: <code className="text-slate-200">{tokenInfo.token_prefix}...</code>
                    {tokenInfo.created_at && ` (created ${new Date(tokenInfo.created_at).toLocaleString()})`}
                </p>
            ) : (
                <p className="text-sm text-slate-400 mb-3">No SCIM token configured yet.</p>
            )}

            <div className="flex space-x-3">
                <Button variant="primary" onClick={handleGenerate} isLoading={isWorking} disabled={isWorking}>
                    {tokenInfo?.token_prefix ? 'Rotate Token' : 'Generate Token'}
                </Button>
                {tokenInfo?.token_prefix && (
                    <Button variant="outline" onClick={handleRevoke} isLoading={isWorking} disabled={isWorking}>
                        Revoke
                    </Button>
                )}
            </div>
        </div>
    );
};

const SSOSettingsTab: React.FC = () => {
    const { currentOrgId } = useOrgContext();
    const orgId = currentOrgId ?? 0;
    const [configs, setConfigs] = useState<SSOConfigResponse[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [activeProvider, setActiveProvider] = useState<ProviderType>('oidc');

    const fetchConfigs = async () => {
        try {
            const data = await apiClient.get<SSOConfigResponse[]>(`/api/v1/orgs/${orgId}/sso-config`);
            setConfigs(data || []);
        } catch (error) {
            console.error('Failed to fetch SSO configuration:', error);
            notify.error('Failed to load SSO configuration');
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => {
        if (orgId) {
            fetchConfigs();
        }
    }, [orgId]);

    if (isLoading) {
        return (
            <div className="flex justify-center p-8">
                <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    const existingForActive = configs.find(c => c.provider_type === activeProvider);

    return (
        <div>
            <div className="flex items-center mb-6">
                <div className="text-indigo-400 mr-3">
                    <Icons.Settings />
                </div>
                <div>
                    <h3 className="font-medium text-white">Single Sign-On (SSO)</h3>
                    <p className="text-sm text-slate-300">
                        Let your team sign in with your company's identity provider, in addition to email/password.
                    </p>
                </div>
            </div>

            <div className="flex space-x-2 mb-6 border-b border-slate-700">
                {(['oidc', 'saml'] as ProviderType[]).map(p => (
                    <button
                        key={p}
                        onClick={() => setActiveProvider(p)}
                        className={`px-4 py-2 text-sm font-medium border-b-2 ${
                            activeProvider === p
                                ? 'border-indigo-500 text-indigo-400'
                                : 'border-transparent text-slate-400 hover:text-slate-200'
                        }`}
                    >
                        {p.toUpperCase()}
                        {configs.find(c => c.provider_type === p)?.enabled && (
                            <span className="ml-2 inline-block w-2 h-2 rounded-full bg-green-500" />
                        )}
                    </button>
                ))}
            </div>

            <ProviderPanel
                key={activeProvider}
                provider={activeProvider}
                orgId={orgId}
                existing={existingForActive}
                onSaved={fetchConfigs}
            />

            <ScimProvisioningPanel orgId={orgId} />
        </div>
    );
};

export default SSOSettingsTab;
