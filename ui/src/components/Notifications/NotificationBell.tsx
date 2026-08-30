import React from 'react';
import { Popover, Badge, Icons } from '../UIPrimitives';
import { useAppSelector } from '../../store/configureStore';
import { selectUnreadCount } from '../../store/Notifications/selectors';
import { NotificationTray } from './NotificationTray';

export const NotificationBell: React.FC = () => {
    const unreadCount = useAppSelector(selectUnreadCount);

    return (
        <Popover
            align="right"
            estimatedWidth={360}
            estimatedHeight={420}
            className="!w-96 !p-0 overflow-hidden"
            trigger={
                <button
                    type="button"
                    className="relative p-2 rounded-lg text-slate-300 hover:text-white hover:bg-slate-700/60 transition-colors"
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
            }
        >
            <NotificationTray />
        </Popover>
    );
};

export default NotificationBell;
