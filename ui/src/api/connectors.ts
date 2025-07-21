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
