import React, { useState, useEffect } from 'react';
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
import { getConnectors, ConnectorResponse, validateAIProviderKey, createAIConnector } from '../../api/connectors';
import apiClient from '../../api/apiClient';

// AI Provider data structure
interface AIProvider {
    id: string;
    name: string;
    url: string;
    description: string;
    icon: React.ReactNode;
    apiKeyPlaceholder: string;
    models?: string[]; // Available models for this provider
    defaultModel?: string; // Default model to use
}

// AI Connector structure (mapped from API)
interface AIConnector {
    id: string;
    name: string;
    providerName: string;
    apiKey: string;
    displayOrder: number;
    createdAt: Date;
    lastUsed?: Date;
    usageStats?: {
        totalCalls: number;
        successfulCalls: number;
        failedCalls: number;
        averageLatency: number; // in ms
        lastError?: string;
    };
    models?: string[]; // Available models for this connector
    selectedModel?: string; // Currently selected model
    isActive: boolean; // Whether this connector is active or disabled
}

const popularAIProviders: AIProvider[] = [
    { 
        id: 'openai',
        name: 'OpenAI', 
        url: 'https://platform.openai.com/', 
        description: 'Access GPT models for code understanding and generation',
        icon: <Icons.OpenAI />,
        apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['gpt-3.5-turbo', 'gpt-4', 'gpt-4-turbo', 'gpt-4o'],
        defaultModel: 'gpt-4'
    },
    { 
        id: 'gemini',
        name: 'Google Gemini', 
        url: 'https://ai.google.dev/', 
        description: 'Google\'s multimodal AI for code and natural language tasks',
        icon: <Icons.Google />,
        apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['gemini-pro', 'gemini-pro-vision', 'gemini-ultra'],
        defaultModel: 'gemini-pro'
    },
    { 
        id: 'claude',
        name: 'Anthropic Claude', 
        url: 'https://www.anthropic.com/', 
        description: 'Constitutional AI focused on helpful, harmless responses',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'claude-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['claude-3-opus', 'claude-3-sonnet', 'claude-3-haiku'],
        defaultModel: 'claude-3-sonnet'
    },
    { 
        id: 'cohere',
        name: 'Cohere', 
        url: 'https://cohere.com/', 
        description: 'Specialized in understanding and generating human language',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'cohere-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['command', 'command-light', 'command-r', 'command-r-plus'],
        defaultModel: 'command-r'
    },
];

