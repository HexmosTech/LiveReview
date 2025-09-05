// Types for Prompts Admin UI

export type CatalogEntry = {
  prompt_key: string;
  provider: string;
  build_id: string;
  variables: string[];
};

export type CatalogResponse = {
  catalog: CatalogEntry[];
};

export type Chunk = {
  id: number;
  type: 'user' | 'system';
  title: string;
  body: string;
  sequence_index: number;
  enabled: boolean;
  allow_markdown: boolean;
  redact_on_log: boolean;
  created_by?: number | null;
  updated_by?: number | null;
};

export type VariableEntry = {
  name: string;
  chunks: Chunk[];
};

export type VariablesResponse = {
  prompt_key: string;
  provider: string;
  variables: VariableEntry[];
};

export type CreateChunkRequest = {
  type?: 'user' | 'system';
  title: string;
  body: string;
  enabled?: boolean;
  allow_markdown?: boolean;
  redact_on_log?: boolean;
  sequence_index?: number;
  ai_connector_id?: number;
  integration_token_id?: number;
  repository?: string;
};

export type CreateChunkResponse = {
  chunk: Chunk;
};

export type ReorderRequest = {
  ordered_ids: number[];
  ai_connector_id?: number;
  integration_token_id?: number;
  repository?: string;
};

export type RenderPreviewResponse = {
  prompt: string;
  build_id: string;
  provider: string;
};
