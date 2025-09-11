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
    const getProviderDetails = (providerId: string) => {
        return providers.find(p => p.id === providerId) || providers[0];
    };

    // Check if current provider requires API key
    const currentProvider = selectedProvider === 'all' ? formData.providerType : selectedProvider;
    const providerDetails = currentProvider ? getProviderDetails(currentProvider) : null;
    const isOllama = providerDetails?.id === 'ollama';
    
    // Validation rules
    const isValidForm = () => {
        if (!formData.name) return false;
        if (selectedProvider === 'all' && !formData.providerType) return false;
        
        // For Ollama, require baseURL but make API key optional
        if (isOllama) {
            return !!formData.baseURL;
        }
        
        // For other providers, require API key
        return !!formData.apiKey;
    };

    return (
        <Card title={
            isEditing 
                ? `Edit ${getProviderDetails(selectedProvider).name} Connector` 
                : selectedProvider === 'all'
                    ? "Add New Connector" 
                    : `Add ${getProviderDetails(selectedProvider).name} Connector`
        }>
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
                                            {p.name} {p.supportLevel === 'recommended' ? '(Recommended)' : p.supportLevel === 'experimental' ? '(Experimental)' : ''}
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
                        disabled={
                            !formData.name || 
                            !formData.apiKey || 
                            (selectedProvider === 'all' && !formData.providerType) ||
                            isLoading
                        }
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
