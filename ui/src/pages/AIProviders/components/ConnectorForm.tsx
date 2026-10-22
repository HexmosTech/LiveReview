import React from 'react';
import AIModelSelect from '../../../components/AIModelSelect';
import { AIProvider, ConnectorFormData, AIConnector } from '../types';
import {
    Card,
    Button,
    Icons,
    Input,
    Alert
} from '../../../components/UIPrimitives';

interface ConnectorFormProps {
    providers: AIProvider[];
    selectedProvider: string;
    formData: ConnectorFormData;
    isEditing: boolean;
    isLoading: boolean;
    isSaved: boolean;
    error: string | null;
    onInputChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
    onProviderTypeChange: (value: string) => void;
    onGenerateName: () => void;
    onSave: () => void;
    onCancel: () => void;
    onDelete?: () => void;
    setError: (error: string | null) => void;
}

const ConnectorForm: React.FC<ConnectorFormProps> = ({
    providers,
    selectedProvider,
    formData,
    isEditing,
    isLoading,
    isSaved,
    error,
    onInputChange,
    onProviderTypeChange,
    onGenerateName,
    onSave,
    onCancel,
    onDelete,
    setError
}) => {
    const [dynamicModels, setDynamicModels] = React.useState<string[]>([]);
    const [apiDefaultModel, setApiDefaultModel] = React.useState<string>('');
    const [loadingModels, setLoadingModels] = React.useState(false);
    // Raised while the model list is open so it overlaps the cards below.
    const [isOpen, setIsOpen] = React.useState(false);

    const getProviderDetails = (providerId: string) => {
        return providers.find(p => p.id === providerId) || providers[0];
    };

    const updateSelectedModel = (modelValue: string) => {
        onInputChange({
            target: {
                name: 'selectedModel',
                value: modelValue,
            },
        } as React.ChangeEvent<HTMLInputElement>);
    };

    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        // Limit file size to 1MB to prevent memory issues with massive files
        if (file.size > 1024 * 1024) {
            setError('File is too large. Please upload a valid service account JSON under 1MB.');
            return;
        }

        const reader = new FileReader();
        reader.onload = (event) => {
            const content = event.target?.result as string;
            try {
                const parsed = JSON.parse(content);
                onInputChange({
                    target: {
                        name: 'apiKey',
                        value: content,
                    },
                } as React.ChangeEvent<HTMLInputElement>);

                // Auto-fill project ID if available in JSON
                if (parsed.project_id) {
                    onInputChange({
                        target: {
                            name: 'gcpProjectID',
                            value: parsed.project_id,
                        },
                    } as React.ChangeEvent<HTMLInputElement>);
                }
                setError(null);
            } catch (err) {
                setError('Invalid JSON file. Please upload a valid Google Service Account JSON.');
            }
        };
        reader.readAsText(file);
    };

    // Check if current provider requires API key
    const currentProvider = selectedProvider === 'all' ? formData.providerType : selectedProvider;
    const providerDetails = currentProvider ? getProviderDetails(currentProvider) : null;
    const isOllama = providerDetails?.id === 'ollama';

    const renderCredentialsInput = () => {
        if (currentProvider === 'gemini-enterprise') {
            return (
                <div className="space-y-1">
                    <label className="block text-sm font-medium text-slate-300">
                        Service Account JSON
                    </label>
                    <div className="flex items-center justify-between border border-dashed border-slate-600 bg-slate-700/50 hover:bg-slate-700/80 rounded-lg p-4 transition-all duration-200">
                        <div className="flex items-center space-x-3">
                            <div className="p-2 bg-blue-600/10 rounded-lg text-blue-400">
                                <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                                </svg>
                            </div>
                            <div>
                                <h4 className="text-sm font-medium text-white">
                                    {formData.apiKey ? "Service Account JSON Loaded" : "Upload Credentials File"}
                                </h4>
                                <p className="text-xs text-slate-400">
                                    {formData.apiKey 
                                        ? `Valid Google Cloud service account JSON configured (${(formData.apiKey.length / 1024).toFixed(2)} KB)`
                                        : "Upload the Google Cloud IAM Service Account JSON keyfile"
                                    }
                                </p>
                            </div>
                        </div>
                        <div>
                            <label className="cursor-pointer">
                                <span className="inline-flex items-center px-4 py-2 border border-slate-500 bg-transparent text-sm font-medium rounded-lg text-blue-300 hover:bg-slate-700 hover:text-blue-200 transition-colors focus-within:ring-2 focus-within:ring-offset-2 focus-within:ring-blue-500">
                                    {formData.apiKey ? "Replace File" : "Choose File"}
                                </span>
                                <input
                                    type="file"
                                    accept=".json"
                                    onChange={handleFileChange}
                                    className="sr-only"
                                />
                            </label>
                        </div>
                    </div>
                    <p className="text-xs text-slate-400 mt-2 leading-relaxed">
                        Follow this guide to <a href="https://developers.google.com/workspace/guides/create-credentials#service-account" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:underline">create a service account JSON file</a> and assign the <strong>Agent Platform user</strong> role.
                    </p>
                </div>
            );
        }

        return (
            <Input
                label={isOllama ? "API Key (Optional)" : "API Key"}
                name="apiKey"
                type="password"
                value={formData.apiKey}
                onChange={onInputChange}
                placeholder={
                    selectedProvider === 'all' && formData.providerType
                        ? getProviderDetails(formData.providerType).apiKeyPlaceholder
                        : selectedProvider !== 'all'
                            ? getProviderDetails(selectedProvider).apiKeyPlaceholder
                            : 'Enter API key'
                }
                helperText={
                    isOllama
                        ? "Optional JWT token for Ollama authentication"
                        : "Your API key will be stored securely"
                }
            />
        );
    };

    // Fetch dynamic models on provider change
    React.useEffect(() => {
        if (!currentProvider || isOllama) {
            setDynamicModels([]);
            setApiDefaultModel('');
            return;
        }

        let isMounted = true;
        const fetchModels = async () => {
            setLoadingModels(true);
            try {
                const { getAIProviderModels } = await import('../../../api/connectors');
                const data = await getAIProviderModels(currentProvider);
                if (isMounted) {
                    const modelIds = data.models.map((m: any) => m.model_id);
                    setDynamicModels(modelIds);

                    const defaultModelObj = data.models.find((m: any) => m.is_default);
                    const defaultModel = defaultModelObj ? defaultModelObj.model_id : (modelIds[0] || '');
                    if (defaultModelObj) {
                        setApiDefaultModel(defaultModelObj.model_id);
                    } else if (modelIds.length > 0) {
                        setApiDefaultModel(modelIds[0]);
                    }

                    // If no model is currently selected, find the default and select it
                    if (!formData.selectedModel || formData.selectedModel === '') {
                        if (defaultModel) {
                            updateSelectedModel(defaultModel);
                        }
                    }
                }
            } catch (err) {
                console.error('Failed to load dynamic models:', err);
                if (isMounted) {
                    setDynamicModels([]);
                    setApiDefaultModel('');
                }
            } finally {
                if (isMounted) {
                    setLoadingModels(false);
                }
            }
        };

        fetchModels();
        return () => {
            isMounted = false;
        };
    }, [currentProvider]);

    const providerModels = dynamicModels;

    // The model picker (AIModelSelect, keyed by provider) resets its own search and custom mode on a provider change.
    React.useEffect(() => {
        setApiDefaultModel('');
    }, [currentProvider]);

    // Validation rules
    const isValidForm = () => {
        if (!formData.name) return false;
        if (selectedProvider === 'all' && !formData.providerType) return false;

        // For Ollama, require baseURL but make API key optional
        if (isOllama) {
            return !!formData.baseURL;
        }

        if (currentProvider === 'gemini-enterprise') {
            return (!!formData.apiKey || isEditing) && !!formData.gcpProjectID && !!formData.gcpLocation;
        }

        if ((providerDetails?.id === 'openrouter' || providerDetails?.id === 'atlas') && !formData.selectedModel) {
            return false;
        }

        // Blank api_key is fine when editing - it means keep the existing key.
        return !!formData.apiKey || isEditing;
    };

    return (
        <Card
            className={isOpen ? 'relative z-[100]' : 'relative z-10'}
            title={
                isEditing
                    ? `Edit ${getProviderDetails(selectedProvider).name} Connector`
                    : selectedProvider === 'all'
                        ? "Add New Connector"
                        : `Add ${getProviderDetails(selectedProvider).name} Connector`
            }
        >
            {isSaved && (
                <Alert
                    variant="success"
                    icon={<Icons.Success />}
                    className="mb-4"
                >
                    {selectedProvider === 'all' ? 'AI' : getProviderDetails(selectedProvider).name} connector {isEditing ? 'updated' : 'saved'} successfully!
                </Alert>
            )}

            {error && (
                <Alert
                    variant="error"
                    icon={<Icons.Error />}
                    className="mb-4"
                    onClose={() => setError(null)}
                >
                    {error}
                </Alert>
            )}

            <div className="space-y-5">
                <div className="flex items-center">
                    <div className="mr-4">
                        <div className="h-12 w-12 rounded-full bg-indigo-600 text-white flex items-center justify-center">
                            {selectedProvider === 'all' ? <Icons.AI /> : getProviderDetails(selectedProvider).icon}
                        </div>
                    </div>
                    <div>
                        {selectedProvider === 'all' ? (
                            <div className="space-y-2">
                                <h3 className="text-lg font-medium text-white">
                                    Select Provider
                                </h3>
                                <select
                                    className="block w-full bg-slate-700 border border-slate-600 text-white rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
                                    value={formData.providerType || ''}
                                    onChange={(e) => onProviderTypeChange(e.target.value)}
                                >
                                    <option value="" disabled>Select a provider</option>
                                    {providers.map(p => (
                                        <option key={p.id} value={p.id}>
                                            {p.name}
                                        </option>
                                    ))}
                                </select>
                            </div>
                        ) : (
                            <>
                                <h3 className="text-lg font-medium text-white">
                                    {getProviderDetails(selectedProvider).name}
                                </h3>
                                <p className="text-sm text-slate-300">
                                    {isEditing ? 'Edit existing connector' : 'Add a new connector for this provider'}
                                </p>
                            </>
                        )}
                    </div>
                </div>

                <div className="flex space-x-2">
                    <Input
                        label="Connector Name"
                        name="name"
                        value={formData.name}
                        onChange={onInputChange}
                        placeholder="Enter a name for this connector"
                        className="flex-grow"
                    />
                    <Button
                        variant="outline"
                        className="mt-7"
                        onClick={onGenerateName}
                        title="Generate a friendly name"
                    >
                        Generate
                    </Button>
                </div>

                {renderCredentialsInput()}

                {currentProvider === 'gemini-enterprise' && (
                    <>
                        <Input
                            label="GCP Project ID"
                            name="gcpProjectID"
                            value={formData.gcpProjectID || ''}
                            onChange={onInputChange}
                            placeholder="e.g. my-gcp-project-id"
                        />
                        <Input
                            label="GCP Location (Region)"
                            name="gcpLocation"
                            value={formData.gcpLocation || ''}
                            onChange={onInputChange}
                            placeholder="e.g. us-central1, europe-west9"
                        />
                    </>
                )}

                {/* Model field for providers that require an explicit model */}
                {providerDetails && !isOllama && (
                    <AIModelSelect
                        key={currentProvider}
                        models={providerModels}
                        defaultModel={apiDefaultModel}
                        loading={loadingModels}
                        value={formData.selectedModel || ''}
                        onChange={updateSelectedModel}
                        onOpenChange={setIsOpen}
                        helperText={providerDetails.id === 'openrouter' ? 'OpenRouter model ID' : providerDetails.id === 'atlas' ? 'Atlas Cloud model ID (defaults to DeepSeek-V3)' : 'Select a model or enter a custom model ID'}
                    />
                )}

                {/* Base URL field for providers that support it (like Ollama) */}
                {((selectedProvider !== 'all' && getProviderDetails(selectedProvider).requiresBaseURL) ||
                    (selectedProvider === 'all' && formData.providerType && getProviderDetails(formData.providerType).requiresBaseURL)) && (
                        <Input
                            label="Base URL"
                            name="baseURL"
                            value={formData.baseURL || ''}
                            onChange={onInputChange}
                            placeholder={
                                selectedProvider === 'all' && formData.providerType
                                    ? getProviderDetails(formData.providerType).baseURLPlaceholder
                                    : selectedProvider !== 'all'
                                        ? getProviderDetails(selectedProvider).baseURLPlaceholder
                                        : 'Enter base URL'
                            }
                            helperText="The full API endpoint for your Ollama server (e.g., http://localhost:11434/ollama/api)"
                        />
                    )}

                <div className="flex space-x-3">
                    <Button
                        variant="primary"
                        onClick={onSave}
                        disabled={!isValidForm() || isLoading}
                    >
                        {isLoading ? 'Processing...' : (isEditing ? 'Update' : 'Save')} Connector
                    </Button>
                    <Button
                        variant="outline"
                        onClick={onCancel}
                    >
                        Cancel
                    </Button>
                    {isEditing && onDelete && (
                        <Button
                            variant="danger"
                            onClick={onDelete}
                            className="ml-auto"
                            title="Delete this connector"
                        >
                            Delete
                        </Button>
                    )}
                </div>
            </div>
        </Card>
    );
};

export default ConnectorForm;
