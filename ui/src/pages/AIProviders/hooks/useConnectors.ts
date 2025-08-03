import { useState, useEffect, useCallback } from 'react';
import { AIConnector } from '../types';
import { 
    createAIConnector, 
    deleteAIConnector
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
        existingConnector?: AIConnector | null
    ) => Promise<boolean>;
    deleteConnector: (connectorId: string) => Promise<boolean>;
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
    
    return {
        connectors,
        isLoading,
        error,
        fetchConnectors,
        saveConnector,
        deleteConnector,
        setError
    };
};
