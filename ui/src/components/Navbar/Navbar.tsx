import React, { useState } from 'react';
import { Button, Icons } from '../UIPrimitives';

type NavbarProps = {
    title: string;
    activePage?: string;
    onNavigate?: (page: string) => void;
};

const navLinks = [
    { name: 'Dashboard', key: 'dashboard', icon: <Icons.Dashboard /> },
    { name: 'Git Providers', key: 'git', icon: <Icons.Git /> },
    { name: 'AI Providers', key: 'ai', icon: <Icons.AI /> },
    { name: 'Settings', key: 'settings', icon: <Icons.Settings /> },
];

export const Navbar: React.FC<NavbarProps> = ({ title, activePage = 'dashboard', onNavigate }) => {
    const [isOpen, setIsOpen] = useState(false);

    const handleNavClick = (key: string) => {
        if (onNavigate) onNavigate(key);
        setIsOpen(false);
    };

    return (
        <nav className="bg-white shadow-sm border-b border-gray-200 sticky top-0 z-10">
            <div className="container mx-auto px-4 py-3 flex justify-between items-center">
                <div className="flex items-center">
                    <img src="/assets/logo.svg" alt="LiveReview Logo" className="h-8 w-auto mr-3" />
                    <h1 className="text-xl font-bold text-gray-900">{title}</h1>
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
                            onClick={() => handleNavClick(link.key)}
                            icon={link.icon}
                            className={activePage === link.key ? '' : 'text-gray-600'}
                        >
                            {link.name}
                        </Button>
                    ))}
                </div>
            </div>
            
            {/* Mobile menu dropdown */}
            {isOpen && (
                <div className="md:hidden px-4 py-3 space-y-2 bg-white border-t border-gray-200 shadow-inner">
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={() => handleNavClick(link.key)}
                            icon={link.icon}
                            className="w-full justify-start"
                            iconPosition="left"
                        >
                            {link.name}
                        </Button>
                    ))}
                </div>
            )}
        </nav>
    );
};
