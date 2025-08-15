import { useState, useEffect, useCallback } from 'react';
import { AIConnector } from '../types';
import { 
    createAIConnector, 
    updateAIConnector,
    deleteAIConnector,
    reorderAIConnectors
} from '../../../api/connectors';
import { 
    fetchAIConnectors,
    validateAIProviderKey
} from '../utils/apiUtils';

interface UseConnectorsResult {
    connectors: AIConnector[];
    isLoading: boolean;
    error: string | null;
    fetchConnectors: () => Promise<void>;
    saveConnector: (
        providerId: string,
        apiKey: string,
        name: string,
        existingConnector?: AIConnector | null,
        baseURL?: string,
        selectedModel?: string
    ) => Promise<boolean>;
    deleteConnector: (connectorId: string) => Promise<boolean>;
    reorderConnectors: (newOrder: AIConnector[]) => Promise<boolean>;
    setError: (error: string | null) => void;
}

export const useConnectors = (): UseConnectorsResult => {
    const [connectors, setConnectors] = useState<AIConnector[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    
    const fetchConnectors = useCallback(async () => {
        setIsLoading(true);
        setError(null);
        try {
            console.log('useConnectors: Starting to fetch connectors');
            const transformedConnectors = await fetchAIConnectors();
            console.log('useConnectors: Received transformed connectors:', transformedConnectors);
            setConnectors(transformedConnectors);
        } catch (error) {
            console.error('useConnectors: Error fetching AI connectors:', error);
            setError('Failed to load AI connectors');
            // Set empty array to show empty state instead of error state
            setConnectors([]);
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
        existingConnector?: AIConnector | null,
        baseURL?: string,
        selectedModel?: string
    ): Promise<boolean> => {
        try {
            setIsLoading(true);
            
            // For Ollama, skip validation since we've already validated by fetching models
            if (providerId !== 'ollama') {
                // First validate the API key for non-Ollama providers
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
            }
            
            // Now save the connector to the database
            try {
                const displayOrder = connectors.filter(c => c.providerName === providerId).length;
                
                if (existingConnector) {
                    // Update existing connector
                    const result = await updateAIConnector(
                        existingConnector.id,
                        providerId,
                        apiKey,
                        name,
                        existingConnector.displayOrder,
                        baseURL,
                        selectedModel
                    );
                    console.log('Connector updated:', result);
                    
                    // After updating, refresh the connector list
                    await fetchConnectors();
                } else {
                // Create new connector in the backend
                const result = await createAIConnector(
                    providerId,
                    apiKey,
                    name,
                    displayOrder,
                    baseURL,
                    selectedModel
                );                    console.log('Connector created:', result);
                    
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
    
    const deleteConnector = async (connectorId: string): Promise<boolean> => {
        try {
            setIsLoading(true);
            
            // Delete the connector via API
            await deleteAIConnector(connectorId);
            
            // Update the UI by removing the deleted connector
            setConnectors(prevConnectors => 
                prevConnectors.filter(connector => connector.id !== connectorId)
            );
            
            return true;
        } catch (error) {
            console.error('Error deleting connector:', error);
            setError('Failed to delete connector. Please try again.');
            return false;
        } finally {
            setIsLoading(false);
        }
    };

    const reorderConnectors = async (newOrder: AIConnector[]): Promise<boolean> => {
        try {
            setIsLoading(true);
            
            // Create updates array with new display orders
            const updates = newOrder.map((connector, index) => ({
                id: connector.id,
                display_order: index + 1 // 1-based ordering
            }));
            
            // Call API to update display orders
            await reorderAIConnectors(updates);
            
            // Update the connectors with their new display orders
            const updatedConnectors = newOrder.map((connector, index) => ({
                ...connector,
                displayOrder: index + 1
            }));
            
            // Update local state with the corrected display orders
            setConnectors(updatedConnectors);
            
            return true;
        } catch (error) {
            console.error('Error reordering connectors:', error);
            setError('Failed to reorder connectors. Please try again.');
            // Refresh to get the correct order from server
            await fetchConnectors();
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
        deleteConnector,
        reorderConnectors,
        setError
    };
};
