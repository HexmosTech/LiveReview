import React, { useState } from 'react';

type NavbarProps = {
    title: string;
};

export const Navbar: React.FC<NavbarProps> = ({ title }) => {
    const [isOpen, setIsOpen] = useState(false);

    return (
        <nav className="bg-gradient-to-r from-indigo-600 to-purple-600 text-white shadow-xl">
            <div className="container mx-auto px-4 py-4 flex justify-between items-center">
                <div className="flex items-center">
                    <h1 className="text-2xl font-extrabold tracking-tight">{title}</h1>
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
                    <a href="#" className="px-4 py-2 rounded-md hover:bg-white hover:bg-opacity-20 transition font-medium">
                        Dashboard
                    </a>
                    <a href="#" className="px-4 py-2 rounded-md hover:bg-white hover:bg-opacity-20 transition font-medium">
                        Connectors
                    </a>
                    <a href="#" className="px-4 py-2 flex items-center space-x-1 bg-white bg-opacity-20 rounded-md text-white px-4 py-2 font-medium hover:bg-opacity-30 transition">
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        </svg>
                        <span>Settings</span>
                    </a>
                </div>
            </div>
            
            {/* Mobile menu */}
            {isOpen && (
                <div className="md:hidden px-4 py-3 space-y-2 bg-indigo-700 shadow-inner">
                    <a href="#" className="block px-3 py-2 rounded-md hover:bg-indigo-800 transition">
                        Dashboard
                    </a>
                    <a href="#" className="block px-3 py-2 rounded-md hover:bg-indigo-800 transition">
                        Connectors
                    </a>
                    <a href="#" className="block px-3 py-2 rounded-md hover:bg-indigo-800 transition">
                        Settings
                    </a>
                </div>
            )}
        </nav>
    );
};