const AIProviders: React.FC = () => {
    // State
    const [selectedProvider, setSelectedProvider] = useState<string>('all');
    const [connectors, setConnectors] = useState<AIConnector[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    const [isSaved, setIsSaved] = useState(false);
    const [isEditing, setIsEditing] = useState(false);
    const [selectedConnector, setSelectedConnector] = useState<AIConnector | null>(null);
    const [showDropdown, setShowDropdown] = useState(false);
    const dropdownRef = React.useRef<HTMLDivElement>(null);
    const [formData, setFormData] = useState({
        name: '',
        apiKey: '',
        providerType: ''
    });
    
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

    // Fetch connectors from API on component mount
    useEffect(() => {
        fetchConnectors();
    }, []);

    const fetchConnectors = async () => {
        try {
            setIsLoading(true);
            const data = await getConnectors();
            
            // Filter for AI connectors (assuming they have a provider name matching our list)
            const aiConnectors = data
                .filter(c => popularAIProviders.map(p => p.id).includes(c.provider))
                .map(connector => ({
                    id: connector.id.toString(),
                    name: connector.connection_name || connector.provider,
                    providerName: connector.provider,
                    apiKey: connector.provider_app_id || '',
                    displayOrder: connector.metadata?.display_order || 0,
                    createdAt: new Date(connector.created_at),
                    lastUsed: connector.metadata?.last_used ? new Date(connector.metadata.last_used) : undefined,
                    usageStats: connector.metadata?.usage_stats || {
                        totalCalls: 0,
                        successfulCalls: 0,
                        failedCalls: 0,
                        averageLatency: 0
                    },
                    models: connector.metadata?.models || [],
                    selectedModel: connector.metadata?.selected_model,
                    isActive: connector.metadata?.is_active !== false // Default to true if not specified
                }));
                
            setConnectors(aiConnectors);
            setError(null);
        } catch (err) {
            console.error('Error fetching connectors:', err);
            setError('Failed to load AI connectors. Please try again.');
        } finally {
            setIsLoading(false);
        }
    };

    // Generate a friendly name for a specific provider
    const generateFriendlyNameForProvider = (providerId: string) => {
        const providerInfo = popularAIProviders.find(p => p.id === providerId);
        
        // Generate a friendly name using adjectives and random numbers
        const adjectives = ['Smart', 'Clever', 'Quick', 'Bright', 'Intelligent', 'Sharp', 'Brilliant', 'Creative'];
        const randomAdjective = adjectives[Math.floor(Math.random() * adjectives.length)];
        const randomNum = Math.floor(Math.random() * 1000);
        
        return `${providerInfo?.name || 'AI'}-${randomAdjective}${randomNum}`;
    };

    // Generate a friendly name for the current provider
    const generateFriendlyName = () => {
        return generateFriendlyNameForProvider(selectedProvider);
    };

    // Handle form input changes
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value } = e.target;
        setFormData({
            ...formData,
            [name]: value
        });
    };

    // Handle save/update connector
    const handleSaveConnector = async () => {
        try {
            // Determine the provider to use
            const providerToUse = selectedProvider === 'all' ? formData.providerType : selectedProvider;
            
            if (!providerToUse) {
                setError('Please select a provider');
                return;
            }
            
            // First validate the API key
            setIsLoading(true);
            
            try {
                const validationResult = await validateAIProviderKey(providerToUse, formData.apiKey);
                
                if (!validationResult.valid) {
                    setError(`API key validation failed: ${validationResult.message}`);
                    setIsLoading(false);
                    return;
                }
            } catch (validationError) {
                console.error('Error validating API key:', validationError);
                setError('Failed to validate API key. Please try again.');
                setIsLoading(false);
                return;
            }
            
            // Now save the connector to the database
            try {
                const displayOrder = connectors.filter(c => c.providerName === providerToUse).length;
                
                if (selectedConnector) {
                    // Update existing connector (not implemented yet in the backend)
                    // For now, just update the UI
                    const updatedConnectors = connectors.map(c => 
                        c.id === selectedConnector.id 
                            ? { 
                                ...c, 
                                name: formData.name || c.name,
                                apiKey: formData.apiKey || c.apiKey 
                            } 
                            : c
                    );
                    setConnectors(updatedConnectors);
                } else {
                    // Create new connector in the backend
                    const result = await createAIConnector(
                        providerToUse,
                        formData.apiKey,
                        formData.name,
                        displayOrder
                    );
                    
                    console.log('Connector created:', result);
                    
                    // After creating, refresh the connector list
                    await fetchConnectors();
                }
            } catch (saveError) {
                console.error('Error saving connector to database:', saveError);
                setError('Failed to save connector to database. Please try again.');
                setIsLoading(false);
                return;
            }
            
            // Show success message
            setIsSaved(true);
            setTimeout(() => setIsSaved(false), 3000);
            
            // Reset form
            resetForm();
        } catch (error) {
            console.error('Error saving connector:', error);
            setError('Failed to save connector. Please try again.');
        } finally {
            setIsLoading(false);
        }
    };

    // Reset form
    const resetForm = () => {
        setFormData({
            name: '',
            apiKey: '',
            providerType: ''
        });
        setSelectedConnector(null);
        setIsEditing(false);
        setShowDropdown(false);
    };

    // Handle editing a connector
    const handleEditConnector = (connector: AIConnector) => {
        setSelectedConnector(connector);
        setSelectedProvider(connector.providerName);
        setFormData({
            name: connector.name,
            apiKey: connector.apiKey,
            providerType: connector.providerName
        });
        setIsEditing(true);
    };

    // Generate friendly name button
    const handleGenerateName = () => {
        const providerToUse = selectedProvider === 'all' ? formData.providerType : selectedProvider;
        if (!providerToUse) {
            setError('Please select a provider first');
            return;
        }
        
        setFormData({
            ...formData,
            name: generateFriendlyNameForProvider(providerToUse)
        });
    };

    // Get provider details
    const getProviderDetails = (providerId: string) => {
        return popularAIProviders.find(p => p.id === providerId) || popularAIProviders[0];
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <PageHeader 
                title="AI Providers" 
                description="Configure and manage AI services for code review"
            />

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                {/* Left panel for selecting providers */}
                <div className="lg:col-span-1">
                    <Card title="AI Providers">
                        <ul className="space-y-2">
                            {/* All Connectors option */}
                            <li 
                                key="all-connectors"
                                className={`p-3 rounded-lg cursor-pointer transition-all ${
                                    selectedProvider === 'all' 
                                        ? 'bg-slate-700 border-l-4 border-blue-500' 
                                        : 'hover:bg-slate-700'
                                }`}
                                onClick={() => {
                                    setSelectedProvider('all');
                                    resetForm();
                                    setShowDropdown(false);
                                }}
                            >
                                <div className="flex items-center">
                                    <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center mr-3">
                                        <Icons.Dashboard />
                                    </div>
                                    <div>
                                        <h3 className="font-medium text-white">All Connectors</h3>
                                        <div className="flex items-center mt-1">
                                            <Badge 
                                                variant="primary" 
                                                size="sm"
                                            >
                                                {connectors.length} 
                                                {' '}total
                                            </Badge>
                                        </div>
                                    </div>
                                </div>
                            </li>
                            
                            {/* Individual provider options */}
                            {popularAIProviders.map((provider) => (
                                <li 
                                    key={provider.id}
                                    className={`p-3 rounded-lg cursor-pointer transition-all ${
                                        selectedProvider === provider.id 
                                            ? 'bg-slate-700 border-l-4 border-blue-500' 
                                            : 'hover:bg-slate-700'
                                    }`}
                                    onClick={() => {
                                        setSelectedProvider(provider.id);
                                        setShowDropdown(false);
                                        if (!isEditing) {
                                            setFormData({
                                                ...formData,
                                                name: '',
                                                providerType: provider.id
                                            });
                                        }
                                    }}
                                >
                                    <div className="flex items-center">
                                        <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center mr-3">
                                            {provider.icon}
                                        </div>
                                        <div>
                                            <h3 className="font-medium text-white">{provider.name}</h3>
                                            <div className="flex items-center mt-1">
                                                <Badge 
                                                    variant="primary" 
                                                    size="sm"
                                                >
                                                    {connectors.filter(c => c.providerName === provider.id).length} 
                                                    {' '}key{connectors.filter(c => c.providerName === provider.id).length !== 1 ? 's' : ''}
                                                </Badge>
                                            </div>
                                        </div>
                                    </div>
                                </li>
                            ))}
                        </ul>
                        
                        {/* Provider Info - Show only for specific providers, not for "all" view */}
                        {selectedProvider !== 'all' && (
                            <div className="mt-6 p-4 bg-slate-700 rounded-lg">
                                <h3 className="text-lg font-medium text-white mb-2">
                                    {getProviderDetails(selectedProvider).name}
                                </h3>
                                <p className="text-sm text-slate-300 mb-3">
                                    {getProviderDetails(selectedProvider).description}
                                </p>
                                <a 
                                    href={getProviderDetails(selectedProvider).url} 
                                    target="_blank" 
                                    rel="noopener noreferrer" 
                                    className="text-sm text-blue-400 hover:text-blue-300 flex items-center"
                                >
                                    Visit Documentation
                                </a>
                            </div>
                        )}
                    </Card>
                    
                    {/* Usage Tips */}
                    <Card title="Usage Tips" className="mt-6">
                        <div className="space-y-4">
                            <div className="flex items-start">
                                <div className="text-blue-400 mt-1 mr-2 flex-shrink-0">
                                    <Icons.Info />
                                </div>
                                <p className="text-sm text-slate-300">
                                    Multiple API keys for the same provider will be used in order of their display position.
                                </p>
                            </div>
                            <div className="flex items-start">
                                <div className="text-blue-400 mt-1 mr-2 flex-shrink-0">
                                    <Icons.Info />
                                </div>
                                <p className="text-sm text-slate-300">
                                    If a key hits rate limits or fails, the system will automatically try the next key.
                                </p>
                            </div>
                        </div>
                    </Card>
                </div>
                
                {/* Main content area - 2 columns */}
                <div className="lg:col-span-2">
                    <div className="grid grid-cols-1 gap-6">
                        {/* Connector Form - Only show when actively adding/editing */}
                        {(formData.name || formData.apiKey || isEditing) && (
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
                                                        onChange={(e) => {
                                                            const providerType = e.target.value;
                                                            setFormData({
                                                                ...formData,
                                                                providerType,
                                                                name: formData.name || generateFriendlyNameForProvider(providerType)
                                                            });
                                                        }}
                                                    >
                                                        <option value="" disabled>Select a provider</option>
                                                        {popularAIProviders.map(p => (
                                                            <option key={p.id} value={p.id}>{p.name}</option>
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
                                            onChange={handleInputChange}
                                            placeholder="Enter a name for this connector"
                                            className="flex-grow"
                                        />
                                        <Button
                                            variant="outline"
                                            className="mt-7"
                                            onClick={handleGenerateName}
                                            title="Generate a friendly name"
                                        >
                                            Generate
                                        </Button>
                                    </div>
                                    
                                    <Input
                                        label="API Key"
                                        name="apiKey"
                                        type="password"
                                        value={formData.apiKey}
                                        onChange={handleInputChange}
                                        placeholder={
                                            selectedProvider === 'all' && formData.providerType
                                                ? getProviderDetails(formData.providerType).apiKeyPlaceholder
                                                : selectedProvider !== 'all'
                                                    ? getProviderDetails(selectedProvider).apiKeyPlaceholder
                                                    : 'Enter API key'
                                        }
                                        helperText="Your API key will be stored securely"
                                    />
                                    
                                    <div className="flex space-x-3">
                                        <Button
                                            variant="primary"
                                            onClick={handleSaveConnector}
                                            disabled={
                                                !formData.name || 
                                                !formData.apiKey || 
                                                (selectedProvider === 'all' && !formData.providerType)
                                            }
                                        >
                                            {isEditing ? 'Update' : 'Save'} Connector
                                        </Button>
                                        <Button
                                            variant="outline"
                                            onClick={resetForm}
                                        >
                                            Cancel
                                        </Button>
                                    </div>
                                </div>
                            </Card>
                        )}
                        
                        {/* Connectors List */}
                        <Card 
                            title="Your Connectors" 
                            badge={`${selectedProvider === 'all' ? connectors.length : connectors.filter(c => c.providerName === selectedProvider).length}`}
                        >
                            <div className="flex justify-end mb-4">
                                {selectedProvider === 'all' ? (
                                    <div className="relative" ref={dropdownRef}>
                                        <Button
                                            variant="primary"
                                            size="sm"
                                            onClick={() => setShowDropdown(!showDropdown)}
                                            className="flex items-center"
                                        >
                                            Add Connector
                                            <span className="ml-1">
                                                <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                                                </svg>
                                            </span>
                                        </Button>
                                        
                                        {showDropdown && (
                                            <div className="absolute right-0 mt-2 w-56 rounded-md shadow-lg bg-slate-800 ring-1 ring-black ring-opacity-5 z-10">
                                                <div className="py-1" role="menu" aria-orientation="vertical">
                                                    <div className="px-3 py-2 text-xs text-slate-400 uppercase">
                                                        Select Provider
                                                    </div>
                                                    {popularAIProviders.map(provider => (
                                                        <button
                                                            key={provider.id}
                                                            className="w-full text-left px-4 py-2 text-sm text-white hover:bg-slate-700 flex items-center"
                                                            onClick={() => {
                                                                setFormData({
                                                                    name: generateFriendlyNameForProvider(provider.id),
                                                                    apiKey: '',
                                                                    providerType: provider.id
                                                                });
                                                                setIsEditing(false);
                                                                setSelectedConnector(null);
                                                                setShowDropdown(false);
                                                            }}
                                                        >
                                                            <span className="w-8 h-8 flex-shrink-0 rounded-full bg-indigo-600 flex items-center justify-center mr-3">
                                                                {provider.icon}
                                                            </span>
                                                            {provider.name}
                                                        </button>
                                                    ))}
                                                </div>
                                            </div>
                                        )}
                                    </div>
                                ) : (
                                    <Button
                                        variant="primary"
                                        size="sm"
                                        onClick={() => {
                                            setFormData({
                                                name: generateFriendlyName(),
                                                apiKey: '',
                                                providerType: selectedProvider
                                            });
                                            setIsEditing(false);
                                            setSelectedConnector(null);
                                        }}
                                    >
                                        Add {getProviderDetails(selectedProvider).name} Connector
                                    </Button>
                                )}
                            </div>
                            {isLoading ? (
                                <div className="flex justify-center items-center py-8">
                                    <Spinner size="md" color="text-blue-400" />
                                    <span className="ml-3 text-slate-300">Loading connectors...</span>
                                </div>
                            ) : error ? (
                                <div className="p-4 text-center">
                                    <Icons.Error />
                                    <p className="text-red-400 mt-2">{error}</p>
                                    <Button 
                                        variant="outline" 
                                        size="sm" 
                                        className="mt-3"
                                        onClick={() => fetchConnectors()}
                                    >
                                        Retry
                                    </Button>
                                </div>
                            ) : selectedProvider === 'all' ? (
                                // All connectors view
                                connectors.length === 0 ? (
                                    <EmptyState
                                        icon={<Icons.EmptyState />}
                                        title="No AI connectors yet"
                                        description="Add your first AI connector to start using AI services"
                                    />
                                ) : (
                                    <ul className="space-y-4">
                                        {connectors
                                            .sort((a, b) => a.displayOrder - b.displayOrder)
                                            .map((connector, index) => (
                                                <li
                                                    key={connector.id}
                                                    className="border border-slate-600 rounded-lg bg-slate-700 hover:bg-slate-600 transition-colors"
                                                >
                                                    <div className="p-4">
                                                        <div className="flex justify-between items-center">
                                                            <div className="flex items-center">
                                                                <div className="flex-shrink-0 mr-3 relative">
                                                                    <Avatar 
                                                                        size="md"
                                                                        initials={connector.name.charAt(0).toUpperCase()}
                                                                    />
                                                                    <span className="absolute -top-1 -right-1 bg-blue-500 text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
                                                                        {index + 1}
                                                                    </span>
                                                                </div>
                                                                <div>
                                                                    <div className="flex items-center">
                                                                        <h3 className="font-medium text-white">
                                                                            {connector.name}
                                                                        </h3>
                                                                        <Badge 
                                                                            variant="primary" 
                                                                            size="sm" 
                                                                            className="ml-2"
                                                                        >
                                                                            {popularAIProviders.find(p => p.id === connector.providerName)?.name || connector.providerName}
                                                                        </Badge>
                                                                    </div>
                                                                    <p className="text-sm text-slate-300">
                                                                        {connector.apiKey.substring(0, 6)}...{connector.apiKey.substring(connector.apiKey.length - 4)}
                                                                    </p>
                                                                </div>
                                                            </div>
                                                            <div className="flex items-center space-x-2">
                                                                {connector.createdAt && (
                                                                    <span className="text-xs text-slate-300">
                                                                        {connector.createdAt.toLocaleDateString()}
                                                                    </span>
                                                                )}
                                                                <div className="flex space-x-1">
                                                                    <Button
                                                                        variant="secondary"
                                                                        size="sm"
                                                                        onClick={() => handleEditConnector(connector)}
                                                                    >
                                                                        Edit
                                                                    </Button>
                                                                    {index > 0 && (
                                                                        <Button
                                                                            variant="outline"
                                                                            size="sm"
                                                                            title="Move up in priority"
                                                                            onClick={() => {
                                                                                // Move connector up in order
                                                                                const updatedConnectors = [...connectors];
                                                                                
                                                                                // Swap display order with connector above
                                                                                const temp = updatedConnectors[index].displayOrder;
                                                                                updatedConnectors[index].displayOrder = updatedConnectors[index - 1].displayOrder;
                                                                                updatedConnectors[index - 1].displayOrder = temp;
                                                                                
                                                                                setConnectors(updatedConnectors);
                                                                            }}
                                                                        >
                                                                            ↑
                                                                        </Button>
                                                                    )}
                                                                    {index < connectors.length - 1 && (
                                                                        <Button
                                                                            variant="outline"
                                                                            size="sm"
                                                                            title="Move down in priority"
                                                                            onClick={() => {
                                                                                // Move connector down in order
                                                                                const updatedConnectors = [...connectors];
                                                                                
                                                                                // Swap display order with connector below
                                                                                const temp = updatedConnectors[index].displayOrder;
                                                                                updatedConnectors[index].displayOrder = updatedConnectors[index + 1].displayOrder;
                                                                                updatedConnectors[index + 1].displayOrder = temp;
                                                                                
                                                                                setConnectors(updatedConnectors);
                                                                            }}
                                                                        >
                                                                            ↓
                                                                        </Button>
                                                                    )}
                                                                </div>
                                                            </div>
                                                        </div>
                                                    </div>
                                                </li>
                                            ))}
                                    </ul>
                                )
                            ) : (
                                // Provider-specific view
                                connectors.filter(c => c.providerName === selectedProvider).length === 0 ? (
                                    <EmptyState
                                        icon={<Icons.EmptyState />}
                                        title={`No ${getProviderDetails(selectedProvider).name} connectors yet`}
                                        description={`Add your first ${getProviderDetails(selectedProvider).name} connector to start using this AI service`}
                                    />
                                ) : (
                                    <ul className="space-y-4">
                                        {connectors
                                            .filter(c => c.providerName === selectedProvider)
                                            .sort((a, b) => a.displayOrder - b.displayOrder)
                                            .map((connector, index) => (
                                                <li
                                                    key={connector.id}
                                                    className="border border-slate-600 rounded-lg bg-slate-700 hover:bg-slate-600 transition-colors"
                                                >
                                                    <div className="p-4">
                                                        <div className="flex justify-between items-center">
                                                            <div className="flex items-center">
                                                                <div className="flex-shrink-0 mr-3 relative">
                                                                    <Avatar 
                                                                        size="md"
                                                                        initials={connector.name.charAt(0).toUpperCase()}
                                                                    />
                                                                    <span className="absolute -top-1 -right-1 bg-blue-500 text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
                                                                        {index + 1}
                                                                    </span>
                                                                </div>
                                                                <div>
                                                                    <div className="flex items-center">
                                                                        <h3 className="font-medium text-white">
                                                                            {connector.name}
                                                                        </h3>
                                                                    </div>
                                                                    <p className="text-sm text-slate-300">
                                                                        {connector.apiKey.substring(0, 6)}...{connector.apiKey.substring(connector.apiKey.length - 4)}
                                                                    </p>
                                                                </div>
                                                            </div>
                                                            <div className="flex items-center space-x-2">
                                                                {connector.createdAt && (
                                                                    <span className="text-xs text-slate-300">
                                                                        {connector.createdAt.toLocaleDateString()}
                                                                    </span>
                                                                )}
                                                                <div className="flex space-x-1">
                                                                    <Button
                                                                        variant="secondary"
                                                                        size="sm"
                                                                        onClick={() => handleEditConnector(connector)}
                                                                    >
                                                                        Edit
                                                                    </Button>
                                                                    {index > 0 && (
                                                                        <Button
                                                                            variant="outline"
                                                                            size="sm"
                                                                            title="Move up in priority"
                                                                            onClick={() => {
                                                                                // Move connector up in order
                                                                                const updatedConnectors = [...connectors];
                                                                                const currentProvider = updatedConnectors.filter(c => c.providerName === selectedProvider);
                                                                                
                                                                                // Swap display order with connector above
                                                                                const currentIndex = currentProvider.findIndex(c => c.id === connector.id);
                                                                                if (currentIndex > 0) {
                                                                                    const temp = currentProvider[currentIndex].displayOrder;
                                                                                    currentProvider[currentIndex].displayOrder = currentProvider[currentIndex - 1].displayOrder;
                                                                                    currentProvider[currentIndex - 1].displayOrder = temp;
                                                                                }
                                                                                
                                                                                setConnectors(updatedConnectors);
                                                                            }}
                                                                        >
                                                                            ↑
                                                                        </Button>
                                                                    )}
                                                                    {index < connectors.filter(c => c.providerName === selectedProvider).length - 1 && (
                                                                        <Button
                                                                            variant="outline"
                                                                            size="sm"
                                                                            title="Move down in priority"
                                                                            onClick={() => {
                                                                                // Move connector down in order
                                                                                const updatedConnectors = [...connectors];
                                                                                const currentProvider = updatedConnectors.filter(c => c.providerName === selectedProvider);
                                                                                
                                                                                // Swap display order with connector below
                                                                                const currentIndex = currentProvider.findIndex(c => c.id === connector.id);
                                                                                if (currentIndex < currentProvider.length - 1) {
                                                                                    const temp = currentProvider[currentIndex].displayOrder;
                                                                                    currentProvider[currentIndex].displayOrder = currentProvider[currentIndex + 1].displayOrder;
                                                                                    currentProvider[currentIndex + 1].displayOrder = temp;
                                                                                }
                                                                                
                                                                                setConnectors(updatedConnectors);
                                                                            }}
                                                                        >
                                                                            ↓
                                                                        </Button>
                                                                    )}
                                                                </div>
                                                            </div>
                                                        </div>
                                                    </div>
                                                </li>
                                            ))}
                                    </ul>
                                )
                            )}
                        </Card>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default AIProviders;
