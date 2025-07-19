import React, { useState } from 'react';
import { Button, Icons } from '../UIPrimitives';

export type NavbarProps = {
    title: string;
    activePage?: string;
    onNavigate?: (page: string) => void;
    onLogout?: () => void;
};

const navLinks = [
    { name: 'Dashboard', key: 'dashboard', icon: <Icons.Dashboard /> },
    { name: 'Git Providers', key: 'git', icon: <Icons.Git /> },
    { name: 'AI Providers', key: 'ai', icon: <Icons.AI /> },
    { name: 'Settings', key: 'settings', icon: <Icons.Settings /> },
];

export const Navbar: React.FC<NavbarProps> = ({ title, activePage = 'dashboard', onNavigate, onLogout }) => {
    const [isOpen, setIsOpen] = useState(false);

    const handleNavClick = (key: string) => {
        if (onNavigate) onNavigate(key);
        setIsOpen(false);
    };

    return (
        <nav className="bg-slate-900 shadow-md border-b border-slate-700 sticky top-0 z-10 navbar-dark">
            <div className="container mx-auto px-4 py-4 flex justify-between items-center">
                <div className="flex items-center">
                    <a 
                        href="#dashboard" 
                        onClick={(e) => {
                            e.preventDefault();
                            handleNavClick('dashboard');
                        }} 
                        className="cursor-pointer"
                        role="button"
                        aria-label="Go to home"
                    >
                        <img src="/assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-12 w-auto mr-3" />
                    </a>
                </div>
                
                {/* Mobile menu button */}
                <div className="md:hidden">
                    <Button
                        variant="ghost"
                        onClick={() => setIsOpen(!isOpen)}
                        aria-label="Toggle menu"
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
                <div className="hidden md:flex items-center space-x-1">
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={(e) => {
                                e.preventDefault();
                                handleNavClick(link.key);
                            }}
                            icon={link.icon}
                            className={activePage === link.key ? '' : 'text-slate-300'}
                            href={`#${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    
                    {/* Logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="ml-4 text-slate-300"
                            icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
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
                <div className="md:hidden px-4 py-3 space-y-2 bg-slate-800 border-t border-slate-700 shadow-lg">
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={(e) => {
                                e.preventDefault();
                                handleNavClick(link.key);
                            }}
                            icon={link.icon}
                            className="w-full justify-start text-slate-100"
                            iconPosition="left"
                            href={`#${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    
                    {/* Mobile logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="w-full justify-start text-slate-100"
                            iconPosition="left"
                            icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
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
