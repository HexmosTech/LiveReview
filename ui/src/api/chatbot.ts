import apiClient from './apiClient';

export interface ChatMessage {
  role: 'user' | 'assistant';
  text: string;
}

export interface ChatHistoryEntry {
  role: string;
  content?: string;
  text?: string;
}

// A chart report the backend hands back as a raw Vega-Lite spec rather than a
// rendered image, so the frontend can render it interactively (tooltips,
// hover, legend filtering) instead of a flat PNG.
export interface ChatChart {
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
  history: ChatHistoryEntry[];
  charts?: ChatChart[];
  files?: ChatFile[];
  sessionId?: string;
}

export async function sendChatMessage(
  message: string,
  history: ChatHistoryEntry[],
  sessionId?: string,
): Promise<ChatResponse> {
  return apiClient.post<ChatResponse>('/api/v1/chat/send', {
    message,
    history: history.length > 0 ? history : undefined,
    // Echoing the session id back lets every turn of one conversation share a
    // correlation id in the server-side debug log.
    sessionId: sessionId || undefined,
  });
}
