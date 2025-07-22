import { AIConnector, ValidationResult } from '../types';

/**
 * Fetch all AI connectors from the API
 */
export const fetchAIConnectors = async (): Promise<AIConnector[]> => {
    try {
        // Try to get AI connectors from the new endpoint
        try {
            const aiConnectorsData = await getAIConnectors();
            console.log('AI Connectors data:', aiConnectorsData);
            
            if (aiConnectorsData && Array.isArray(aiConnectorsData) && aiConnectorsData.length > 0) {
                // Map the new format to the component's format
                const aiConnectors = aiConnectorsData.map(connector => ({
                    id: connector.id.toString(),
                    name: connector.connector_name,
                    providerName: connector.provider_name,
                    apiKey: connector.api_key_preview || '',
                    displayOrder: connector.display_order || 0,
                    createdAt: new Date(connector.created_at),
                    lastUsed: undefined as Date | undefined,
                    usageStats: {
                        totalCalls: 0,
                        successfulCalls: 0,
                        failedCalls: 0,
                        averageLatency: 0
                    },
                    models: [] as string[],
                    selectedModel: '',
                    isActive: true
                }));
                
                return aiConnectors;
            }
        } catch (aiErr) {
            console.warn('Error fetching from AI connectors endpoint:', aiErr);
            // Fall back to legacy method
        }
        
        // Fallback to original method
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
                lastUsed: connector.metadata?.last_used ? new Date(connector.metadata.last_used) : undefined as Date | undefined,
                usageStats: connector.metadata?.usage_stats || {
                    totalCalls: 0,
                    successfulCalls: 0,
                    failedCalls: 0,
                    averageLatency: 0
                },
                models: connector.metadata?.models || [] as string[],
                selectedModel: connector.metadata?.selected_model,
                isActive: connector.metadata?.is_active !== false // Default to true if not specified
            }));
            
        return aiConnectors;
    } catch (err) {
        console.error('Error fetching connectors:', err);
        throw new Error('Failed to load AI connectors. Please try again.');
    }
};

/**
 * Validate an AI provider API key
 */
export const validateAIProviderKey = async (
    providerId: string, 
    apiKey: string
): Promise<ValidationResult> => {
    // This function would interact with your API to validate the key
    // For now, we'll assume it returns a Promise with validation result
    try {
        const response = await fetch('/api/ai/validate-key', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                provider: providerId,
                apiKey
            }),
        });
        
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('Error validating API key:', error);
        return { valid: false, message: 'Failed to validate API key' };
    }
};

/**
 * Create a new AI connector
 */
export const createAIConnector = async (
    providerId: string,
    apiKey: string,
    name: string,
    displayOrder: number
) => {
    // This function would interact with your API to create a connector
    // For now, we'll assume it returns a Promise with the created connector
    try {
        const response = await fetch('/api/ai/connectors', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                provider_name: providerId,
                api_key: apiKey,
                connector_name: name,
                display_order: displayOrder
            }),
        });
        
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('Error creating AI connector:', error);
        throw new Error('Failed to create AI connector');
    }
};

// These functions would be imported from your existing API modules
// For now, we'll just declare them as placeholders
declare function getAIConnectors(): Promise<any[]>;
declare function getConnectors(): Promise<any[]>;

// This would be imported from your constants or context
declare const popularAIProviders: any[];
