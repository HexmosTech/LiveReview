import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LuSearch, LuPlus, LuTrash2, LuMessageSquare, LuPin, LuPinOff } from 'react-icons/lu';
import { EmptyState, Tooltip } from '../../components/UIPrimitives';
import { deleteConversation, listConversations, type Conversation } from '../../api/chatConversations';
import {
  SIDEBAR_COLLAPSED_WIDTH,
  setChatSidebarHoverExpanded,
  setChatSidebarResizing,
  setChatSidebarWidth,
  toggleChatSidebarPinned,
  useChatSidebarHoverExpanded,
  useChatSidebarPinned,
  useChatSidebarResizing,
  useChatSidebarWidth,
} from '../../store/chatSidebar';

// How long to wait after the last keystroke before firing the search
// request, mirroring the Reviews search box's debounce.
const SEARCH_DEBOUNCE_MS = 300;

export const CONVERSATIONS_QUERY_KEY = ['chat', 'conversations'] as const;

function formatUpdatedAt(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const sameDay = date.toDateString() === now.toDateString();
  if (sameDay) {
    return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

// Drag-to-resize handle on the sidebar's right edge. Reads the starting
// width once on mousedown and applies the delta on every mousemove, rather
// than reading the (stale, closure-captured) width prop on each move.
const ResizeHandle: React.FC<{ currentWidth: number }> = ({ currentWidth }) => {
  const startRef = useRef<{ x: number; width: number } | null>(null);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startRef.current = { x: e.clientX, width: currentWidth };
      setChatSidebarResizing(true);

      const onMove = (moveEvent: MouseEvent) => {
        if (!startRef.current) return;
        const delta = moveEvent.clientX - startRef.current.x;
        setChatSidebarWidth(startRef.current.width + delta);
      };
      const onUp = () => {
        startRef.current = null;
        setChatSidebarResizing(false);
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
      };
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
    },
    [currentWidth],
  );

  return (
    <div
      onMouseDown={handleMouseDown}
      className="absolute right-0 top-0 bottom-0 w-1.5 cursor-col-resize group/handle z-10"
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize sidebar"
    >
      <div className="w-px h-full mx-auto bg-transparent group-hover/handle:bg-blue-500/60 transition-colors" />
    </div>
  );
};

// Unpinned (default): the rail is a compact, auto-hiding strip that peeks
// open on hover - this button pins it so it stays open and reserves layout
// space instead. Pinned: this button releases it back to auto-hide.
const PinButton: React.FC<{ pinned: boolean }> = ({ pinned }) => (
  <Tooltip content={pinned ? 'Unpin sidebar' : 'Pin sidebar open'}>
    <button
      onClick={toggleChatSidebarPinned}
      className={
        pinned
          ? 'p-2 rounded-md text-blue-400 hover:text-blue-300 hover:bg-slate-800 transition-colors'
          : 'p-2 rounded-md text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors'
      }
      aria-label={pinned ? 'Unpin sidebar' : 'Pin sidebar open'}
      aria-pressed={pinned}
    >
      {pinned ? <LuPinOff className="w-4 h-4" /> : <LuPin className="w-4 h-4" />}
    </button>
  </Tooltip>
);

interface SidebarBodyProps {
  width: number;
  showResizeHandle: boolean;
  pinned: boolean;
  search: string;
  searchInput: string;
  onSearchInputChange: (v: string) => void;
  conversations: Conversation[];
  isLoading: boolean;
  activeId: number | undefined;
  onSelect: (id: number) => void;
  onDelete: (id: number) => void;
}

// Shared between the pinned (persistent) state and the hover-peek (while
// unpinned) state, so the two don't drift out of sync.
const SidebarBody: React.FC<SidebarBodyProps> = ({
  width,
  showResizeHandle,
  pinned,
  search,
  searchInput,
  onSearchInputChange,
  conversations,
  isLoading,
  activeId,
  onSelect,
  onDelete,
}) => (
  <div className="h-full flex flex-col relative" style={{ width }}>
    <div className="flex-none flex items-center h-16 px-2.5 border-b border-slate-800/60">
      <PinButton pinned={pinned} />
    </div>
    <div className="flex-none px-2.5 pt-3 pb-2 space-y-2">
      <button
        onClick={() => onSelect(0)}
        className="w-full inline-flex items-center gap-2 px-2.5 py-2 rounded-md border border-slate-700/60 bg-slate-800/60 hover:bg-slate-800 hover:border-slate-600 text-slate-200 text-sm font-medium transition-colors"
      >
        <LuPlus className="w-3.5 h-3.5 text-blue-400" />
        New chat
      </button>
      <div className="relative">
        <LuSearch className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500 pointer-events-none" />
        <input
          type="text"
          value={searchInput}
          onChange={(e) => onSearchInputChange(e.target.value)}
          placeholder="Search conversations"
          aria-label="Search conversations"
          className="w-full rounded-md border border-slate-700/60 bg-slate-800/40 text-slate-200 placeholder-slate-500 text-sm pl-8 pr-2.5 py-1.5 outline-none focus:border-slate-600 focus:bg-slate-800/70 transition-colors"
        />
      </div>
    </div>

    <div className="flex-1 overflow-y-auto px-1.5 pb-3 space-y-0.5">
      {!isLoading && conversations.length === 0 && (
        <EmptyState
          icon={<LuMessageSquare className="w-6 h-6" />}
          title={search ? 'No matches' : 'No conversations yet'}
          description={search ? 'Try a different search term.' : 'Start a new chat to see it here.'}
          className="py-6 px-2 text-sm"
        />
      )}
      {conversations.map((conv) => {
        const isActive = conv.id === activeId;
        return (
          <div
            key={conv.id}
            className={`group relative rounded-md ${isActive ? 'bg-slate-800/80' : 'hover:bg-slate-800/50'}`}
          >
            <button onClick={() => onSelect(conv.id)} className="w-full text-left px-2.5 py-2 pr-8 rounded-md">
              <div className={`text-sm truncate ${isActive ? 'text-slate-100 font-medium' : 'text-slate-300'}`}>
                {conv.title || 'New conversation'}
              </div>
              {conv.snippet ? (
                <div className="text-xs text-slate-500 truncate mt-0.5">{conv.snippet}</div>
              ) : (
                <div className="text-xs text-slate-500 mt-0.5">{formatUpdatedAt(conv.updatedAt)}</div>
              )}
            </button>
            <Tooltip content="Delete conversation">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (window.confirm('Delete this conversation?')) onDelete(conv.id);
                }}
                className="absolute right-1 top-1/2 -translate-y-1/2 p-1 rounded text-slate-500 hover:text-red-400 hover:bg-slate-700 opacity-0 group-hover:opacity-100 transition-opacity"
                aria-label="Delete conversation"
              >
                <LuTrash2 className="w-3.5 h-3.5" />
              </button>
            </Tooltip>
          </div>
        );
      })}
    </div>
    {showResizeHandle && <ResizeHandle currentWidth={width} />}
  </div>
);

