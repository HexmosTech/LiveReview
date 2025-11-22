import React, { useEffect, useRef } from 'react';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { useOrgContext } from '../../hooks/useOrgContext';
import { Organization } from '../../store/Organizations/types';

export interface OrganizationSelectorProps {
    /**
     * Position of the selector (for styling)
     */
    position?: 'navbar' | 'sidebar' | 'inline';
    
    /**
     * Size variant
     */
    size?: 'sm' | 'md' | 'lg';
    
    /**
     * Custom className
     */
    className?: string;
    
    /**
     * Show organization creation option (super admin only)
     */
    showCreateOption?: boolean;
    
    /**
     * Callback when organization is switched
     */
    onOrgSwitch?: (org: Organization) => void;
    
    /**
     * Callback when create organization is clicked
     */
    onCreateOrg?: () => void;
}

export const OrganizationSelector: React.FC<OrganizationSelectorProps> = ({
    position = 'navbar',
    size = 'md',
    className,
    showCreateOption = true,
    onOrgSwitch,
    onCreateOrg,
}) => {
    const {
        currentOrg,
        userOrganizations,
        loading,
        error,
        orgSelectorOpen,
        isSuperAdmin,
        hasOrganizations,
        loadUserOrgs,
        switchToOrg,
        toggleOrgSelector,
        closeOrgSelector,
        clearOrgError,
    } = useOrgContext();

    const dropdownRef = useRef<HTMLDivElement>(null);

    // Load organizations on mount - This is now handled by useOrgContext
    // useEffect(() => {
    //     if (!hasOrganizations && !loading.organizations) {
    //         loadUserOrgs();
    //     }
    // }, [hasOrganizations, loading.organizations, loadUserOrgs]);

    // Handle click outside to close dropdown
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                closeOrgSelector();
            }
        };

        if (orgSelectorOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [orgSelectorOpen, closeOrgSelector]);

    // Handle organization switch
    const handleOrgSwitch = (org: Organization) => {
        switchToOrg(org.id);
        if (onOrgSwitch) {
            onOrgSwitch(org);
        }
        // Reload the page to refresh all data for the new organization
        window.location.reload();
    };

    // Handle create organization
    const handleCreateOrg = () => {
        closeOrgSelector();
        if (onCreateOrg) {
            onCreateOrg();
        }
    };

    // Size-based styling
    const sizeStyles = {
        sm: {
            button: 'px-2 py-1 text-sm',
            dropdown: 'w-48 text-sm',
            icon: 'w-3 h-3',
        },
        md: {
            button: 'px-3 py-2 text-sm',
            dropdown: 'w-56 text-sm',
            icon: 'w-4 h-4',
        },
        lg: {
            button: 'px-4 py-2 text-base',
            dropdown: 'w-64 text-base',
            icon: 'w-5 h-5',
        },
    };

    // Position-based styling
    const positionStyles = {
        navbar: 'relative',
        sidebar: 'relative w-full',
        inline: 'relative inline-block',
    };

    const styles = sizeStyles[size];

    // Show loading state
    if (loading.organizations && !hasOrganizations) {
        return (
            <div className={classNames(positionStyles[position], className)}>
                <Button
                    variant="ghost"
                    disabled
                    className={classNames(
                        styles.button,
                        'bg-slate-700/50 text-slate-400',
                        position === 'sidebar' && 'w-full justify-start'
                    )}
                    icon={
                        <div className={classNames(styles.icon, 'animate-spin')}>
                            <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                            </svg>
                        </div>
                    }
                >
                    Loading...
                </Button>
            </div>
        );
    }

    // Show error state
    if (error && !hasOrganizations) {
        return (
            <div className={classNames(positionStyles[position], className)}>
                <Button
                    variant="ghost"
                    onClick={() => {
                        clearOrgError();
                        loadUserOrgs();
                    }}
                    className={classNames(
                        styles.button,
                        'bg-red-900/20 text-red-300 hover:bg-red-900/30',
                        position === 'sidebar' && 'w-full justify-start'
                    )}
                    icon={
                        <svg className={styles.icon} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                        </svg>
                    }
                >
                    Error - Retry
                </Button>
            </div>
        );
    }

    // Show no organizations state
    if (!hasOrganizations) {
        return (
            <div className={classNames(positionStyles[position], className)}>
                <Button
                    variant="ghost"
                    disabled
                    className={classNames(
                        styles.button,
                        'bg-slate-700/50 text-slate-400',
                        position === 'sidebar' && 'w-full justify-start'
                    )}
                    icon={
                        <svg className={styles.icon} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                        </svg>
                    }
                >
                    No Organizations
                </Button>
            </div>
        );
    }

    return (
        <div className={classNames(positionStyles[position], className)} ref={dropdownRef}>
            {/* Selector Button */}
            <Button
                variant="ghost"
                onClick={toggleOrgSelector}
                className={classNames(
                    styles.button,
                    'bg-slate-700/60 hover:bg-slate-600/60 text-slate-200 border border-slate-600/60',
                    position === 'sidebar' && 'w-full justify-between',
                    position !== 'sidebar' && 'justify-start',
                    orgSelectorOpen && 'bg-slate-600/60'
                )}
                icon={
                    <svg className={styles.icon} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                    </svg>
                }
                iconPosition="left"
            >
                <span className="truncate">
                    {currentOrg ? currentOrg.name : 'Select Organization'}
                </span>
                <svg
                    className={classNames(
                        styles.icon,
                        'ml-auto transition-transform duration-200',
                        orgSelectorOpen && 'rotate-180'
                    )}
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                </svg>
            </Button>

            {/* Dropdown Menu */}
            {orgSelectorOpen && (
                <div className={classNames(
                    'absolute z-50 mt-1 bg-slate-800 border border-slate-600/60 rounded-lg shadow-xl backdrop-blur-sm',
                    styles.dropdown,
                    position === 'navbar' && 'right-0',
                    position === 'sidebar' && 'left-0',
                    position === 'inline' && 'left-0'
                )}>
                    <div className="py-2">
                        {/* Organizations List */}
                        <div className="px-3 py-1 text-xs font-medium text-slate-400 uppercase tracking-wider">
                            Organizations
                        </div>
                        
                        {userOrganizations.map((org) => (
                            <button
                                key={org.id}
                                onClick={() => handleOrgSwitch(org)}
                                className={classNames(
                                    'w-full px-3 py-2 text-left hover:bg-slate-700/60 transition-colors',
                                    'flex items-center space-x-2',
                                    currentOrg?.id === org.id && 'bg-blue-600/20 text-blue-300'
                                )}
                                disabled={loading.switching}
                            >
                                <svg className={styles.icon} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
                                </svg>
                                <div className="flex-1 min-w-0">
                                    <div className="font-medium truncate">{org.name}</div>
                                    {org.role && (
                                        <div className="text-xs text-slate-400 capitalize">{org.role}</div>
                                    )}
                                </div>
                                {currentOrg?.id === org.id && (
                                    <svg className={styles.icon} fill="currentColor" viewBox="0 0 20 20">
                                        <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                                    </svg>
                                )}
                            </button>
                        ))}

                        {/* Create Organization Option (Super Admin) */}
                        {isSuperAdmin && showCreateOption && (
                            <>
                                <div className="border-t border-slate-600/60 my-2"></div>
                                <button
                                    onClick={handleCreateOrg}
                                    className="w-full px-3 py-2 text-left hover:bg-slate-700/60 transition-colors flex items-center space-x-2 text-green-400"
                                >
                                    <svg className={styles.icon} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                                    </svg>
                                    <span>Create Organization</span>
                                </button>
                            </>
                        )}
                    </div>
                </div>
            )}

            {/* Loading Overlay */}
            {loading.switching && (
                <div className="absolute inset-0 bg-slate-900/50 rounded flex items-center justify-center">
                    <div className={classNames(styles.icon, 'animate-spin text-blue-400')}>
                        <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                    </div>
                </div>
            )}
        </div>
    );
};