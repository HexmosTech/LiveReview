import React, { useEffect, useRef, useState } from 'react';
import { Badge, Icons } from '../UIPrimitives';
import { useAppSelector } from '../../store/configureStore';
import { selectUnreadCount } from '../../store/Notifications/selectors';
import { NotificationTray } from './NotificationTray';

export const NotificationBell: React.FC = () => {
    const unreadCount = useAppSelector(selectUnreadCount);
    const [isOpen, setIsOpen] = useState(false);
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        };
        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                setIsOpen(false);
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            document.addEventListener('keydown', handleKeyDown);
            return () => {
                document.removeEventListener('mousedown', handleClickOutside);
                document.removeEventListener('keydown', handleKeyDown);
            };
        }
    }, [isOpen]);

    return (
        <div ref={containerRef} className="relative inline-flex items-center">
            <button
                type="button"
                onClick={() => setIsOpen((prev) => !prev)}
                aria-expanded={isOpen}
                aria-haspopup="true"
                className={`relative p-2 rounded-lg text-slate-300 hover:text-white hover:bg-slate-700/60 transition-colors ${
                    isOpen ? 'bg-slate-700/60 text-white' : ''
                }`}
                aria-label={unreadCount > 0 ? `${unreadCount} unread notifications` : 'Notifications'}
                data-testid="notification-bell"
            >
                <Icons.Bell />
                {unreadCount > 0 && (
                    <span className="absolute -top-1 -right-1">
                        <Badge variant="danger" size="sm">
                            {unreadCount > 99 ? '99+' : unreadCount}
                        </Badge>
                    </span>
                )}
            </button>

            {isOpen && (
                <div
                    role="dialog"
                    aria-label="Notifications"
                    className="absolute right-0 top-full mt-2 w-96 rounded-lg border border-slate-600/60 bg-slate-800 shadow-2xl backdrop-blur-sm z-[110] overflow-hidden"
                >
                    <NotificationTray />
                </div>
            )}
        </div>
    );
};

export default NotificationBell;
