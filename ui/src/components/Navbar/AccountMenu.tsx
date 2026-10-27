import React, { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import classNames from 'classnames';
import { useAppSelector } from '../../store/configureStore';
import { Avatar } from './Avatar';

export type AccountMenuProps = {
    onLogout?: () => void;
    className?: string;
};

export const AccountMenu: React.FC<AccountMenuProps> = ({ onLogout, className }) => {
    const user = useAppSelector((state) => state.Auth.user);
    const [isOpen, setIsOpen] = useState(false);
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [isOpen]);

    if (!user) return null;

    const displayName = user.name || user.email;

    return (
        <div ref={containerRef} className={classNames('relative', className)}>
            <button
                type="button"
                onClick={() => setIsOpen((open) => !open)}
                aria-haspopup="true"
                aria-expanded={isOpen}
                aria-label="Account menu"
                className={classNames(
                    'rounded-full transition-all',
                    isOpen ? 'ring-2 ring-blue-400' : 'hover:ring-2 hover:ring-slate-500'
                )}
            >
                <Avatar identity={user} />
            </button>

            {isOpen && (
                <div
                    role="menu"
                    aria-label="Account menu"
                    className="absolute right-0 mt-2 w-64 rounded-lg border border-slate-600/60 bg-slate-800 shadow-xl backdrop-blur-sm z-[110] py-1"
                >
                    <div className="flex items-center gap-3 px-4 py-3 border-b border-slate-700/60">
                        <Avatar identity={user} size={38} />
                        <div className="min-w-0">
                            <div className="text-sm font-semibold text-slate-100 truncate">{displayName}</div>
                            <div className="text-xs text-slate-400 truncate">{user.email}</div>
                        </div>
                    </div>

                    <Link
                        to="/settings#account"
                        role="menuitem"
                        onClick={() => setIsOpen(false)}
                        className="flex items-center gap-2 px-4 py-2 text-sm text-slate-200 hover:bg-slate-700/60 transition-colors"
                    >
                        <svg className="w-4 h-4 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                        </svg>
                        Profile and preferences
                    </Link>

                    <Link
                        to="/settings#account"
                        role="menuitem"
                        onClick={() => setIsOpen(false)}
                        className="flex items-center gap-2 px-4 py-2 text-sm text-slate-200 hover:bg-slate-700/60 transition-colors"
                    >
                        <svg className="w-4 h-4 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 10-8 0v4h8z" />
                        </svg>
                        Change password
                    </Link>

                    <Link
                        to="/settings#api-keys"
                        role="menuitem"
                        onClick={() => setIsOpen(false)}
                        className="flex items-center gap-2 px-4 py-2 text-sm text-slate-200 hover:bg-slate-700/60 transition-colors"
                    >
                        <svg className="w-4 h-4 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                        </svg>
                        API keys
                    </Link>

                    {onLogout && (
                        <>
                            <div className="my-1 border-t border-slate-700/60" />
                            <button
                                type="button"
                                role="menuitem"
                                onClick={() => {
                                    setIsOpen(false);
                                    onLogout();
                                }}
                                className="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-300 hover:bg-red-900/20 transition-colors text-left"
                            >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                                </svg>
                                Sign out
                            </button>
                        </>
                    )}
                </div>
            )}
        </div>
    );
};
