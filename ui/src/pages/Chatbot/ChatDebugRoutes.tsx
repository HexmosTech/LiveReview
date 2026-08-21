import React from 'react';
import { Routes, Route, useParams } from 'react-router-dom';
import { ChatLayout } from './ChatLayout';
import ChatDebugPage from './ChatDebugPage';

const ChatDebugForRoute: React.FC = () => {
  const { conversationId } = useParams<{ conversationId?: string }>();
  return <ChatDebugPage key={conversationId ?? 'new'} />;
};

const ChatDebugRoutes: React.FC = () => {
  return (
    <Routes>
      <Route element={<ChatLayout />}>
        <Route index element={<ChatDebugForRoute />} />
        <Route path=":conversationId" element={<ChatDebugForRoute />} />
      </Route>
    </Routes>
  );
};

export default ChatDebugRoutes;
