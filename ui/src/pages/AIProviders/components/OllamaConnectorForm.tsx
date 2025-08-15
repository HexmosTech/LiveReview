import React, { useState } from 'react';
import { AIProvider } from '../types';
import { 
    Card, 
    Button, 
    Icons, 
    Input,
    Alert,
    Spinner
} from '../../../components/UIPrimitives';
import { fetchOllamaModels } from '../../../api/connectors';

interface OllamaConnectorFormProps {
    provider: AIProvider;
    onSave: (baseURL: string, jwtToken: string, selectedModel: string, name: string) => void;
    onCancel: () => void;
    isLoading?: boolean;
    error?: string | null;
    setError: (error: string | null) => void;
    editingConnector?: {
        name: string;
        baseURL: string;
        jwtToken: string;
        selectedModel: string;
    } | null;
}

const OllamaConnectorForm: React.FC<OllamaConnectorFormProps> = ({
    provider,
    onSave,
    onCancel,
    isLoading = false,
    error,
    setError,
    editingConnector = null
}) => {
    const [formState, setFormState] = useState({
        name: editingConnector?.name || `Ollama-${Date.now()}`,
        baseURL: editingConnector?.baseURL || 'http://localhost:11434/ollama/api',
        jwtToken: editingConnector?.jwtToken || '',
        selectedModel: editingConnector?.selectedModel || ''
    });
    
    const [availableModels, setAvailableModels] = useState<string[]>([]);
    const [fetchingModels, setFetchingModels] = useState(false);
    const [modelsFetched, setModelsFetched] = useState(false);

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
        const { name, value } = e.target;
        setFormState(prev => ({
            ...prev,
            [name]: value
        }));
        
        // Reset model selection if base URL or JWT changes
        if (name === 'baseURL' || name === 'jwtToken') {
            setModelsFetched(false);
            setAvailableModels([]);
            setFormState(prev => ({
                ...prev,
                selectedModel: ''
            }));
        }
    };

    const fetchModels = async () => {
        if (!formState.baseURL.trim()) {
            setError('Base URL is required');
            return;
        }

        setError(null);
        setFetchingModels(true);
        
        try {
            const response = await fetchOllamaModels(formState.baseURL, formState.jwtToken || undefined);
            setAvailableModels(response.models);
            setModelsFetched(true);
            
            if (response.models.length === 0) {
                setError('No models found in this Ollama instance. Please pull some models first.');
            }
        } catch (err) {
            console.error('Error fetching Ollama models:', err);
            setError(err instanceof Error ? err.message : 'Failed to fetch models from Ollama instance');
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
        if (!formState.baseURL.trim()) {
            setError('Base URL is required');
            return;
        }
        if (!formState.selectedModel) {
            setError('Please select a model');
            return;
        }

        onSave(formState.baseURL, formState.jwtToken, formState.selectedModel, formState.name);
    };

    const canFetchModels = formState.baseURL.trim() && !fetchingModels;
    const canSave = formState.name.trim() && formState.baseURL.trim() && formState.selectedModel && !isLoading;

    return (
        <Card title={editingConnector ? "Edit Ollama Connector" : "Add Ollama Connector"}>
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
                            Connect to your local Ollama instance
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
                    label="Base URL"
                    name="baseURL"
                    value={formState.baseURL}
                    onChange={handleInputChange}
                    placeholder="http://localhost:11434/ollama/api"
                    helperText="The full API endpoint for your Ollama server (e.g., http://localhost:11434/ollama/api)"
                />
                
                <Input
                    label="JWT Token (Optional)"
                    name="jwtToken"
                    type="password"
                    value={formState.jwtToken}
                    onChange={handleInputChange}
                    placeholder="Optional JWT token for authentication"
                    helperText="Leave empty if your Ollama instance doesn't require authentication"
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
                            icon={fetchingModels ? <Spinner className="w-4 h-4" /> : <Icons.Refresh />}
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
                    
                    {!modelsFetched && (
                        <div className="text-sm text-slate-400 p-3 bg-slate-800 rounded-md">
                            Click "Fetch Models" to load available models from your Ollama instance
                        </div>
                    )}
                    
                    {modelsFetched && availableModels.length > 0 && (
                        <select
                            name="selectedModel"
                            value={formState.selectedModel}
                            onChange={handleInputChange}
                            className="block w-full bg-slate-700 border border-slate-600 text-white rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
                        >
                            <option value="">Select a model</option>
                            {availableModels.map(model => (
                                <option key={model} value={model}>{model}</option>
                            ))}
                        </select>
                    )}
                    
                    {modelsFetched && availableModels.length === 0 && (
                        <div className="text-sm text-amber-400 p-3 bg-amber-900/20 rounded-md border border-amber-700">
                            No models found. Use <code>ollama pull &lt;model-name&gt;</code> to download models first.
                        </div>
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

export default OllamaConnectorForm;
