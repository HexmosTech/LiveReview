import React from 'react';

type NavbarProps = {
    title: string;
};

export const Navbar: React.FC<NavbarProps> = ({ title }) => {
    return (
        <nav className="bg-slate-800 text-white shadow-lg">
            <div className="container mx-auto px-4 py-3 flex justify-between items-center">
                <div className="flex items-center">
                    <h1 className="text-xl font-bold">{title}</h1>
                </div>
                <div className="flex items-center space-x-4">
                    <button className="px-3 py-1 rounded hover:bg-slate-700 transition">
                        Dashboard
                    </button>
                    <button className="px-3 py-1 rounded hover:bg-slate-700 transition">
                        Connectors
                    </button>
                    <button className="px-3 py-1 rounded hover:bg-slate-700 transition">
                        Settings
                    </button>
                </div>
            </div>
        </nav>
    );
};
