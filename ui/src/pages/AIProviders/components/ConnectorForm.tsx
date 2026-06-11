import React from 'react';
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
    const [customModelMode, setCustomModelMode] = React.useState(false);
    const [dynamicModels, setDynamicModels] = React.useState<string[]>([]);
    const [apiDefaultModel, setApiDefaultModel] = React.useState<string>('');
    const [loadingModels, setLoadingModels] = React.useState(false);
    const [searchQuery, setSearchQuery] = React.useState('');
    const [isOpen, setIsOpen] = React.useState(false);
    const dropdownRef = React.useRef<HTMLDivElement>(null);

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

    // Close dropdown on click outside
    React.useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    // Check if current provider requires API key
    const currentProvider = selectedProvider === 'all' ? formData.providerType : selectedProvider;
    const providerDetails = currentProvider ? getProviderDetails(currentProvider) : null;
    const isOllama = providerDetails?.id === 'ollama';

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
    const currentModelValue = formData.selectedModel || apiDefaultModel || '';
    const usesCustomModel = !!currentModelValue && !providerModels.includes(currentModelValue);
    const shouldShowCustomModelInput = customModelMode || usesCustomModel;

    const filteredModels = React.useMemo(() => {
        if (!searchQuery) return providerModels;
        return providerModels.filter((model: string) =>
            model.toLowerCase().includes(searchQuery.toLowerCase())
        );
    }, [providerModels, searchQuery]);

    // Sync custom mode when provider changes or model becomes valid
    React.useEffect(() => {
        setCustomModelMode(false);
        setSearchQuery('');
        setApiDefaultModel('');
    }, [currentProvider]);

    React.useEffect(() => {
        // Auto-exit custom mode only if the model matches a standard one and is non-empty.
        // This prevents the input from disappearing while the user is actively typing.
        if (!usesCustomModel && formData.selectedModel !== '') {
            setCustomModelMode(false);
        }
    }, [usesCustomModel, formData.selectedModel]);

    // Validation rules
    const isValidForm = () => {
        if (!formData.name) return false;
        if (selectedProvider === 'all' && !formData.providerType) return false;

        // For Ollama, require baseURL but make API key optional
        if (isOllama) {
            return !!formData.baseURL;
        }

        if ((providerDetails?.id === 'openrouter' || providerDetails?.id === 'atlas') && !formData.selectedModel) {
            return false;
        }

        // For other providers, require API key
        return !!formData.apiKey;
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
                    helperText={isOllama ? "Optional JWT token for Ollama authentication" : "Your API key will be stored securely"}
                />

                {/* Model field for providers that require an explicit model */}
                {providerDetails && !isOllama && (
                    <div className="space-y-1 relative z-[50]">
                        <div className="flex justify-between items-center">
                            <label className="block text-sm font-medium text-slate-300">Model</label>
                            {loadingModels && <span className="text-xs text-blue-400 animate-pulse">Syncing models from server...</span>}
                        </div>
                        {/* Custom Searchable Select Dropdown */}
                        <div ref={dropdownRef} className={`relative w-full ${isOpen ? 'z-[100]' : 'z-0'}`}>
                            <button
                                type="button"
                                className="w-full bg-slate-700 border border-slate-600 text-white rounded-md px-3 py-2 text-left flex justify-between items-center focus:outline-none focus:ring-2 focus:ring-blue-500 cursor-pointer disabled:cursor-not-allowed disabled:opacity-80"
                                onClick={() => setIsOpen(!isOpen)}
                                disabled={loadingModels}
                            >
                                <span className="truncate flex items-center">
                                    {loadingModels && (
                                        <svg className="animate-spin -ml-1 mr-2.5 h-4 w-4 text-blue-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                        </svg>
                                    )}
                                    <span className="truncate">
                                        {loadingModels
                                            ? 'Loading models...'
                                            : shouldShowCustomModelInput
                                                ? 'Custom model…'
                                                : currentModelValue
                                                    ? `${currentModelValue}${currentModelValue === apiDefaultModel ? ' (Default)' : ''}`
                                                    : 'Select a model'}
                                    </span>
                                </span>
                                <span className="text-slate-400 ml-2">
                                    {isOpen ? '▲' : '▼'}
                                </span>
                            </button>

                            {isOpen && (
                                <div className="absolute z-[999] mt-1 w-full bg-slate-800 border border-slate-700 rounded-md shadow-2xl max-h-80 overflow-y-auto focus:outline-none">
                                    {/* Search Input inside the dropdown menu */}
                                    <div className="sticky top-0 p-2 bg-slate-800 border-b border-slate-700 z-10">
                                        <input
                                            type="text"
                                            placeholder="Search model name..."
                                            value={searchQuery}
                                            onChange={(e) => setSearchQuery(e.target.value)}
                                            className="block w-full bg-slate-900 border border-slate-700 text-white rounded px-2.5 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
                                            autoFocus
                                            onClick={(e) => e.stopPropagation()} // Prevent closing dropdown on clicking input
                                        />
                                    </div>

                                    {/* Options List */}
                                    <div className="py-1">
                                        {loadingModels ? (
                                            <div className="flex items-center justify-center py-6 text-slate-400 space-x-2">
                                                <svg className="animate-spin h-5 w-5 text-indigo-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                                </svg>
                                                <span>Loading models...</span>
                                            </div>
                                        ) : (
                                            <>
                                                {filteredModels.map((model: string) => {
                                                    const isSelected = !shouldShowCustomModelInput && currentModelValue === model;
                                                    return (
                                                        <button
                                                            key={model}
                                                            type="button"
                                                            className={`w-full text-left px-3 py-2 text-sm hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${isSelected ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                                                }`}
                                                            onClick={() => {
                                                                setCustomModelMode(false);
                                                                updateSelectedModel(model);
                                                                setIsOpen(false);
                                                                setSearchQuery('');
                                                            }}
                                                        >
                                                            <span className="truncate">{model}</span>
                                                            {model === apiDefaultModel && (
                                                                <span className="text-[10px] bg-slate-700 text-slate-300 px-1.5 py-0.5 rounded">Default</span>
                                                            )}
                                                        </button>
                                                    );
                                                })}

                                                {/* Custom Model Option */}
                                                <button
                                                    type="button"
                                                    className={`w-full text-left px-3 py-2 text-sm border-t border-slate-700 hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${shouldShowCustomModelInput ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                                        }`}
                                                    onClick={() => {
                                                        setCustomModelMode(true);
                                                        if (!usesCustomModel) {
                                                            updateSelectedModel('');
                                                        }
                                                        setIsOpen(false);
                                                        setSearchQuery('');
                                                    }}
                                                >
                                                    <span>Custom model…</span>
                                                </button>

                                                {filteredModels.length === 0 && (
                                                    <div className="px-3 py-2 text-sm text-slate-500 italic">No matching models found</div>
                                                )}
                                            </>
                                        )}
                                    </div>
                                </div>
                            )}
                        </div>
                        {shouldShowCustomModelInput && (
                            <Input
                                label="Custom Model ID"
                                name="selectedModel"
                                value={usesCustomModel ? currentModelValue : ''}
                                onChange={onInputChange}
                                placeholder="Enter model ID (e.g., gpt-4.1, o3-mini)"
                            />
                        )}
                        <p className="text-xs text-slate-400">
                            {providerDetails.id === 'openrouter' ? 'OpenRouter model ID (defaults to free DeepSeek route)' : providerDetails.id === 'atlas' ? 'Atlas Cloud model ID (defaults to DeepSeek-V3)' : 'Select a model or enter a custom model ID'}
                        </p>
                    </div>
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
