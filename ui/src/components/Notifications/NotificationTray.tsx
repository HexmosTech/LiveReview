import React from 'react';
import classNames from 'classnames';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { selectVisibleList } from '../../store/Notifications/selectors';
import { markRead, markAllRead, dismiss } from '../../store/Notifications/slice';
import { NotificationSeverity } from '../../store/Notifications/types';
import { runNotificationAction } from '../../store/Notifications/actionRegistry';
import { Icons } from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp/HumanizedTimestamp';

const severityDot: Record<NotificationSeverity, string> = {
    info: 'bg-blue-400',
    success: 'bg-green-400',
    warning: 'bg-amber-400',
    error: 'bg-red-400',
};

export const NotificationTray: React.FC = () => {
    const dispatch = useAppDispatch();
    const items = useAppSelector(selectVisibleList);
    const hasUnread = items.some((n) => !n.read);

    return (
        <div className="flex flex-col max-h-[28rem]" data-testid="notification-tray">
            <div className="flex items-center justify-between px-4 py-3 border-b border-slate-700 flex-shrink-0">
                <span className="font-semibold text-slate-100">Notifications</span>
                {hasUnread && (
                    <button
                        type="button"
                        className="text-xs text-blue-400 hover:text-blue-300"
                        onClick={() => dispatch(markAllRead())}
                    >
                        Mark all read
                    </button>
                )}
            </div>
            <div className="flex-1 overflow-y-auto">
                {items.length === 0 ? (
                    <div className="px-4 py-8 text-center text-slate-400 text-sm">You're all caught up</div>
                ) : (
                    items.map((n) => (
                        <div
                            key={n.id}
                            className={classNames(
                                'flex items-start gap-2.5 px-4 py-3 border-b border-slate-700/60 last:border-b-0 cursor-pointer hover:bg-slate-700/40 transition-colors',
                                !n.read && 'bg-slate-700/20'
                            )}
                            onClick={() => !n.read && dispatch(markRead(n.id))}
                        >
                            <span
                                className={classNames(
                                    'mt-1.5 w-2 h-2 rounded-full flex-shrink-0',
                                    n.read ? 'bg-transparent' : severityDot[n.severity]
                                )}
                            />
                            <div className="flex-1 min-w-0">
                                {n.title && <p className="text-sm font-medium text-slate-100">{n.title}</p>}
                                <p className="text-sm text-slate-300 break-words">{n.message}</p>
                                <div className="flex items-center gap-3 mt-1">
                                    <HumanizedTimestamp
                                        timestamp={new Date(n.createdAt)}
                                        className="text-xs text-slate-500 inline-block"
                                    />
                                    {n.actions && n.actions.length > 0 && (
                                        <div className="flex items-center gap-3">
                                            {n.actions.map((action) => (
                                                <button
                                                    key={action.actionId}
                                                    type="button"
                                                    className="text-xs font-medium text-blue-400 hover:text-blue-300"
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        runNotificationAction(action.actionId);
                                                    }}
                                                >
                                                    {action.label}
                                                </button>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            </div>
                            <button
                                type="button"
                                aria-label="Dismiss notification"
                                className="text-slate-500 hover:text-slate-200 flex-shrink-0"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    dispatch(dismiss(n.id));
                                }}
                            >
                                <Icons.Close />
                            </button>
                        </div>
                    ))
                )}
            </div>
        </div>
    );
};

export default NotificationTray;
