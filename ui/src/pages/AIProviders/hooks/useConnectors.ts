import { useState, useEffect, useCallback } from 'react';
import { AIConnector } from '../types';
import { 
    getAIConnectors, 
    createAIConnector, 
    validateAIProviderKey 
} from '../../../api/connectors';

interface UseConnectorsResult {
    connectors: AIConnector[];
    isLoading: boolean;
    error: string | null;
    fetchConnectors: () => Promise<void>;
    saveConnector: (
        providerId: string,
        apiKey: string,
        name: string,
        existingConnector?: AIConnector | null
    ) => Promise<boolean>;
    setError: (error: string | null) => void;
}

export const useConnectors = (): UseConnectorsResult => {
    const [connectors, setConnectors] = useState<AIConnector[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    
    const fetchConnectors = useCallback(async () => {
        setIsLoading(true);
        try {
            const data = await getAIConnectors();
            
            // Transform the API response to match our AIConnector type
            const transformedConnectors: AIConnector[] = data.map(connector => ({
                id: connector.id?.toString() || Math.random().toString(36).substring(2, 9),
                name: connector.connector_name || connector.provider_name || 'Unnamed Connector',
                providerName: connector.provider_name || '',
                apiKey: connector.api_key_preview || '',
                displayOrder: connector.display_order || 0,
                createdAt: connector.created_at ? new Date(connector.created_at) : new Date(),
                lastUsed: connector.last_used ? new Date(connector.last_used) : undefined,
                usageStats: {
                    totalCalls: connector.total_calls || 0,
                    successfulCalls: connector.successful_calls || 0,
                    failedCalls: connector.failed_calls || 0,
                    averageLatency: connector.average_latency || 0
                },
                models: connector.models || [],
                selectedModel: connector.selected_model || '',
                isActive: connector.is_active !== false // Default to true if not specified
            }));
            
            setConnectors(transformedConnectors);
        } catch (error) {
            console.error('Error fetching AI connectors:', error);
            setError('Failed to load AI connectors');
        } finally {
            setIsLoading(false);
        }
    }, []);
    
    useEffect(() => {
        fetchConnectors();
    }, [fetchConnectors]);
    
    const saveConnector = async (
        providerId: string,
        apiKey: string,
        name: string,
        existingConnector?: AIConnector | null
    ): Promise<boolean> => {
        try {
            setIsLoading(true);
            
            // First validate the API key
            try {
                const validationResult = await validateAIProviderKey(providerId, apiKey);
                
                if (!validationResult.valid) {
                    setError(`API key validation failed: ${validationResult.message}`);
                    return false;
                }
            } catch (validationError) {
                console.error('Error validating API key:', validationError);
                setError('Failed to validate API key. Please try again.');
                return false;
            }
            
            // Now save the connector to the database
            try {
                const displayOrder = connectors.filter(c => c.providerName === providerId).length;
                
                if (existingConnector) {
                    // Update existing connector (not implemented yet in the backend)
                    // For now, just update the UI
                    const updatedConnectors = connectors.map(c => 
                        c.id === existingConnector.id 
                            ? { 
                                ...c, 
                                name: name || c.name,
                                apiKey: apiKey || c.apiKey 
                            } 
                            : c
                    );
                    setConnectors(updatedConnectors);
                } else {
                    // Create new connector in the backend
                    const result = await createAIConnector(
                        providerId,
                        apiKey,
                        name,
                        displayOrder
                    );
                    
                    console.log('Connector created:', result);
                    
                    // After creating, refresh the connector list
                    await fetchConnectors();
                }
                
                return true;
            } catch (saveError) {
                console.error('Error saving connector to database:', saveError);
                setError('Failed to save connector to database. Please try again.');
                return false;
            }
        } catch (error) {
            console.error('Error saving connector:', error);
            setError('Failed to save connector. Please try again.');
            return false;
        } finally {
            setIsLoading(false);
        }
    };
    
    return {
        connectors,
        isLoading,
        error,
        fetchConnectors,
        saveConnector,
        setError
    };
};
