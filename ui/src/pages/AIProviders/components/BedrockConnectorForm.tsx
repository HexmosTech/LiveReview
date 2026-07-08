import React, { useState, useRef, useEffect, useMemo } from 'react';
import { AIProvider } from '../types';
import {
    Card,
    Button,
    Icons,
    Input,
    Alert
} from '../../../components/UIPrimitives';
import { fetchBedrockModels, BedrockModel } from '../../../api/connectors';

interface BedrockConnectorFormProps {
    provider: AIProvider;
    onSave: (accessKeyId: string, secretAccessKey: string, region: string, selectedModel: string, name: string) => void;
    onCancel: () => void;
    isLoading?: boolean;
    error?: string | null;
    setError: (error: string | null) => void;
    editingConnector?: {
        name: string;
        awsAccessKeyID: string;
        secretAccessKey: string;
        awsRegion: string;
        selectedModel: string;
    } | null;
}

const BedrockConnectorForm: React.FC<BedrockConnectorFormProps> = ({
    provider,
    onSave,
    onCancel,
    isLoading = false,
    error,
    setError,
    editingConnector = null
}) => {
    const [formState, setFormState] = useState({
        name: editingConnector?.name || `Bedrock-${Date.now()}`,
        awsAccessKeyID: editingConnector?.awsAccessKeyID || '',
        secretAccessKey: editingConnector?.secretAccessKey || '',
        awsRegion: editingConnector?.awsRegion || '',
        selectedModel: editingConnector?.selectedModel || ''
    });

    const [availableModels, setAvailableModels] = useState<BedrockModel[]>([]);
    const [fetchingModels, setFetchingModels] = useState(false);
    const [modelsFetched, setModelsFetched] = useState(false);
    const [isModelDropdownOpen, setIsModelDropdownOpen] = useState(false);
    const [modelSearchQuery, setModelSearchQuery] = useState('');
    const [customModelMode, setCustomModelMode] = useState(false);
    const modelDropdownRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (modelDropdownRef.current && !modelDropdownRef.current.contains(event.target as Node)) {
                setIsModelDropdownOpen(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    const filteredModels = useMemo(() => {
        if (!modelSearchQuery) return availableModels;
        return availableModels.filter(model =>
            model.model_id.toLowerCase().includes(modelSearchQuery.toLowerCase()) ||
            (model.name || '').toLowerCase().includes(modelSearchQuery.toLowerCase())
        );
    }, [availableModels, modelSearchQuery]);

    const selectedModelDetails = availableModels.find(model => model.model_id === formState.selectedModel);
    const usesCustomModel = !!formState.selectedModel && !selectedModelDetails;
    const shouldShowCustomModelInput = customModelMode || usesCustomModel;

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
        const { name, value } = e.target;
        setFormState(prev => ({
            ...prev,
            [name]: value
        }));

        // Reset model selection if credentials or region change
        if (name === 'awsAccessKeyID' || name === 'secretAccessKey' || name === 'awsRegion') {
            setModelsFetched(false);
            setAvailableModels([]);
            setModelSearchQuery('');
            setCustomModelMode(false);
            setFormState(prev => ({
                ...prev,
                selectedModel: ''
            }));
        }
    };

    const selectModel = (modelId: string) => {
        setCustomModelMode(false);
        setFormState(prev => ({
            ...prev,
            selectedModel: modelId
        }));
        setIsModelDropdownOpen(false);
        setModelSearchQuery('');
    };

    const selectCustomModel = () => {
        setCustomModelMode(true);
        if (!usesCustomModel) {
            setFormState(prev => ({
                ...prev,
                selectedModel: ''
            }));
        }
        setIsModelDropdownOpen(false);
        setModelSearchQuery('');
    };

    const fetchModels = async () => {
        if (!formState.awsRegion.trim()) {
            setError('Region is required');
            return;
        }

        setError(null);
        setFetchingModels(true);

        try {
            const response = await fetchBedrockModels(formState.awsAccessKeyID, formState.secretAccessKey, formState.awsRegion);
            setAvailableModels(response.models);
            setModelsFetched(true);

            if (response.models.length === 0) {
                setError('No foundation models found for this region. Request model access in the AWS Bedrock console first.');
            }
        } catch (err) {
            console.error('Error fetching Bedrock models:', err);
            setError(err instanceof Error ? err.message : 'Failed to fetch models from Bedrock');
            setAvailableModels([]);
            setModelsFetched(false);
        } finally {
            setFetchingModels(false);
        }
    };

    const handleSave = () => {
        if (!formState.name.trim()) {
            setError('Connector name is required');
            return;
        }
        if (!formState.awsAccessKeyID.trim()) {
            setError('AWS Access Key ID is required');
            return;
        }
        if (!formState.secretAccessKey.trim()) {
            setError('AWS Secret Access Key is required');
            return;
        }
        if (!formState.awsRegion.trim()) {
            setError('Region is required');
            return;
        }
        if (!formState.selectedModel) {
            setError('Please select a model');
            return;
        }

        onSave(formState.awsAccessKeyID, formState.secretAccessKey, formState.awsRegion, formState.selectedModel, formState.name);
    };

    const canFetchModels = formState.awsAccessKeyID.trim() && formState.secretAccessKey.trim() && formState.awsRegion.trim() && !fetchingModels;
    const canSave = formState.name.trim() && formState.awsAccessKeyID.trim() && formState.secretAccessKey.trim() && formState.awsRegion.trim() && formState.selectedModel && !isLoading;

    return (
        <Card
            className={isModelDropdownOpen ? 'relative z-[100]' : 'relative z-10'}
            title={editingConnector ? "Edit Bedrock Connector" : "Add Bedrock Connector"}
        >
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
                            {provider.icon}
                        </div>
                    </div>
                    <div>
                        <h3 className="text-lg font-medium text-white">
                            {provider.name}
                        </h3>
                        <p className="text-sm text-slate-300">
                            Connect to AWS Bedrock using your own AWS account
                        </p>
                    </div>
                </div>

                <Input
                    label="Connector Name"
                    name="name"
                    value={formState.name}
                    onChange={handleInputChange}
                    placeholder="Enter a name for this connector"
                />

                <Input
                    label="AWS Access Key ID"
                    name="awsAccessKeyID"
                    value={formState.awsAccessKeyID}
                    onChange={handleInputChange}
                    placeholder="AKIAXXXXXXXXXXXXXXXX"
                />

                <Input
                    label="AWS Secret Access Key"
                    name="secretAccessKey"
                    type="password"
                    value={formState.secretAccessKey}
                    onChange={handleInputChange}
                    placeholder="Enter your AWS Secret Access Key"
                    helperText="Your secret key will be stored securely"
                />

                <Input
                    label="AWS Region"
                    name="awsRegion"
                    value={formState.awsRegion}
                    onChange={handleInputChange}
                    placeholder="e.g. us-east-1, eu-west-1"
                />

                <div className="space-y-3">
                    <div className="flex items-center justify-between">
                        <label className="block text-sm font-medium text-slate-300">
                            Available Models
                        </label>
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={fetchModels}
                            disabled={!canFetchModels}
                            icon={!fetchingModels ? <Icons.Refresh /> : undefined}
                            isLoading={fetchingModels}
                        >
                            {fetchingModels ? 'Fetching...' : 'Fetch Models'}
                        </Button>
                    </div>

                    {/* Show currently selected model from database when editing */}
                    {editingConnector && editingConnector.selectedModel && !modelsFetched && (
                        <div className="text-sm text-blue-400 p-3 bg-blue-900/20 rounded-md border border-blue-700">
                            Currently selected: <strong>{editingConnector.selectedModel}</strong>
                            <br />
                            <span className="text-slate-400">Click "Fetch Models" to see all available models and change selection</span>
                        </div>
                    )}

                    {!modelsFetched && !(editingConnector && editingConnector.selectedModel) && (
                        <div className="text-sm text-slate-400 p-3 bg-slate-800 rounded-md">
                            Click "Fetch Models" to load available foundation models for this region
                        </div>
                    )}

                    {modelsFetched && availableModels.length > 0 && (
                        <div ref={modelDropdownRef} className={`relative w-full ${isModelDropdownOpen ? 'z-[100]' : 'z-0'}`}>
                            <button
                                type="button"
                                className="w-full bg-slate-700 border border-slate-600 text-white rounded-md px-3 py-2 text-left flex justify-between items-center focus:outline-none focus:ring-2 focus:ring-blue-500 cursor-pointer"
                                onClick={() => setIsModelDropdownOpen(!isModelDropdownOpen)}
                            >
                                <span className="truncate">
                                    {shouldShowCustomModelInput
                                        ? 'Custom model…'
                                        : selectedModelDetails
                                            ? (selectedModelDetails.name || selectedModelDetails.model_id)
                                            : 'Select a model'}
                                </span>
                                <span className="text-slate-400 ml-2">
                                    {isModelDropdownOpen ? '▲' : '▼'}
                                </span>
                            </button>

                            {isModelDropdownOpen && (
                                <div className="absolute z-[999] mt-1 w-full bg-slate-800 border border-slate-700 rounded-md shadow-2xl max-h-80 overflow-y-auto focus:outline-none">
                                    <div className="sticky top-0 p-2 bg-slate-800 border-b border-slate-700 z-10">
                                        <input
                                            type="text"
                                            placeholder="Search model name..."
                                            value={modelSearchQuery}
                                            onChange={(e) => setModelSearchQuery(e.target.value)}
                                            className="block w-full bg-slate-900 border border-slate-700 text-white rounded px-2.5 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
                                            autoFocus
                                            onClick={(e) => e.stopPropagation()}
                                        />
                                    </div>

                                    <div className="py-1">
                                        {filteredModels.map(model => {
                                            const isSelected = !shouldShowCustomModelInput && formState.selectedModel === model.model_id;
                                            return (
                                                <button
                                                    key={model.model_id}
                                                    type="button"
                                                    className={`w-full text-left px-3 py-2 text-sm hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${isSelected ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                                        }`}
                                                    onClick={() => selectModel(model.model_id)}
                                                >
                                                    <span className="truncate">{model.name || model.model_id}</span>
                                                </button>
                                            );
                                        })}

                                        {/* Custom Model Option */}
                                        <button
                                            type="button"
                                            className={`w-full text-left px-3 py-2 text-sm border-t border-slate-700 hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${shouldShowCustomModelInput ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                                }`}
                                            onClick={selectCustomModel}
                                        >
                                            <span>Custom model…</span>
                                        </button>

                                        {filteredModels.length === 0 && (
                                            <div className="px-3 py-2 text-sm text-slate-500 italic">No matching models found</div>
                                        )}
                                    </div>
                                </div>
                            )}
                        </div>
                    )}

                    {availableModels.length > 0 && shouldShowCustomModelInput && (
                        <Input
                            label="Custom Model ID"
                            name="selectedModel"
                            value={usesCustomModel ? formState.selectedModel : ''}
                            onChange={handleInputChange}
                            placeholder="e.g. anthropic.claude-3-5-haiku-20241022-v1:0"
                        />
                    )}

                    {modelsFetched && availableModels.length === 0 && (
                        <>
                            <div className="text-sm text-amber-400 p-3 bg-amber-900/20 rounded-md border border-amber-700">
                                No models found. Request access to foundation models in the AWS Bedrock console for this region first, or enter a model ID manually below.
                            </div>
                            <Input
                                label="Custom Model ID"
                                name="selectedModel"
                                value={formState.selectedModel}
                                onChange={handleInputChange}
                                placeholder="e.g. anthropic.claude-3-5-haiku-20241022-v1:0"
                            />
                        </>
                    )}
                </div>

                <div className="flex space-x-3">
                    <Button
                        variant="primary"
                        onClick={handleSave}
                        disabled={!canSave}
                    >
                        {isLoading ? 'Saving...' : (editingConnector ? 'Update Connector' : 'Save Connector')}
                    </Button>
                    <Button
                        variant="outline"
                        onClick={onCancel}
                    >
                        Cancel
                    </Button>
                </div>
            </div>
        </Card>
    );
};

export default BedrockConnectorForm;
