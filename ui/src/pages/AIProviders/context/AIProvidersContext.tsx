import React, { createContext, useContext, ReactNode } from 'react';
import { AIProvider, AIConnector } from '../types';
import { useProviderSelection, useConnectors, useFormState } from '../hooks';

// Define the context type
interface AIProvidersContextType {
    // Provider selection
    selectedProvider: string;
    setSelectedProvider: (provider: string) => void;
    updateUrlFragment: (provider: string, action?: string | null, connectorId?: string | null) => void;
    isEditing: boolean;
    setIsEditing: (value: boolean) => void;
    
    // Connectors
    connectors: AIConnector[];
    isLoading: boolean;
    error: string | null;
    setError: (error: string | null) => void;
    fetchConnectors: () => Promise<void>;
    saveConnector: (providerId: string, apiKey: string, name: string, existingConnector?: AIConnector | null) => Promise<boolean>;
    
    // Form state
    formData: {
        name: string;
        apiKey: string;
        providerType: string;
    };
    selectedConnector: AIConnector | null;
    setSelectedConnector: (connector: AIConnector | null) => void;
    handleInputChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
    handleProviderTypeChange: (providerType: string) => void;
    resetForm: () => void;
    setFormData: (data: { name: string; apiKey: string; providerType: string; }) => void;
    generateFriendlyName: (providerId: string) => void;
    
    // Providers data
    providers: AIProvider[];
}

// Create the context with a default undefined value
const AIProvidersContext = createContext<AIProvidersContextType | undefined>(undefined);

// Provider component
interface AIProvidersProviderProps {
    children: ReactNode;
    providers: AIProvider[];
}

export const AIProvidersProvider: React.FC<AIProvidersProviderProps> = ({ children, providers }) => {
    // Use our custom hooks
    const providerSelection = useProviderSelection(providers);
    const connectorsData = useConnectors();
    const formState = useFormState();
    
    // Create the context value object
    const contextValue: AIProvidersContextType = {
        ...providerSelection,
        ...connectorsData,
        ...formState,
        providers,
        handleProviderTypeChange: (providerType: string) => {
            formState.handleProviderTypeChange(providerType, providers);
        },
        generateFriendlyName: (providerId: string) => {
            formState.generateFriendlyName(providerId, providers);
        }
    };
    
    return (
        <AIProvidersContext.Provider value={contextValue}>
            {children}
        </AIProvidersContext.Provider>
    );
};

// Custom hook to use the context
export const useAIProviders = (): AIProvidersContextType => {
    const context = useContext(AIProvidersContext);
    if (context === undefined) {
        throw new Error('useAIProviders must be used within an AIProvidersProvider');
    }
    return context;
};
