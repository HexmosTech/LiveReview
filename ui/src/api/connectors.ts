import apiClient from './apiClient';

// Types for repository access
export interface ProjectWithStatus {
  project_path: string;
  webhook_status: 'unconnected' | 'manual' | 'automatic';
  last_verified?: string;
  webhook_id?: string;
  updated_at: string;
}

export interface RepositoryAccess {
  projects_with_status: ProjectWithStatus[];
  projects?: string[]; // Legacy field, kept for backward compatibility
}

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
 * Fetch a single connector by ID from the server
 * @param connectorId The ID of the connector to fetch
 * @returns Promise with the connector data
 */
export const getConnector = async (connectorId: string): Promise<ConnectorResponse> => {
  try {
    const response = await apiClient.get<ConnectorResponse>(`/api/v1/connectors/${connectorId}`);
    return response;
  } catch (error) {
    console.error('Error fetching connector:', error);
    throw error;
  }
};

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
 * Delete a Git connector
 * @param connectorId The ID of the connector to delete
 * @returns Promise with the deletion result
 */
export const deleteConnector = async (connectorId: string): Promise<any> => {
  try {
    const response = await apiClient.delete(`/api/v1/connectors/${connectorId}`);
    return response;
  } catch (error) {
    console.error('Error deleting Git connector:', error);
    throw error;
  }
};

/**
 * Fetch repository access information for a connector
 * @param connectorId The ID of the connector to get repository access for
 * @param refresh Optional parameter to force refresh the cached data
 * @returns Promise with repository access information
 */
export const getRepositoryAccess = async (connectorId: string, refresh?: boolean): Promise<RepositoryAccess> => {
  try {
    let url = `/api/v1/connectors/${connectorId}/repository-access`;
    if (refresh) {
      url += '?refresh=true';
    }
    const response = await apiClient.get<RepositoryAccess>(url);
    return response;
  } catch (error) {
    console.error('Error fetching repository access:', error);
    throw error;
  }
};

/**
 * Fetch all AI connectors from the server
 * @returns Promise with array of AI connectors
 */
export const getAIConnectors = async (): Promise<any[]> => {
  try {
    return await apiClient.get<any[]>('/api/v1/aiconnectors');
  } catch (error) {
    console.error('Error fetching AI connectors:', error);
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

/**
 * Create a new AI connector
 * @param providerName The provider name (e.g., 'openai', 'gemini', 'claude')
 * @param apiKey The API key for the connector
 * @param connectorName A user-friendly name for this connector
 * @param displayOrder Order to display in the UI (lower numbers first)
 * @param baseURL Optional base URL for the API (for custom endpoints)
 * @returns Promise with the created connector
 */
export const createAIConnector = async (
  providerName: string,
  apiKey: string,
  connectorName: string,
  displayOrder: number = 0,
  baseURL?: string
): Promise<any> => {
  try {
    const response = await apiClient.post(
      '/api/v1/aiconnectors',
      {
        provider_name: providerName,
        api_key: apiKey,
        connector_name: connectorName,
        display_order: displayOrder,
        base_url: baseURL,
      }
    );
    return response;
  } catch (error) {
    console.error('Error creating AI connector:', error);
    throw error;
  }
};

/**
 * Delete an AI connector
 * @param connectorId The ID of the connector to delete
 * @returns Promise with the deletion result
 */
export const deleteAIConnector = async (connectorId: string): Promise<any> => {
  try {
    const response = await apiClient.delete(`/api/v1/aiconnectors/${connectorId}`);
    return response;
  } catch (error) {
    console.error('Error deleting AI connector:', error);
    throw error;
  }
};

/**
 * Enable manual trigger for all projects for a connector
 * @param connectorId The ID of the connector to enable manual trigger for
 * @returns Promise with the result
 */
export const enableManualTriggerForAllProjects = async (connectorId: string): Promise<any> => {
  try {
    const response = await apiClient.post(`/api/v1/connectors/${connectorId}/enable-manual-trigger`, {});
    return response;
  } catch (error) {
    console.error('Error enabling manual trigger for all projects:', error);
    throw error;
  }
};

/**
 * Disable manual trigger for all projects for a connector
 * @param connectorId The ID of the connector to disable manual trigger for
 * @returns Promise with the result
 */
export const disableManualTriggerForAllProjects = async (connectorId: string): Promise<any> => {
  try {
    const response = await apiClient.post(`/api/v1/connectors/${connectorId}/disable-manual-trigger`, {});
    return response;
  } catch (error) {
    console.error('Error disabling manual trigger for all projects:', error);
    throw error;
  }
};
