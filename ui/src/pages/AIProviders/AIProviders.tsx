import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation, useParams } from 'react-router-dom';
import {
    PageHeader,
    Card,
    Button,
    Icons,
    Input,
    Alert,
    Section,
    EmptyState,
    Spinner,
    Badge,
    Avatar
} from '../../components/UIPrimitives';
import { getReviewAISettings, updateReviewAISettings } from '../../api/connectors';

// Types
import { AIProvider, AIConnector, ReviewAISettings } from './types';

// Hooks
import { useProviderSelection, useConnectors, useFormState } from './hooks';

// Components
import {
    ProvidersList,
    ProviderDetail,
    ConnectorForm,
    ConnectorsList,
    UsageTips,
    AdaptiveReviewInfo
} from './components';
import OllamaConnectorForm from './components/OllamaConnectorForm';
import BedrockConnectorForm from './components/BedrockConnectorForm';

// Utils
import { generateFriendlyNameForProvider, getProviderDetails } from './utils/nameUtils';

// Constant data
const popularAIProviders: AIProvider[] = [
    {
        id: 'gemini',
        name: 'Google Gemini',
        url: 'https://ai.google.dev/',
        description: 'High quality, multimodal reasoning with balanced cost and performance.',
        icon: <Icons.Google />,
        apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
    },
    {
        id: 'gemini-enterprise',
        name: 'Gemini Enterprise',
        url: 'https://cloud.google.com/vertex-ai',
        description: 'Enterprise-grade LLM services via GCP Vertex AI with IAM authentication.',
        icon: <Icons.Google />,
        apiKeyPlaceholder: 'Paste Service Account JSON content here'
    },
    {
        id: 'deepseek',
        name: 'DeepSeek',
        url: 'https://platform.deepseek.com/',
        description: 'Native DeepSeek connector with chat as default and R1 available for deeper reasoning.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        baseURLPlaceholder: 'https://api.deepseek.com/v1'
    },
    {
        id: 'openrouter',
        name: 'OpenRouter',
        url: 'https://openrouter.ai/',
        description: 'Bring your own key and choose any OpenRouter model. Defaults to the free DeepSeek route.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        baseURLPlaceholder: 'https://openrouter.ai/api/v1'
    },
    {
        id: 'ollama',
        name: 'Ollama',
        url: 'https://ollama.ai/',
        description: 'Run models locally. Great for privacy & air‑gapped workflows.',
        icon: <Icons.Ollama />,
        apiKeyPlaceholder: 'Optional JWT token for authentication',
        baseURLPlaceholder: 'http://localhost:11434/ollama/api',
        requiresBaseURL: true
    },
    {
        id: 'openai',
        name: 'OpenAI',
        url: 'https://platform.openai.com/',
        description: 'Fast, strong reasoning via OpenAI models with broad model compatibility.',
        icon: <Icons.OpenAI />,
        apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
    },
    {
        id: 'atlas',
        name: 'Atlas Cloud',
        url: 'https://atlascloud.ai/',
        description: 'OpenAI-compatible AI cloud engine. Choose from a selection of models including DeepSeek.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'ac_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        baseURLPlaceholder: 'https://api.atlascloud.ai/v1'
    },
    {
        id: 'claude',
        name: 'Anthropic Claude',
        url: 'https://www.anthropic.com/',
        description: 'Advanced reasoning & long context. Vote to prioritize deeper integration.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
    },
    {
        id: 'bedrock',
        name: 'AWS Bedrock',
        url: 'https://aws.amazon.com/bedrock/',
        description: 'Claude, Nova, Llama, and other foundation models via your own AWS account.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'AWS Secret Access Key'
    },
];

