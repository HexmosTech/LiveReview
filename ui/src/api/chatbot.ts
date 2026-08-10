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

export interface ChatImage {
  url: string;
  title?: string;
  description?: string;
  query?: string;
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
  rows?: number;
}

export interface ChatResponse {
  response: string;
  history: ChatHistoryEntry[];
  images?: ChatImage[];
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
