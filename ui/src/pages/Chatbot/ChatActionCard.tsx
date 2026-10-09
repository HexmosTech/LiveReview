import React from 'react';
import { useNavigate } from 'react-router-dom';
import { ChatEntry } from './ChatConversation';

interface ChatActionCardProps {
  msg: ChatEntry;
  idx: number;
  messages: ChatEntry[];
  handleSend: (text: string) => void;
}

const ACTION_RETRY = '#retry';

export function ChatActionCard({ msg, idx, messages, handleSend }: ChatActionCardProps) {
  const navigate = useNavigate();

  if (!msg.actionCard) return null;

  return (
    <div className="mt-4 flex flex-col gap-2 w-full">
      <div className="flex items-center justify-between gap-3 p-3 sm:px-4 bg-slate-800/30 border border-slate-700 rounded-lg w-full">
        <div>
          <h4 className="text-sm font-semibold text-slate-200">{msg.actionCard.title}</h4>
          <p className="text-xs text-slate-400 mt-0.5">{msg.actionCard.description}</p>
        </div>
        <button
          onClick={() => {
            const url = msg.actionCard?.action_url;
            if (url === ACTION_RETRY) {
              // Find the most recent user message before this error
              let prevUserMsg = undefined;
              for (let i = idx - 1; i >= 0; i--) {
                if (messages[i].role === 'user') {
                  prevUserMsg = messages[i];
                  break;
                }
              }
              
              if (prevUserMsg?.text) {
                handleSend(prevUserMsg.text);
              }
            } else if (url && url.startsWith('/')) {
              navigate(url);
            }
          }}
          className="inline-flex items-center gap-1.5 text-sm font-medium px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white transition-colors cursor-pointer whitespace-nowrap flex-shrink-0"
        >
          {msg.actionCard.button_text}
        </button>
      </div>
      {msg.debugArtifacts?.raw_llm_error && (
        <details className="group px-1">
          <summary className="w-fit text-xs text-slate-500 cursor-pointer hover:text-slate-400 select-none">
            Debug logs
          </summary>
          <div className="mt-2 p-2.5 bg-slate-900 rounded border border-slate-700 font-mono text-[10px] sm:text-xs text-red-400 whitespace-pre-wrap overflow-x-auto">
            {msg.debugArtifacts.raw_llm_error}
          </div>
        </details>
      )}
    </div>
  );
}