const AIProviders: React.FC = () => {
    // Custom hooks
    const {
        selectedProvider,
        setSelectedProvider,
        updateUrlFragment,
        isEditing,
        setIsEditing
    } = useProviderSelection(popularAIProviders);

    const {
        connectors,
        isLoading,
        error,
        fetchConnectors,
        saveConnector,
        deleteConnector,
        reorderConnectors,
        setError
    } = useConnectors();

    const {
        formData,
        selectedConnector,
        setSelectedConnector,
        handleInputChange,
        handleProviderTypeChange,
        resetForm,
        setFormData,
        generateFriendlyName
    } = useFormState();

	// Local state
	const [isSaved, setIsSaved] = useState(false);
	const [showDropdown, setShowDropdown] = useState(false);
    const [activeRole, setActiveRole] = useState<'leader' | 'helper'>('leader');
    const [helperSettings, setHelperSettings] = useState<ReviewAISettings>({
        helper_enabled: true,
        helper_mode: 'concise_then_expand'
    });
    const [helperSettingsSaved, setHelperSettingsSaved] = useState(false);
	const dropdownRef = useRef<HTMLDivElement>(null);

	const getDefaultModelFor = (providerId?: string) => {
		if (!providerId) return '';
		const meta = popularAIProviders.find((p) => p.id === providerId);
		return meta?.defaultModel || '';
	};

    // Calculate provider connector counts
    const connectorCounts = connectors
		.filter((connector) => (connector.role || 'leader') === activeRole)
		.reduce((counts: Record<string, number>, connector) => {
        counts[connector.providerName] = (counts[connector.providerName] || 0) + 1;
        return counts;
    }, {});

	const roleScopedConnectors = connectors.filter((connector) => (connector.role || 'leader') === activeRole);

    // Close dropdown when clicking outside
    useEffect(() => {
        function handleClickOutside(event: MouseEvent) {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setShowDropdown(false);
            }
        }

        document.addEventListener("mousedown", handleClickOutside);
        return () => {
            document.removeEventListener("mousedown", handleClickOutside);
        };
    }, [dropdownRef]);

    useEffect(() => {
        const loadReviewAISettings = async () => {
            try {
                const settings = await getReviewAISettings();
                setHelperSettings({
                    helper_enabled: !!settings.helper_enabled,
                    helper_mode: (settings.helper_mode as 'concise_then_expand' | 'polish_only') || 'concise_then_expand'
                });
            } catch (settingsError) {
                console.error('Failed to load review AI settings:', settingsError);
            }
        };

        loadReviewAISettings();
    }, []);

    useEffect(() => {
        setFormData({
            ...formData,
            role: activeRole,
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeRole]);

    // Handle URL path changes
    useEffect(() => {
        const params = new URLSearchParams(location.search);
        const action = params.get('action');
        const connectorId = params.get('connectorId');

        if (action === 'edit' && connectorId) {
            // Find the connector to edit based on connectorId
            if (connectors.length > 0) {
                const connector = connectors.find(c => c.id === connectorId);
                if (connector) {
                    handleEditConnector(connector);
                }
            }
        } else if (action === 'add') {
            handleAddConnector();
        }
    }, [location.search, connectors.length]);

    // Handle provider selection
    const handleSelectProvider = (providerId: string) => {
        setSelectedProvider(providerId);
        resetForm();
        setShowDropdown(false);
        updateUrlFragment(providerId);
    };

    // Handle adding a new connector
    const handleAddConnector = () => {
        setFormData({
            name: generateFriendlyNameForProvider(selectedProvider, popularAIProviders),
            apiKey: '',
            providerType: selectedProvider === 'all' ? '' : selectedProvider,
            role: activeRole,
            selectedModel: getDefaultModelFor(selectedProvider === 'all' ? undefined : selectedProvider),
            baseURL: '',
            gcpProjectID: '',
            gcpLocation: ''
        });
        setIsEditing(false);
        setSelectedConnector(null);
        updateUrlFragment(selectedProvider, 'add');
    };

    // Handle selecting a provider from dropdown
    const handleSelectProviderToAdd = (providerId: string) => {
        setFormData({
            name: generateFriendlyNameForProvider(providerId, popularAIProviders),
            apiKey: '',
            providerType: providerId,
            role: activeRole,
            selectedModel: getDefaultModelFor(providerId),
            baseURL: '',
            gcpProjectID: '',
            gcpLocation: ''
        });
        setIsEditing(false);
        setSelectedConnector(null);
        setShowDropdown(false);
        updateUrlFragment(providerId, 'add');
    };

    // Handle editing a connector
    const handleEditConnector = (connector: AIConnector) => {
        if (connector.providerName === 'livereview-default-ai') {
            return; // Managed connectors are read-only
        }
        setSelectedConnector(connector);
        setSelectedProvider(connector.providerName);
		setActiveRole((connector.role || 'leader') as 'leader' | 'helper');
        setFormData({
            name: connector.name,
            apiKey: connector.fullApiKey || connector.apiKey,
            providerType: connector.providerName,
            role: (connector.role || 'leader') as 'leader' | 'helper',
            baseURL: connector.baseURL || connector.base_url || '',
            selectedModel: connector.selectedModel || connector.selected_model || getDefaultModelFor(connector.providerName),
            gcpProjectID: connector.gcpProjectID || connector.gcp_project_id || '',
            gcpLocation: connector.gcpLocation || connector.gcp_location || ''
        });
        setIsEditing(true);
        updateUrlFragment(connector.providerName, 'edit', connector.id);
    };

    // Handle save/update connector
    const handleSaveConnector = async () => {
        // Determine the provider to use
        const providerToUse = selectedProvider === 'all' ? formData.providerType : selectedProvider;

        if (!providerToUse) {
            setError('Please select a provider');
            return;
        }

        try {
            const success = await saveConnector(
                providerToUse,
                formData.role || activeRole,
                formData.apiKey,
                formData.name,
                selectedConnector,
                formData.baseURL,
                formData.selectedModel || getDefaultModelFor(providerToUse),
                formData.gcpProjectID,
                formData.gcpLocation
            );

            if (success) {
                // Show success message
                setIsSaved(true);
                setTimeout(() => setIsSaved(false), 3000);

                // Reset form
                resetForm();

                // Update URL to show the provider without any specific action
                updateUrlFragment(providerToUse);
            }
        } catch (error) {
            console.error('Error in handleSaveConnector:', error);
        }
    };

    // Handle Ollama-specific save
    const handleSaveOllamaConnector = async (baseURL: string, jwtToken: string, selectedModel: string, name: string) => {
        try {
            const success = await saveConnector(
                'ollama',
                formData.role || activeRole,
                jwtToken, // Use JWT token as the "API key" for Ollama
                name,
                selectedConnector,
                baseURL,
                selectedModel
            );

            if (success) {
                // Show success message
                setIsSaved(true);
                setTimeout(() => setIsSaved(false), 3000);

                // Reset form
                resetForm();

                // Update URL to show the provider without any specific action
                updateUrlFragment('ollama');
            }
        } catch (error) {
            console.error('Error in handleSaveOllamaConnector:', error);
        }
    };

    // Handle Bedrock-specific save
    const handleSaveBedrockConnector = async (accessKeyId: string, secretAccessKey: string, region: string, selectedModel: string, name: string) => {
        try {
            const success = await saveConnector(
                'bedrock',
                formData.role || activeRole,
                secretAccessKey, // Use the AWS Secret Access Key as the "API key" for Bedrock
                name,
                selectedConnector,
                undefined,
                selectedModel,
                undefined,
                undefined,
                accessKeyId,
                region
            );

            if (success) {
                // Show success message
                setIsSaved(true);
                setTimeout(() => setIsSaved(false), 3000);

                // Reset form
                resetForm();

                // Update URL to show the provider without any specific action
                updateUrlFragment('bedrock');
            }
        } catch (error) {
            console.error('Error in handleSaveBedrockConnector:', error);
        }
    };

    // Handle generate name button
    const handleGenerateName = () => {
        const providerToUse = selectedProvider === 'all' ? formData.providerType : selectedProvider;
        if (!providerToUse) {
            setError('Please select a provider first');
            return;
        }

        generateFriendlyName(providerToUse, popularAIProviders);
    };

    // Handle provider type change in "all" view
    const handleProviderChange = (providerType: string) => {
        handleProviderTypeChange(providerType, popularAIProviders);
    };

    const handleSaveHelperSettings = async () => {
        try {
            const updated = await updateReviewAISettings(helperSettings.helper_enabled, helperSettings.helper_mode);
            setHelperSettings({
                helper_enabled: !!updated.helper_enabled,
                helper_mode: (updated.helper_mode as 'concise_then_expand' | 'polish_only') || 'concise_then_expand'
            });
            setHelperSettingsSaved(true);
            setTimeout(() => setHelperSettingsSaved(false), 3000);
        } catch (settingsError) {
            console.error('Error saving helper settings:', settingsError);
            setError('Failed to save Helper model settings. Please try again.');
        }
    };

    // Handle deleting a connector
    const handleDeleteConnector = async () => {
        if (!selectedConnector) {
            return;
        }

        if (window.confirm(`Are you sure you want to delete the connector "${selectedConnector.name}"?`)) {
            try {
                const success = await deleteConnector(selectedConnector.id);

                if (success) {
                    // Reset form and update URL
                    resetForm();
                    updateUrlFragment(selectedProvider);
                }
            } catch (error) {
                console.error('Error in handleDeleteConnector:', error);
            }
        }
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <PageHeader
                title="AI Providers"
                description={
                    "Configure Leader and Helper AI models for code review."
                }
                actions={<a href="https://github.com/HexmosTech/LiveReview/discussions/9" target="_blank" rel="noopener noreferrer"><Button variant="outline" size="sm">Vote / Request Provider</Button></a>}
            />

            <div className="mb-6 flex flex-wrap gap-3">
                <Button
                    variant={activeRole === 'leader' ? 'primary' : 'outline'}
                    size="sm"
                    onClick={() => setActiveRole('leader')}
                >
                    Leader Model
                </Button>
                <Button
                    variant={activeRole === 'helper' ? 'primary' : 'outline'}
                    size="sm"
                    onClick={() => setActiveRole('helper')}
                >
                    Helper Model
                </Button>
            </div>
            <AdaptiveReviewInfo activeRole={activeRole} variant="tab" />

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                {/* Left panel for selecting providers */}
                <div className="lg:col-span-1">
                    <Card title="AI Providers">
                        <ProvidersList
                            providers={popularAIProviders}
                            selectedProvider={selectedProvider}
                            connectorCounts={connectorCounts}
                            onSelectProvider={handleSelectProvider}
                            totalConnectors={roleScopedConnectors.length}
                        />

                        {/* Provider Info - Show only for specific providers, not for "all" view */}
                        {selectedProvider !== 'all' && (
                            <ProviderDetail
                                provider={popularAIProviders.find(p => p.id === selectedProvider)!}
                            />
                        )}
                    </Card>

                    {/* Usage Tips */}
                    <UsageTips />
                </div>

                {/* Main content area - 2 columns */}
                <div className="lg:col-span-2">
                    <div className="grid grid-cols-1 gap-6">
                        {activeRole === 'helper' && (
                            <>
                            <AdaptiveReviewInfo activeRole={activeRole} variant="settings" />
                            <Card title="Helper Model Settings" badge={helperSettings.helper_enabled ? 'Enabled' : 'Disabled'}>
                                <div className="space-y-4">
                                    <label className="flex items-center gap-3 text-sm text-slate-200">
                                        <input
                                            type="checkbox"
                                            checked={helperSettings.helper_enabled}
                                            onChange={(e) => setHelperSettings((prev) => ({ ...prev, helper_enabled: e.target.checked }))}
                                            className="h-4 w-4 rounded border-slate-500 bg-slate-800 text-blue-500"
                                        />
                                        <span>Enable Helper model for text expansion and polishing</span>
                                    </label>
                                    <div>
                                        <label className="block text-sm font-medium text-slate-300 mb-2">Helper Mode</label>
                                        <select
                                            value={helperSettings.helper_mode}
                                            onChange={(e) => setHelperSettings((prev) => ({
                                                ...prev,
                                                helper_mode: e.target.value as 'concise_then_expand' | 'polish_only'
                                            }))}
                                            className="w-full rounded-lg border border-slate-600 bg-slate-800 px-3 py-2 text-sm text-white"
                                        >
                                            <option value="concise_then_expand">Concise Then Expand</option>
                                            <option value="polish_only">Polish Only</option>
                                        </select>
                                    </div>
                                    <div className="flex items-center gap-3">
                                        <Button variant="primary" size="sm" onClick={handleSaveHelperSettings}>
                                            Save Helper Settings
                                        </Button>
                                        {helperSettingsSaved && <span className="text-sm text-emerald-400">Saved</span>}
                                    </div>
                                </div>
                            </Card>
                            </>
                        )}

                        {/* Connector Form - Only show when actively adding/editing */}
                        {(formData.name || formData.apiKey || isEditing) && (
                            <>
                                {/* Special form for Ollama */}
                                {((selectedProvider === 'ollama') || (selectedProvider === 'all' && formData.providerType === 'ollama')) ? (
                                    <OllamaConnectorForm
                                        provider={popularAIProviders.find(p => p.id === 'ollama')!}
                                        onSave={handleSaveOllamaConnector}
                                        onCancel={resetForm}
                                        isLoading={isLoading}
                                        error={error}
                                        setError={setError}
                                        editingConnector={isEditing && selectedConnector ? (() => {
                                            console.log('Debug: selectedConnector data:', selectedConnector);
                                            return {
                                                name: selectedConnector.name,
                                                baseURL: selectedConnector.baseURL || '',
                                                jwtToken: selectedConnector.fullApiKey || '',
                                                selectedModel: selectedConnector.selectedModel || ''
                                            };
                                        })() : null}
                                    />
                                ) : ((selectedProvider === 'bedrock') || (selectedProvider === 'all' && formData.providerType === 'bedrock')) ? (
                                    /* Special form for Bedrock */
                                    <BedrockConnectorForm
                                        provider={popularAIProviders.find(p => p.id === 'bedrock')!}
                                        onSave={handleSaveBedrockConnector}
                                        onCancel={resetForm}
                                        isLoading={isLoading}
                                        error={error}
                                        setError={setError}
                                        editingConnector={isEditing && selectedConnector ? {
                                            name: selectedConnector.name,
                                            awsAccessKeyID: selectedConnector.awsAccessKeyID || '',
                                            secretAccessKey: selectedConnector.fullApiKey || '',
                                            awsRegion: selectedConnector.awsRegion || '',
                                            selectedModel: selectedConnector.selectedModel || ''
                                        } : null}
                                    />
                                ) : (
                                    /* Regular form for other providers */
                                    <ConnectorForm
                                        providers={popularAIProviders}
                                        selectedProvider={selectedProvider}
                                        formData={formData}
                                        isEditing={isEditing}
                                        isLoading={isLoading}
                                        isSaved={isSaved}
                                        error={error}
                                        onInputChange={handleInputChange}
                                        onProviderTypeChange={handleProviderChange}
                                        onGenerateName={handleGenerateName}
                                        onSave={handleSaveConnector}
                                        onCancel={resetForm}
                                        onDelete={isEditing ? handleDeleteConnector : undefined}
                                        setError={setError}
                                    />
                                )}
                            </>
                        )}

                        {/* Connectors List */}
                        <ConnectorsList
                            connectors={roleScopedConnectors}
                            providers={popularAIProviders}
                            selectedProvider={selectedProvider}
                            isLoading={isLoading}
                            error={error}
                            onEditConnector={handleEditConnector}
                            onAddConnector={handleAddConnector}
                            onRetry={fetchConnectors}
                            showAddDropdown={showDropdown}
                            onToggleDropdown={() => setShowDropdown(!showDropdown)}
                            onSelectProviderToAdd={handleSelectProviderToAdd}
                            dropdownRef={dropdownRef}
                            reorderConnectors={reorderConnectors}
                        />
                    </div>
                </div>
            </div>
        </div>
    );
};

export default AIProviders;
