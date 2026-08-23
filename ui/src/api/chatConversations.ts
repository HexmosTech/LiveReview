import apiClient, { BASE_URL } from './apiClient';
import { ChatChart, ChatFile } from './chatbot';

// Which UI surface a conversation was started from — /chat and /chat-debug
// share the same backend table, so this is what keeps their sidebar lists
// separate.
export type ChatSurface = 'chat' | 'chat_debug';

export interface Conversation {
  id: number;
  title: string;
  updatedAt: string;
  snippet?: string;
}

export interface ConversationMessage {
  id: number;
  role: 'user' | 'assistant';
  content: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  debug_artifacts?: unknown;
}

export interface ConversationDetail {
  id: number;
  title: string;
  updatedAt: string;
  messages: ConversationMessage[];
}

// Creates an empty conversation (titled from the first message) up front, so
// the sidebar can show it immediately - the caller sends the actual message
// afterwards via sendChatMessage(text, conversationId).
export function createConversation(firstMessage: string, surface: ChatSurface = 'chat'): Promise<Conversation> {
  return apiClient.post<Conversation>('/api/v1/chat', { message: firstMessage, surface });
}

export function listConversations(
  params: { search?: string; limit?: number; offset?: number; surface?: ChatSurface } = {},
): Promise<Conversation[]> {
  const qs = new URLSearchParams();
  if (params.search) qs.set('search', params.search);
  if (params.limit) qs.set('limit', String(params.limit));
  if (params.offset) qs.set('offset', String(params.offset));
  qs.set('surface', params.surface ?? 'chat');
  return apiClient.get<Conversation[]>(`/api/v1/chat?${qs.toString()}`);
}

export function getConversation(id: number): Promise<ConversationDetail> {
  return apiClient.get<ConversationDetail>(`/api/v1/chat/${id}`);
}

export function renameConversation(id: number, title: string): Promise<void> {
  return apiClient.patch<void>(`/api/v1/chat/${id}`, { title });
}

export function deleteConversation(id: number): Promise<void> {
  return apiClient.delete<void>(`/api/v1/chat/${id}`);
}

// Returns a same-origin URL for re-rendering a persisted chart in a given
// format, e.g. for an <img src> or a fetch-then-download. Only relative,
// same-origin URLs are ever constructed here, matching the existing
// resolveImageUrl convention in Chatbot.tsx for chat file downloads.
export function chartRenderUrl(chartId: number, format: 'png' | 'json' = 'png'): string {
  return `${BASE_URL}/api/v1/chat/charts/${chartId}/render?format=${format}`;
}
