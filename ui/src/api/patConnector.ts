import apiClient from './apiClient';

export interface PATConnectorRequest {
  name: string; // connector_name
  type: string;
  url: string;
  pat_token: string;
  metadata?: any;
}

export interface PATConnectorResponse {
  id: number;
  provider: string;
  connection_name: string;
  provider_url?: string;
  metadata: any;
  created_at: string;
  updated_at: string;
}

export const createPATConnector = async (data: PATConnectorRequest): Promise<PATConnectorResponse> => {
  // Ensure connector_name is sent as 'name' in payload
  const payload = {
    name: data.name,
    type: data.type,
    url: data.url,
    pat_token: data.pat_token,
    metadata: data.metadata,
  };
  try {
    return await apiClient.post<PATConnectorResponse>('/api/v1/integration_tokens/pat', payload);
  } catch (error) {
    console.error('Error creating PAT connector:', error);
    throw error;
  }
};
