import React from 'react';
import { ChatConversation } from './ChatConversation';

// Thin wrapper - all rendering/send/loading logic lives in ChatConversation
// (shared with ChatDebugPage) so the two surfaces can never drift. See the
// "Chat UI" rule in AGENTS.md.
const Chatbot: React.FC = () => <ChatConversation surface="chat" />;

export default Chatbot;
