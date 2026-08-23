import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LuSearch, LuPlus, LuTrash2, LuMessageSquare, LuPin, LuPinOff, LuPencil, LuCheck } from 'react-icons/lu';
import { EmptyState, Tooltip } from '../../components/UIPrimitives';
import { deleteConversation, listConversations, renameConversation, type ChatSurface, type Conversation } from '../../api/chatConversations';
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

// Truncated text that scrolls to reveal the rest on hover, instead of just
// cutting it off with an ellipsis. Only animates when the text actually
// overflows its box - measured via ResizeObserver so it stays correct
// across sidebar resizes and font loads, not just on mount. The scroll
// distance and duration both scale with how much text is hidden, so a
// barely-clipped title doesn't get the same multi-second scroll as a long
// one.
const MarqueeText: React.FC<{ text: string; className?: string }> = ({ text, className }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const textRef = useRef<HTMLSpanElement>(null);
  const [overflow, setOverflow] = useState(0);

  useEffect(() => {
    const container = containerRef.current;
    const el = textRef.current;
    if (!container || !el) return;
    const measure = () => setOverflow(Math.max(0, el.scrollWidth - container.clientWidth));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(container);
    return () => ro.disconnect();
  }, [text]);

  const durationMs = Math.min(8000, Math.max(1500, overflow * 20));

  return (
    <div ref={containerRef} className={`overflow-hidden ${className ?? ''}`}>
      <span
        ref={textRef}
        className="inline-block whitespace-nowrap translate-x-0 ease-linear group-hover:translate-x-[var(--marquee-distance)]"
        style={{
          ['--marquee-distance' as string]: overflow > 0 ? `-${overflow}px` : '0px',
          transitionProperty: 'transform',
          transitionDuration: `${durationMs}ms`,
        }}
      >
        {text}
      </span>
    </div>
  );
};

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
  editingId: number | null;
  editValue: string;
  onStartRename: (conv: Conversation) => void;
  onEditValueChange: (v: string) => void;
  onSaveRename: (id: number) => void;
  onCancelRename: () => void;
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
  editingId,
  editValue,
  onStartRename,
  onEditValueChange,
  onSaveRename,
  onCancelRename,
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
        const isEditing = conv.id === editingId;
        return (
          <div
            key={conv.id}
            className={`group relative rounded-md px-2.5 py-2 ${isActive ? 'bg-slate-800/80' : 'hover:bg-slate-800/50'}`}
          >
            {isEditing ? (
              <input
                autoFocus
                type="text"
                value={editValue}
                onChange={(e) => onEditValueChange(e.target.value)}
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    onSaveRename(conv.id);
                  } else if (e.key === 'Escape') {
                    e.preventDefault();
                    onCancelRename();
                  }
                }}
                className="w-full bg-slate-900 text-slate-100 text-sm rounded border border-slate-600 px-1.5 py-1 outline-none focus:border-blue-500"
              />
            ) : (
              <button onClick={() => onSelect(conv.id)} className="w-full text-left">
                <MarqueeText
                  text={conv.title || 'New conversation'}
                  className={`text-sm ${isActive ? 'text-slate-100 font-medium' : 'text-slate-300'}`}
                />
                {conv.snippet ? (
                  <MarqueeText text={conv.snippet} className="text-xs text-slate-500 mt-0.5" />
                ) : (
                  <div className="text-xs text-slate-500 mt-0.5">{formatUpdatedAt(conv.updatedAt)}</div>
                )}
              </button>
            )}
            {/* Overlaid on hover only, never reserving layout space from the
                title/snippet text above - a background pill keeps the icons
                legible over whatever text ends up underneath them, aligned
                to the timestamp line rather than the title line. */}
            {isEditing ? (
              <Tooltip content="Save">
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onSaveRename(conv.id);
                  }}
                  className="absolute right-1.5 bottom-1.5 p-1 rounded bg-slate-800 text-slate-500 hover:text-emerald-400 hover:bg-slate-700 transition-colors"
                  aria-label="Save name"
                >
                  <LuCheck className="w-3.5 h-3.5" />
                </button>
              </Tooltip>
            ) : (
              <div className="absolute right-1.5 bottom-1.5 flex items-center gap-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto transition-opacity">
                <Tooltip content="Rename conversation">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      onStartRename(conv);
                    }}
                    className="p-1 rounded bg-slate-800 text-slate-500 hover:text-slate-200 hover:bg-slate-700"
                    aria-label="Rename conversation"
                  >
                    <LuPencil className="w-3.5 h-3.5" />
                  </button>
                </Tooltip>
                <Tooltip content="Delete conversation">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      if (window.confirm('Delete this conversation?')) onDelete(conv.id);
                    }}
                    className="p-1 rounded bg-slate-800 text-slate-500 hover:text-red-400 hover:bg-slate-700"
                    aria-label="Delete conversation"
                  >
                    <LuTrash2 className="w-3.5 h-3.5" />
                  </button>
                </Tooltip>
              </div>
            )}
          </div>
        );
      })}
    </div>
    {showResizeHandle && <ResizeHandle currentWidth={width} />}
  </div>
);

export const ConversationSidebar: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { conversationId } = useParams<{ conversationId?: string }>();
  const queryClient = useQueryClient();

  // /chat and /chat-debug share this sidebar component but must show
  // separate conversation lists - derive which one we're in from the route
  // rather than threading a prop through ChatLayout, since both route trees
  // mount the same <ChatLayout><Outlet/></ChatLayout> shell.
  const surface: ChatSurface = location.pathname.startsWith('/chat-debug') ? 'chat_debug' : 'chat';
  const basePath = surface === 'chat_debug' ? '/chat-debug' : '/chat';
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
    queryKey: [...CONVERSATIONS_QUERY_KEY, surface, search],
    queryFn: () => listConversations({ search: search || undefined, surface }),
    enabled: showFull,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteConversation(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
      if (String(id) === conversationId) navigate(basePath);
    },
  });

  const [editingId, setEditingId] = useState<number | null>(null);
  const [editValue, setEditValue] = useState('');

  const renameMutation = useMutation({
    mutationFn: ({ id, title }: { id: number; title: string }) => renameConversation(id, title),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CONVERSATIONS_QUERY_KEY });
    },
  });

  const startRename = (conv: Conversation) => {
    setEditingId(conv.id);
    setEditValue(conv.title || '');
  };
  const cancelRename = () => {
    setEditingId(null);
    setEditValue('');
  };
  const saveRename = (id: number) => {
    const trimmed = editValue.trim();
    if (!trimmed) {
      cancelRename();
      return;
    }
    renameMutation.mutate({ id, title: trimmed });
    setEditingId(null);
  };

  const activeId = conversationId ? Number(conversationId) : undefined;
  const select = (id: number) => navigate(id ? `${basePath}/${id}` : basePath);

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
              onClick={() => navigate(basePath)}
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
        editingId={editingId}
        editValue={editValue}
        onStartRename={startRename}
        onEditValueChange={setEditValue}
        onSaveRename={saveRename}
        onCancelRename={cancelRename}
      />
    </div>
  );
};
