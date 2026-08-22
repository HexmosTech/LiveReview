import apiClient from './apiClient';

export interface ChatMessage {
  role: 'user' | 'assistant';
  text: string;
}

// A chart report the backend hands back as a raw Vega-Lite spec rather than a
// rendered image, so the frontend can render it interactively (tooltips,
// hover, legend filtering) instead of a flat PNG. `id` is only present once
// the chart has been persisted (loaded via getConversation) - a chart fresh
// off a live turn doesn't have one yet.
export interface ChatChart {
  id?: number;
  title?: string;
  description?: string;
  query?: string;
  time_range?: string;
  granularity?: string;
  spec: Record<string, unknown>;
}

// A downloadable export (CSV) produced alongside an answer. Unlike charts,
// these are served from an authenticated endpoint scoped to the caller's org,
// so they must be fetched with the usual auth headers rather than by <a href>.
export interface ChatFile {
  url: string;
  filename: string;
  title?: string;
  description?: string;
  query?: string;
  time_range?: string;
  granularity?: string;
  rows?: number;
}

export interface ChatResponse {
  response: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  debug_artifacts?: unknown;
  sessionId?: string;
  conversationId: number;
}

// The backend now owns conversation history: it loads prior turns by
// conversationId and persists this one, so the client only ever sends the
// new message plus which conversation it belongs to (omitted to start a new
// one).
export async function sendChatMessage(
  message: string,
  conversationId?: number,
): Promise<ChatResponse> {
  return apiClient.post<ChatResponse>('/api/v1/chat/send', {
    message,
    conversationId: conversationId ?? undefined,
  });
}
