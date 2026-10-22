import React, { useCallback, useEffect, useState } from 'react';
import { Button, Icons, Input, Select, Spinner } from '../../components/UIPrimitives';
import { Table, TableHead, TableHeaderCell, TableBody, TableRow, TableCell } from '../../components/DataTable/SimpleTable';
import { ConfirmModal } from '../../components/ConfirmModal';
import apiClient from '../../api/apiClient';
import { getAIProviderModels } from '../../api/connectors';
import { notify } from '../../utils/notify';

// Super admin only (Cloud): which provider/model/key runs the "LiveReview AI Model" that paid orgs and
// launch-trial sign-ups review on. Each tier can hold several configurations; exactly one is active. Keys are
// write-only: the API never returns them.

interface ManagedAIConfig {
    id: number;
    tier_name: string;
    provider_name: string;
    model_name: string;
    gcp_location: string;
    label: string;
    is_active: boolean;
    has_key: boolean;
    updated_at: string;
}

interface ConfigForm {
    id?: number;
    tier_name: string;
    provider_name: string;
    model_name: string;
    api_key: string;
    gcp_location: string;
    label: string;
}

const TIERS: { id: string; name: string; description: string }[] = [
    { id: 'default', name: 'Main model', description: 'Writes the review comments.' },
    { id: 'default_lite', name: 'Helper model', description: 'Rewrites comments when the helper stage is on. Cheaper and faster.' },
];

const PROVIDERS: { value: string; label: string; modelHint: string }[] = [
    { value: 'openrouter', label: 'OpenRouter', modelHint: 'deepseek/deepseek-v4-flash' },
    { value: 'deepseek', label: 'DeepSeek', modelHint: 'deepseek-v4-flash' },
    { value: 'gemini-enterprise', label: 'Gemini Enterprise (Vertex AI)', modelHint: 'gemini-3.8-flash' },
    { value: 'gemini', label: 'Gemini (AI Studio key)', modelHint: 'gemini-3.8-flash' },
    { value: 'openai', label: 'OpenAI', modelHint: 'gpt-5-mini' },
    { value: 'claude', label: 'Claude', modelHint: 'claude-haiku-5-5' },
];

const providerLabel = (value: string) => PROVIDERS.find((p) => p.value === value)?.label || value;

const emptyForm = (tier: string): ConfigForm => ({
    tier_name: tier,
    provider_name: 'openrouter',
    model_name: '',
    api_key: '',
    gcp_location: '',
    label: '',
});

