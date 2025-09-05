import apiClient from '../api/apiClient';
import type {
  CatalogResponse,
  VariablesResponse,
  CreateChunkRequest,
  CreateChunkResponse,
  ReorderRequest,
  RenderPreviewResponse,
} from '../types/prompts';

export const promptsService = {
  getCatalog: async (): Promise<CatalogResponse> => {
    return apiClient.get<CatalogResponse>('/prompts/catalog');
  },

  getVariables: async (promptKey: string, params?: {
    ai_connector_id?: number;
    integration_token_id?: number;
    repository?: string;
  }): Promise<VariablesResponse> => {
    const search = new URLSearchParams();
    if (params?.ai_connector_id) search.set('ai_connector_id', String(params.ai_connector_id));
    if (params?.integration_token_id) search.set('integration_token_id', String(params.integration_token_id));
    if (params?.repository) search.set('repository', params.repository);
    const qs = search.toString();
    const path = qs ? `/prompts/${encodeURIComponent(promptKey)}/variables?${qs}` : `/prompts/${encodeURIComponent(promptKey)}/variables`;
    return apiClient.get<VariablesResponse>(path);
  },

  createChunk: async (
    promptKey: string,
    variable: string,
    payload: CreateChunkRequest
  ): Promise<CreateChunkResponse> => {
    return apiClient.post<CreateChunkResponse>(
      `/prompts/${encodeURIComponent(promptKey)}/variables/${encodeURIComponent(variable)}/chunks`,
      payload
    );
  },

  reorderChunks: async (
    promptKey: string,
    variable: string,
    payload: ReorderRequest
  ): Promise<{ status: string }> => {
    return apiClient.post<{ status: string }>(
      `/prompts/${encodeURIComponent(promptKey)}/variables/${encodeURIComponent(variable)}/reorder`,
      payload
    );
  },

  renderPreview: async (
    promptKey: string,
    params?: {
      ai_connector_id?: number;
      integration_token_id?: number;
      repository?: string;
    }
  ): Promise<RenderPreviewResponse> => {
    const search = new URLSearchParams();
    if (params?.ai_connector_id) search.set('ai_connector_id', String(params.ai_connector_id));
    if (params?.integration_token_id) search.set('integration_token_id', String(params.integration_token_id));
    if (params?.repository) search.set('repository', params.repository);
    const qs = search.toString();
    const path = qs ? `/prompts/${encodeURIComponent(promptKey)}/render?${qs}` : `/prompts/${encodeURIComponent(promptKey)}/render`;
    return apiClient.get<RenderPreviewResponse>(path);
  },
};

export default promptsService;
