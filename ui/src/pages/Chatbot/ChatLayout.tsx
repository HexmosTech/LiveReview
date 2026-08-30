import React from 'react';
import { Outlet } from 'react-router-dom';
import { ConversationSidebar } from './ConversationSidebar';
import { useChatSidebarReservedWidth, useChatSidebarResizing } from '../../store/chatSidebar';

// Sidebar is a true left rail, pinned to the viewport's actual left edge,
// spanning the full height (not inset inside the same centered `container`
// the page content uses, and not just below the navbar) - so it reads as
// part of the site's own navigation rather than a floating panel. The main
// content area gets a matching left margin so it starts right where the
// sidebar's *reserved* width ends - deliberately NOT the same as the
// sidebar's rendered width: a hover-peek while unpinned is an overlay on
// top of this content, not a layout change, so nothing here shifts just
// because the mouse is over the rail (see chatSidebar.ts).
export const ChatLayout: React.FC = () => {
  const reservedWidth = useChatSidebarReservedWidth();
  const resizing = useChatSidebarResizing();

  return (
    <div className="h-[calc(100vh-4rem)] bg-slate-900 relative">
      <ConversationSidebar />
      <div
        className={resizing ? 'h-full' : 'h-full transition-[margin-left] duration-200 ease-in-out'}
        style={{ marginLeft: reservedWidth }}
      >
        <Outlet />
      </div>
    </div>
  );
};
