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
}

export interface ChatResponse {
  response: string;
  history: ChatHistoryEntry[];
  images?: ChatImage[];
}

export async function sendChatMessage(
  message: string,
  history: ChatHistoryEntry[],
): Promise<ChatResponse> {
  return apiClient.post<ChatResponse>('/api/v1/chat/send', {
    message,
    history: history.length > 0 ? history : undefined,
  });
}
