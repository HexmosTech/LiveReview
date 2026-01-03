import { AIConnector, ValidationResult } from '../types';
import { getAIConnectors as fetchAIConnectorsAPI, validateAIProviderKey as validateAPIKey, createAIConnector as createConnectorAPI } from '../../../api/connectors';

/**
 * Fetch all AI connectors from the API
 */
export const fetchAIConnectors = async (): Promise<AIConnector[]> => {
    try {
        console.log('Fetching AI connectors...');
        const aiConnectorsData = await fetchAIConnectorsAPI();
        console.log('AI Connectors data received:', aiConnectorsData);
        
        // Always return an array, even if empty
        if (!aiConnectorsData || !Array.isArray(aiConnectorsData)) {
            console.log('No AI connectors data or invalid format, returning empty array');
            return [];
        }
        
        // Map the API response to the component's format
        const aiConnectors = aiConnectorsData.map(connector => ({
            id: connector.id?.toString() || Math.random().toString(36).substring(2, 9),
            name: connector.connector_name || connector.provider_name || 'Unnamed Connector',
            providerName: connector.provider_name || '',
            apiKey: connector.api_key_preview || '',
            fullApiKey: connector.api_key || '', // Store full API key for editing
            baseURL: connector.base_url || '',
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
        
        console.log('Transformed AI connectors:', aiConnectors);
        return aiConnectors;
    } catch (err) {
        console.error('Error fetching AI connectors:', err);
        // Don't throw error, return empty array to show empty state instead of error
        return [];
    }
};

/**
 * Validate an AI provider API key
 */
export const validateAIProviderKey = async (
    providerId: string, 
    apiKey: string,
    model?: string
): Promise<ValidationResult> => {
    try {
        console.log(`Validating API key for provider: ${providerId}`);
		const result = await validateAPIKey(providerId, apiKey, undefined, model);
        console.log('Validation result:', result);
        return result;
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
    displayOrder: number,
    baseURL?: string,
    selectedModel?: string
) => {
    try {
        console.log(`Creating AI connector: ${name} for provider: ${providerId}`);
        const result = await createConnectorAPI(providerId, apiKey, name, displayOrder, baseURL, selectedModel);
        console.log('Connector created successfully:', result);
        return result;
    } catch (error) {
        console.error('Error creating AI connector:', error);
        throw error; // Re-throw to let the caller handle it
    }
};


