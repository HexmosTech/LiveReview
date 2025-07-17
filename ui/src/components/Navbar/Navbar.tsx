import React, { useState } from 'react';

type NavbarProps = {
    title: string;
    activePage?: string;
    onNavigate?: (page: string) => void;
};

const navLinks = [
    { name: 'Dashboard', key: 'dashboard' },
    { name: 'Git Providers', key: 'git' },
    { name: 'AI Providers', key: 'ai' },
    { name: 'Settings', key: 'settings' },
];

export const Navbar: React.FC<NavbarProps> = ({ title, activePage = 'dashboard', onNavigate }) => {
    const [isOpen, setIsOpen] = useState(false);

    const handleNavClick = (key: string) => {
        if (onNavigate) onNavigate(key);
    };

    return (
        <nav className="bg-livereview text-cardText shadow-xl border-b border-cardPurple">
            <div className="container mx-auto px-4 py-4 flex justify-between items-center">
                <div className="flex items-center">
                    <h1 className="text-2xl font-extrabold tracking-tight text-accent">{title}</h1>
                </div>
                {/* Mobile menu button */}
                <div className="md:hidden">
                    <button 
                        onClick={() => setIsOpen(!isOpen)}
                        className="focus:outline-none"
                    >
                        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            {isOpen ? (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            ) : (
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                            )}
                        </svg>
                    </button>
                </div>
                {/* Desktop menu */}
                <div className="hidden md:flex items-center space-x-4">
                    {navLinks.map(link => (
                        <button
                            key={link.key}
                            onClick={() => handleNavClick(link.key)}
                            className={`px-4 py-2 rounded-md font-bold transition focus:outline-none ${activePage === link.key ? 'bg-cardPurple text-cardText shadow' : 'hover:bg-cardPurple hover:text-cardText text-accent'}`}
                        >
                            {link.name}
                        </button>
                    ))}
                </div>
            </div>
            {/* Mobile menu */}
            {isOpen && (
                <div className="md:hidden px-4 py-3 space-y-2 bg-livereview border-t border-cardPurple shadow-inner">
                    {navLinks.map(link => (
                        <button
                            key={link.key}
                            onClick={() => handleNavClick(link.key)}
                            className={`block px-3 py-2 rounded-md transition focus:outline-none ${activePage === link.key ? 'bg-cardPurple text-cardText shadow' : 'hover:bg-cardPurple hover:text-cardText text-accent'}`}
                        >
                            {link.name}
                        </button>
                    ))}
                </div>
            )}
        </nav>
    );
};
