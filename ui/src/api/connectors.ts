import apiClient from './apiClient';

export interface ConnectorResponse {
  id: number;
  provider: string;
  provider_app_id: string;
  connection_name: string;
  provider_url?: string;
  metadata: any;
  created_at: string;
  updated_at: string;
}

/**
 * Fetch all connectors/integrations from the server
 * @returns Promise with array of connectors
 */
export const getConnectors = async (): Promise<ConnectorResponse[]> => {
  try {
    return await apiClient.get<ConnectorResponse[]>('/api/v1/connectors');
  } catch (error) {
    console.error('Error fetching connectors:', error);
    throw error;
  }
};

/**
 * Validate an AI provider API key
 * @param provider The provider name (e.g., 'openai', 'gemini', 'claude')
 * @param apiKey The API key to validate
 * @param baseURL Optional base URL for the API (for custom endpoints)
 * @returns Promise with validation result
 */
export const validateAIProviderKey = async (
  provider: string,
  apiKey: string,
  baseURL?: string
): Promise<{ valid: boolean; message: string }> => {
  try {
    const response = await apiClient.post<{ valid: boolean; message: string }>(
      '/api/v1/aiconnectors/validate-key',
      {
        provider,
        api_key: apiKey,
        base_url: baseURL,
      }
    );
    return response;
  } catch (error) {
    console.error('Error validating API key:', error);
    throw error;
  }
};
