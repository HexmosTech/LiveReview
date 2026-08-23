import React from 'react';
import { ChatConversation } from './ChatConversation';

// Thin wrapper - all rendering/send/loading logic lives in ChatConversation
// (shared with Chatbot.tsx) so the two surfaces can never drift. The only
// thing this surface adds is the debug-artifacts button/dialog, gated
// inside ChatConversation by surface === 'chat_debug'. See the "Chat UI"
// rule in AGENTS.md.
const ChatDebugPage: React.FC = () => <ChatConversation surface="chat_debug" />;

export default ChatDebugPage;
