import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { OrganizationSelector } from '../OrganizationSelector';
import { useSystemInfo } from '../../hooks/useSystemInfo';
import { useOrgContext } from '../../hooks/useOrgContext';

export type NavbarProps = {
    title: string;
    activePage?: string;
    onNavigate?: (page: string) => void;
    onLogout?: () => void;
};

const baseNavLinks = [
    { name: 'Dashboard', key: 'dashboard', icon: <Icons.Dashboard /> },
    { name: 'Reviews', key: 'reviews', icon: <Icons.Reviews /> },
    { name: 'Git Providers', key: 'git', icon: <Icons.Git />, requiresOwnerOrAdmin: true },
    { name: 'AI Providers', key: 'ai', icon: <Icons.AI />, requiresOwnerOrAdmin: true },
    { name: 'Settings', key: 'settings', icon: <Icons.Settings /> },
];

const testNavLink = {
    name: 'Test Middleware', 
    key: 'test-middleware', 
    icon: (
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
    )
};

export const Navbar: React.FC<NavbarProps> = ({ title, activePage = 'dashboard', onNavigate, onLogout }) => {
    const [isOpen, setIsOpen] = useState(false);
    const { isDevMode } = useSystemInfo();
    const { isSuperAdmin, currentOrg } = useOrgContext();

    // Check if user can manage current org (owner or super admin)
    const canManageCurrentOrg = isSuperAdmin || currentOrg?.role === 'owner';

    // Filter nav links based on permissions
    const filteredBaseLinks = baseNavLinks.filter(link => {
        if (link.requiresOwnerOrAdmin) {
            return canManageCurrentOrg;
        }
        return true;
    });

    // Conditionally include test middleware link based on dev mode
    const navLinks = isDevMode ? [...filteredBaseLinks, testNavLink] : filteredBaseLinks;

    const handleNavClick = (key: string) => {
        if (onNavigate) onNavigate(key);
        setIsOpen(false);
    };

    return (
        <nav className="bg-slate-900/95 backdrop-blur-sm shadow-lg border-b border-slate-700/60 sticky top-0 z-50">
            <div className="container mx-auto px-4 py-3 flex justify-between items-center">
                <div className="flex items-center">
                    <Link 
                        to="/"
                        onClick={() => handleNavClick('dashboard')}
                        className="cursor-pointer transition-transform hover:scale-105"
                        role="button"
                        aria-label="Go to home"
                    >
                        <img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-10 w-auto mr-3" />
                    </Link>
                </div>
                
                {/* Mobile menu button */}
                <div className="md:hidden">
                    <Button
                        variant="ghost"
                        onClick={() => setIsOpen(!isOpen)}
                        aria-label="Toggle menu"
                        className="text-slate-300"
                        icon={isOpen ? (
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        ) : (
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                            </svg>
                        )}
                    />
                </div>
                
                {/* Desktop menu */}
                <div className="hidden md:flex items-center space-x-2">
                    {/* Organization Selector */}
                    <OrganizationSelector 
                        position="navbar"
                        size="sm"
                        className="mr-4"
                    />
                    
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={() => handleNavClick(link.key)}
                            icon={link.icon}
                            className={classNames(
                                'text-sm font-medium transition-all duration-200',
                                activePage === link.key 
                                    ? 'bg-blue-600 text-white shadow-lg' 
                                    : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                            )}
                            as={Link}
                            to={`/${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    
                    {/* Logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="ml-3 text-slate-300 hover:text-red-300 hover:bg-red-900/20 transition-colors"
                            icon={<svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                            </svg>}
                        >
                            Logout
                        </Button>
                    )}
                </div>
            </div>
            
            {/* Mobile menu dropdown */}
            {isOpen && (
                <div className="md:hidden px-4 py-3 space-y-2 bg-slate-800/95 border-t border-slate-700/60 backdrop-blur-sm">
                    {/* Mobile Organization Selector */}
                    <div className="mb-3">
                        <OrganizationSelector 
                            position="sidebar"
                            size="sm"
                        />
                    </div>
                    
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={() => handleNavClick(link.key)}
                            icon={link.icon}
                            className={classNames(
                                'w-full justify-start text-sm font-medium',
                                activePage === link.key 
                                    ? 'bg-blue-600 text-white' 
                                    : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                            )}
                            iconPosition="left"
                            as={Link}
                            to={`/${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    
                    {/* Mobile logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="w-full justify-start text-slate-300 hover:text-red-300 hover:bg-red-900/20"
                            iconPosition="left"
                            icon={<svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                            </svg>}
                        >
                            Logout
                        </Button>
                    )}
                </div>
            )}
        </nav>
    );
};
