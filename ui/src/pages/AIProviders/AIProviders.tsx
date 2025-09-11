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

// Types
import { AIProvider, AIConnector } from './types';

// Hooks
import { useProviderSelection, useConnectors, useFormState } from './hooks';

// Components
import { 
    ProvidersList, 
    ProviderDetail, 
    ConnectorForm, 
    ConnectorsList,
    UsageTips
} from './components';
import OllamaConnectorForm from './components/OllamaConnectorForm';

// Utils
import { generateFriendlyNameForProvider, getProviderDetails } from './utils/nameUtils';

// Constant data
// Order: recommended providers (Gemini, Ollama) first, then experimental ones.
const popularAIProviders: AIProvider[] = [
    { 
        id: 'gemini',
        name: 'Google Gemini', 
        url: 'https://ai.google.dev/', 
        description: 'High quality, multimodal reasoning. Recommended for balanced cost + performance.',
        icon: <Icons.Google />,
        apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['gemini-pro', 'gemini-pro-vision', 'gemini-ultra'],
        defaultModel: 'gemini-pro',
        supportLevel: 'recommended'
    },
    { 
        id: 'ollama',
        name: 'Ollama', 
        url: 'https://ollama.ai/', 
        description: 'Run models locally. Great for privacy & air‑gapped workflows.',
        icon: <Icons.Ollama />,
        apiKeyPlaceholder: 'Optional JWT token for authentication',
        models: ['llama3', 'llama3.1', 'codellama', 'mistral', 'gemma'],
        defaultModel: 'llama3',
        baseURLPlaceholder: 'http://localhost:11434/ollama/api',
        requiresBaseURL: true,
        supportLevel: 'recommended'
    },
    { 
        id: 'openai',
        name: 'OpenAI', 
        url: 'https://platform.openai.com/', 
        description: 'Powerful GPT models; full first‑class support coming soon.',
        icon: <Icons.OpenAI />,
        apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['gpt-3.5-turbo', 'gpt-4', 'gpt-4-turbo', 'gpt-4o'],
        defaultModel: 'gpt-4',
        supportLevel: 'experimental'
    },
    { 
        id: 'claude',
        name: 'Anthropic Claude', 
        url: 'https://www.anthropic.com/', 
        description: 'Advanced reasoning & long context. Vote to prioritize deeper integration.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'claude-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['claude-3-opus', 'claude-3-sonnet', 'claude-3-haiku'],
        defaultModel: 'claude-3-sonnet',
        supportLevel: 'experimental'
    },
    { 
        id: 'cohere',
        name: 'Cohere', 
        url: 'https://cohere.com/', 
        description: 'Language focused LLMs. Community votes help unlock full support sooner.',
        icon: <Icons.AI />,
        apiKeyPlaceholder: 'cohere-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        models: ['command', 'command-light', 'command-r', 'command-r-plus'],
        defaultModel: 'command-r',
        supportLevel: 'experimental'
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
    const dropdownRef = useRef<HTMLDivElement>(null);
    
    // Calculate provider connector counts
    const connectorCounts = connectors.reduce((counts: Record<string, number>, connector) => {
        counts[connector.providerName] = (counts[connector.providerName] || 0) + 1;
        return counts;
    }, {});
    
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
            providerType: selectedProvider === 'all' ? '' : selectedProvider
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
            providerType: providerId
        });
        setIsEditing(false);
        setSelectedConnector(null);
        setShowDropdown(false);
        updateUrlFragment(providerId, 'add');
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
                formData.apiKey,
                formData.name,
                selectedConnector,
                formData.baseURL,
                formData.selectedModel
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
                    "Configure and manage AI services for code review. Gemini & Ollama are recommended today. Other providers are experimental—help prioritize full support by voting in the community discussion."
                }
                actions={<a href="https://github.com/HexmosTech/LiveReview/discussions/9" target="_blank" rel="noopener noreferrer"><Button variant="outline" size="sm">Vote / Request Provider</Button></a>}
            />

            {/* High-visibility banner for experimental provider selection */}
            {selectedProvider !== 'all' && (() => {
                const meta = popularAIProviders.find(p => p.id === selectedProvider);
                if (!meta || meta.supportLevel !== 'experimental') return null;
                return (
                    <div className="mb-6">
                        <Alert variant="warning" icon={<Icons.Warning />}> 
                            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                                <div className="text-sm md:pr-4">
                                    <strong>{meta.name}</strong> is currently <strong>Experimental</strong>. Vote to accelerate full support (advanced settings, performance tuning, model coverage).
                                </div>
                                <a
                                    href="https://github.com/HexmosTech/LiveReview/discussions/9"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="inline-flex"
                                >
                                    <Button variant="primary" size="sm" className="whitespace-nowrap">
                                        <Icons.AI />
                                        <span className="ml-1">Add Your Vote</span>
                                    </Button>
                                </a>
                            </div>
                        </Alert>
                    </div>
                );
            })()}

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                {/* Left panel for selecting providers */}
                <div className="lg:col-span-1">
                    <Card title="AI Providers">
                        <ProvidersList 
                            providers={popularAIProviders}
                            selectedProvider={selectedProvider}
                            connectorCounts={connectorCounts}
                            onSelectProvider={handleSelectProvider}
                            totalConnectors={connectors.length}
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
                            connectors={connectors}
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