export const ConversationSidebar: React.FC = () => {
  const navigate = useNavigate();
  const { conversationId } = useParams<{ conversationId?: string }>();
  const queryClient = useQueryClient();
  const pinned = useChatSidebarPinned();
  const width = useChatSidebarWidth();
  const resizing = useChatSidebarResizing();
  const hoverExpanded = useChatSidebarHoverExpanded();

  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => setSearch(searchInput.trim()), SEARCH_DEBOUNCE_MS);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [searchInput]);

  const showFull = pinned || hoverExpanded;

  const { data: conversations = [], isLoading } = useQuery({
    queryKey: [...CONVERSATIONS_QUERY_KEY, search],
    queryFn: () => listConversations({ search: search || undefined }),
    enabled: showFull,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteConversation(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
      if (String(id) === conversationId) navigate('/chat');
    },
  });

  const activeId = conversationId ? Number(conversationId) : undefined;
  const select = (id: number) => navigate(id ? `/chat/${id}` : '/chat');

  // Hover-to-peek only makes sense while unpinned - once pinned it's
  // already fully open, and mid-drag-resize a hover-driven state flip
  // would just be confusing.
  const hoverHandlers =
    !pinned && !resizing
      ? {
          onMouseEnter: () => setChatSidebarHoverExpanded(true),
          onMouseLeave: () => setChatSidebarHoverExpanded(false),
        }
      : undefined;

  if (!pinned && !hoverExpanded) {
    // Compact rail: permanently reserves SIDEBAR_COLLAPSED_WIDTH of layout
    // space (see ChatLayout/Navbar), nothing more - hovering it peeks the
    // full sidebar open as an overlay (below) without changing that.
    return (
      <div
        {...hoverHandlers}
        className="fixed left-0 top-0 bottom-0 z-30 flex flex-col items-center border-r border-slate-700/60 bg-slate-900/95 backdrop-blur-sm"
        style={{ width: SIDEBAR_COLLAPSED_WIDTH }}
      >
        <div className="flex-none flex items-center justify-center h-16 border-b border-slate-800/60 w-full">
          <PinButton pinned={false} />
        </div>
        <div className="pt-3">
          <Tooltip content="New chat">
            <button
              onClick={() => navigate('/chat')}
              className="p-2 rounded-md border border-slate-700/60 bg-slate-800/60 hover:bg-slate-800 hover:border-slate-600 text-blue-400 transition-colors"
              aria-label="New chat"
            >
              <LuPlus className="w-4 h-4" />
            </button>
          </Tooltip>
        </div>
      </div>
    );
  }

  // Either pinned (persistent, reserves `width` of layout space) or
  // hover-peeking while unpinned (overlay - ChatLayout/Navbar still only
  // reserve SIDEBAR_COLLAPSED_WIDTH, so this visually sits on top of the
  // content, not pushing it).
  return (
    <div
      {...hoverHandlers}
      className={
        resizing
          ? 'fixed left-0 top-0 bottom-0 z-30 overflow-hidden border-r border-slate-700/60 bg-slate-900/95 backdrop-blur-sm'
          : 'fixed left-0 top-0 bottom-0 z-30 overflow-hidden border-r border-slate-700/60 bg-slate-900/95 backdrop-blur-sm transition-[width] duration-200 ease-in-out'
      }
      style={{ width }}
    >
      <SidebarBody
        width={width}
        showResizeHandle={pinned}
        pinned={pinned}
        search={search}
        searchInput={searchInput}
        onSearchInputChange={setSearchInput}
        conversations={conversations}
        isLoading={isLoading}
        activeId={activeId}
        onSelect={select}
        onDelete={(id) => deleteMutation.mutate(id)}
      />
    </div>
  );
};
