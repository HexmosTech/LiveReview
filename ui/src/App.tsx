import React, { useState } from 'react';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';

const Settings = () => (
    <div className="container mx-auto px-4 py-8">
        <h1 className="text-2xl font-bold text-gray-800 mb-6">Settings</h1>
        <p className="text-gray-600">App settings will go here.</p>
    </div>
);

const App: React.FC = () => {
    const [page, setPage] = useState('dashboard');

    const renderPage = () => {
        switch (page) {
            case 'dashboard':
                return <Dashboard />;
            case 'git':
                return <GitProviders />;
            case 'ai':
                return <AIProviders />;
            case 'settings':
                return <Settings />;
            default:
                return <Dashboard />;
        }
    };

    return (
        <div className="min-h-screen bg-gray-50">
            <Navbar
                title="LiveReview"
                activePage={page}
                onNavigate={setPage}
            />
            <div>
                {renderPage()}
            </div>
        </div>
    );
};

export default App;
