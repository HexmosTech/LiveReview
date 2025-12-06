import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { OrganizationSelector } from '../OrganizationSelector';
import { useSystemInfo } from '../../hooks/useSystemInfo';
import { useOrgContext } from '../../hooks/useOrgContext';
import { useAppSelector } from '../../store/configureStore';

// Upgrade Badge Component for Navbar
const UpgradeBadge: React.FC = () => {
    const navigate = useNavigate();
    
    return (
        <button
            onClick={() => navigate('/subscribe')}
            className="relative ml-2 px-4 py-2 bg-gradient-to-r from-yellow-500 to-orange-500 hover:from-yellow-400 hover:to-orange-400 text-slate-900 text-sm font-bold rounded-lg transition-all duration-200 shadow-lg hover:shadow-xl transform hover:scale-105 flex items-center gap-2"
        >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            Upgrade
        </button>
    );
};

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
                    
                    {/* Upgrade / Manage Licenses */}
                    <UpgradeBadge />
                    
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
                    
                    {/* Mobile Upgrade/Manage Licenses */}
                    <div className="pt-2 border-t border-slate-700">
                        <UpgradeBadge />
                    </div>
                    
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