const ManagedAITab: React.FC = () => {
    const [configs, setConfigs] = useState<ManagedAIConfig[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [form, setForm] = useState<ConfigForm | null>(null);
    const [isSaving, setIsSaving] = useState(false);
    const [isTestingForm, setIsTestingForm] = useState(false);
    const [testingId, setTestingId] = useState<number | null>(null);
    const [activateTarget, setActivateTarget] = useState<ManagedAIConfig | null>(null);
    const [isActivating, setIsActivating] = useState(false);
    // Model IDs from the same catalog the AI Providers page uses, offered as suggestions (any ID can still be typed).
    const [models, setModels] = useState<{ id: string; isDefault: boolean }[]>([]);
    const formProvider = form?.provider_name;

    useEffect(() => {
        if (!formProvider) {
            setModels([]);
            return;
        }
        let active = true;
        getAIProviderModels(formProvider)
            .then((data) => {
                if (active) setModels((data?.models || []).map((m) => ({ id: m.model_id, isDefault: m.is_default })));
            })
            .catch(() => {
                if (active) setModels([]);
            });
        return () => {
            active = false;
        };
    }, [formProvider]);

    const load = useCallback(async () => {
        try {
            const data = await apiClient.get<{ configs: ManagedAIConfig[] }>('/api/v1/admin/managed-ai');
            setConfigs(data?.configs || []);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to load managed AI configurations');
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => {
        load();
    }, [load]);

    const isVertex = form?.provider_name === 'gemini-enterprise';
    const isEditing = form?.id !== undefined;

    const formPayload = (f: ConfigForm) => ({
        id: f.id,
        tier_name: f.tier_name,
        provider_name: f.provider_name,
        model_name: f.model_name.trim(),
        api_key: f.api_key.trim(),
        gcp_location: f.provider_name === 'gemini-enterprise' ? f.gcp_location.trim() : '',
        label: f.label.trim(),
    });

    const handleTestForm = async () => {
        if (!form) return;
        setIsTestingForm(true);
        try {
            const res = await apiClient.post<{ ok: boolean; message: string }>('/api/v1/admin/managed-ai/test', formPayload(form));
            if (res?.ok) notify.success(res.message || 'The model answered');
            else notify.error(res?.message || 'The test call failed');
        } catch (error: any) {
            notify.error(error?.message || 'Failed to test the configuration');
        } finally {
            setIsTestingForm(false);
        }
    };

    const handleSave = async () => {
        if (!form) return;
        setIsSaving(true);
        try {
            const payload = formPayload(form);
            if (isEditing) {
                await apiClient.put(`/api/v1/admin/managed-ai/${form.id}`, payload);
                notify.success('Configuration saved successfully');
            } else {
                await apiClient.post('/api/v1/admin/managed-ai', payload);
                notify.success('Configuration added successfully');
            }
            setForm(null);
            await load();
        } catch (error: any) {
            notify.error(error?.message || 'Failed to save the configuration');
        } finally {
            setIsSaving(false);
        }
    };

    const handleTestSaved = async (cfg: ManagedAIConfig) => {
        setTestingId(cfg.id);
        try {
            const res = await apiClient.post<{ ok: boolean; message: string }>('/api/v1/admin/managed-ai/test', {
                id: cfg.id,
                tier_name: cfg.tier_name,
                provider_name: cfg.provider_name,
                model_name: cfg.model_name,
                gcp_location: cfg.gcp_location,
            });
            if (res?.ok) notify.success(res.message || 'The model answered');
            else notify.error(res?.message || 'The test call failed');
        } catch (error: any) {
            notify.error(error?.message || 'Failed to test the configuration');
        } finally {
            setTestingId(null);
        }
    };

    const handleActivate = async () => {
        if (!activateTarget) return;
        setIsActivating(true);
        try {
            await apiClient.post(`/api/v1/admin/managed-ai/${activateTarget.id}/activate`, {});
            notify.success('Configuration activated successfully');
            setActivateTarget(null);
            await load();
        } catch (error: any) {
            notify.error(error?.message || 'Failed to activate the configuration');
        } finally {
            setIsActivating(false);
        }
    };

    if (isLoading) {
        return (
            <div className="flex justify-center p-8">
                <Spinner />
            </div>
        );
    }

    const providerHint =
        models.find((m) => m.isDefault)?.id || PROVIDERS.find((p) => p.value === form?.provider_name)?.modelHint || '';

    return (
        <div className="space-y-6">
            <div className="flex items-center">
                <div className="text-blue-400 mr-3">
                    <Icons.AI />
                </div>
                <div>
                    <h2 className="text-xl font-semibold text-white">Managed AI</h2>
                    <p className="text-sm text-slate-400">
                        The provider and model behind the "LiveReview AI Model": paid plans and launch-trial sign-ups review on it.
                        Only the active configuration of each tier is used; switching takes effect on the next review.
                    </p>
                </div>
            </div>

            {TIERS.map((tier) => {
                const rows = configs.filter((c) => c.tier_name === tier.id);
                return (
                    <div key={tier.id} className="rounded-lg border border-slate-700 bg-slate-900/40">
                        <div className="flex items-center justify-between gap-4 px-4 py-3 border-b border-slate-700">
                            <div>
                                <h3 className="text-base font-medium text-white">{tier.name}</h3>
                                <p className="text-xs text-slate-400">{tier.description}</p>
                            </div>
                            <Button size="sm" variant="outline" icon={<Icons.Add />} onClick={() => setForm(emptyForm(tier.id))}>
                                Add configuration
                            </Button>
                        </div>
                        {rows.length === 0 ? (
                            <p className="px-4 py-4 text-sm text-slate-400">No configuration yet. Reviews on the LiveReview model will fail for this tier.</p>
                        ) : (
                            <div className="overflow-x-auto">
                                <Table>
                                    <TableHead>
                                        <TableHeaderCell>Configuration</TableHeaderCell>
                                        <TableHeaderCell>Model</TableHeaderCell>
                                        <TableHeaderCell>Key</TableHeaderCell>
                                        <TableHeaderCell>Status</TableHeaderCell>
                                        <TableHeaderCell className="text-right">Actions</TableHeaderCell>
                                    </TableHead>
                                    <TableBody>
                                        {rows.map((cfg) => (
                                            <TableRow key={cfg.id}>
                                                <TableCell>
                                                    <div className="text-sm text-white">{cfg.label || providerLabel(cfg.provider_name)}</div>
                                                    <div className="text-xs text-slate-400">
                                                        {providerLabel(cfg.provider_name)}
                                                        {cfg.gcp_location ? ` · ${cfg.gcp_location}` : ''}
                                                    </div>
                                                </TableCell>
                                                <TableCell className="text-sm text-slate-300 font-mono">{cfg.model_name}</TableCell>
                                                <TableCell className="text-sm text-slate-400">{cfg.has_key ? 'Set' : 'Missing'}</TableCell>
                                                <TableCell>
                                                    {cfg.is_active ? (
                                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900/20 text-green-300">
                                                            Active
                                                        </span>
                                                    ) : (
                                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700/40 text-slate-400">
                                                            Inactive
                                                        </span>
                                                    )}
                                                </TableCell>
                                                <TableCell>
                                                    <div className="flex justify-end gap-2">
                                                        <Button size="sm" variant="ghost" isLoading={testingId === cfg.id} onClick={() => handleTestSaved(cfg)}>
                                                            Test
                                                        </Button>
                                                        <Button
                                                            size="sm"
                                                            variant="ghost"
                                                            onClick={() =>
                                                                setForm({
                                                                    id: cfg.id,
                                                                    tier_name: cfg.tier_name,
                                                                    provider_name: cfg.provider_name,
                                                                    model_name: cfg.model_name,
                                                                    api_key: '',
                                                                    gcp_location: cfg.gcp_location,
                                                                    label: cfg.label,
                                                                })
                                                            }
                                                        >
                                                            Edit
                                                        </Button>
                                                        {!cfg.is_active && (
                                                            <Button size="sm" onClick={() => setActivateTarget(cfg)}>
                                                                Activate
                                                            </Button>
                                                        )}
                                                    </div>
                                                </TableCell>
                                            </TableRow>
                                        ))}
                                    </TableBody>
                                </Table>
                            </div>
                        )}
                    </div>
                );
            })}

            {form && (
                <div className="rounded-lg border border-slate-700 bg-slate-900/40 p-4 space-y-4">
                    <h3 className="text-base font-medium text-white">
                        {isEditing ? 'Edit configuration' : 'Add configuration'} · {TIERS.find((t) => t.id === form.tier_name)?.name}
                    </h3>
                    <Select
                        id="managed-ai-provider"
                        label="Provider"
                        value={form.provider_name}
                        options={PROVIDERS.map((p) => ({ value: p.value, label: p.label }))}
                        onChange={(e: React.ChangeEvent<HTMLSelectElement>) => setForm({ ...form, provider_name: e.target.value, model_name: '' })}
                    />
                    <Input
                        id="managed-ai-model"
                        label="Model"
                        autoComplete="off"
                        list="managed-ai-models"
                        value={form.model_name}
                        placeholder={providerHint}
                        helperText={models.length > 0 ? `Start typing to pick from ${models.length} known models, or enter any model ID.` : undefined}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, model_name: e.target.value })}
                    />
                    <datalist id="managed-ai-models">
                        {models.map((m) => (
                            <option key={m.id} value={m.id} />
                        ))}
                    </datalist>
                    {isVertex ? (
                        <>
                            <div>
                                <label htmlFor="managed-ai-key" className="block text-sm font-medium text-slate-300 mb-1">
                                    Service account key (JSON)
                                </label>
                                <textarea
                                    id="managed-ai-key"
                                    rows={5}
                                    value={form.api_key}
                                    placeholder={isEditing ? 'Leave blank to keep the stored key' : '{"type": "service_account", ...}'}
                                    onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                                    className="w-full rounded-md bg-slate-700 border border-slate-600 px-3 py-2 text-sm text-white font-mono placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                                />
                            </div>
                            <Input
                                id="managed-ai-location"
                                label="Location"
                                autoComplete="off"
                                value={form.gcp_location}
                                placeholder="us"
                                helperText="Gemini 3.x is served from us, eu or global, not single regions such as us-central1."
                                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, gcp_location: e.target.value })}
                            />
                        </>
                    ) : (
                        <Input
                            id="managed-ai-key"
                            label="API key"
                            type="password"
                            autoComplete="new-password"
                            value={form.api_key}
                            placeholder={isEditing ? 'Leave blank to keep the stored key' : ''}
                            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, api_key: e.target.value })}
                        />
                    )}
                    <Input
                        id="managed-ai-label"
                        label="Label (optional)"
                        autoComplete="off"
                        value={form.label}
                        placeholder="OpenRouter DeepSeek V4 Flash"
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, label: e.target.value })}
                    />
                    <p className="text-xs text-slate-400">
                        {isEditing ? 'Save' : 'Add'} tests the configuration first and only saves it if the model answers. New configurations
                        start inactive; Activate switches the tier to one (it is tested again before it goes live).
                    </p>
                    <div className="flex justify-end space-x-4 pt-4">
                        <Button variant="ghost" onClick={() => setForm(null)}>
                            Cancel
                        </Button>
                        <Button variant="secondary" isLoading={isTestingForm} onClick={handleTestForm}>
                            Test
                        </Button>
                        <Button isLoading={isSaving} onClick={handleSave}>
                            {isEditing ? 'Test and save' : 'Test and add'}
                        </Button>
                    </div>
                </div>
            )}

            <ConfirmModal
                show={activateTarget !== null}
                title="Activate this configuration?"
                message={
                    activateTarget
                        ? `Reviews on the LiveReview model (${TIERS.find((t) => t.id === activateTarget.tier_name)?.name}) switch to ${providerLabel(activateTarget.provider_name)} ${activateTarget.model_name} immediately. It is tested first and only activated if the test passes.`
                        : ''
                }
                confirmText="Activate"
                type="warning"
                loading={isActivating}
                onConfirm={handleActivate}
                onCancel={() => setActivateTarget(null)}
            />
        </div>
    );
};

export default ManagedAITab;
