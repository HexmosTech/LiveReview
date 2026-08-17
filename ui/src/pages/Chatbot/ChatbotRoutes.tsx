import React from 'react';
import { Routes, Route, useParams } from 'react-router-dom';
import { ChatLayout } from './ChatLayout';
import Chatbot from './Chatbot';

// Keying by conversationId forces a clean remount when switching threads (or
// starting a new one) instead of reconciling in place, so each conversation
// starts from a blank local state that then hydrates from the server. See
// Chatbot.tsx's post-send navigation, which relies on this remount to pick
// up the now-persisted turn for a brand new conversation.
const ChatbotForRoute: React.FC = () => {
  const { conversationId } = useParams<{ conversationId?: string }>();
  return <Chatbot key={conversationId ?? 'new'} />;
};

const ChatbotRoutes: React.FC = () => {
  return (
    <Routes>
      <Route element={<ChatLayout />}>
        <Route index element={<ChatbotForRoute />} />
        <Route path=":conversationId" element={<ChatbotForRoute />} />
      </Route>
    </Routes>
  );
};

export default ChatbotRoutes;
