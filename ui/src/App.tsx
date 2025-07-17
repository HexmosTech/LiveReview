import React, { useState } from 'react';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';
import { PageHeader, Card, Section, Button, Icons } from './components/UIPrimitives';

const Settings = () => (
    <div className="container mx-auto px-4 py-8">
        <PageHeader 
            title="Settings" 
            description="Configure application preferences and behaviors"
        />
        
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="md:col-span-1">
                <Card title="Navigation">
                    <div className="space-y-2">
                        <Button 
                            variant="ghost" 
                            fullWidth 
                            className="justify-start"
                            icon={<Icons.Settings />}
                        >
                            General
                        </Button>
                        <Button 
                            variant="ghost" 
                            fullWidth 
                            className="justify-start"
                            icon={<Icons.AI />}
                        >
                            AI Configuration
                        </Button>
                        <Button 
                            variant="ghost" 
                            fullWidth 
                            className="justify-start"
                            icon={<Icons.Dashboard />}
                        >
                            UI Preferences
                        </Button>
                    </div>
                </Card>
            </div>
            
            <div className="md:col-span-2">
                <Section title="General Settings">
                    <Card className="card-brand">
                        <div className="flex items-center mb-4">
                            <img src="/assets/logo.svg" alt="LiveReview Logo" className="h-8 w-auto mr-3" />
                            <div>
                                <h3 className="font-medium text-gray-900">LiveReview v1.0.0</h3>
                                <p className="text-sm text-gray-500">Automated code reviews powered by AI</p>
                            </div>
                        </div>
                        <p className="text-gray-600">App settings content will go here.</p>
                    </Card>
                </Section>
            </div>
        </div>
    </div>
);

const Footer = () => (
    <footer className="bg-gray-900 border-t border-gray-700 py-8 mt-auto">
        <div className="container mx-auto px-4 flex flex-col md:flex-row justify-between items-center">
            <div className="flex items-center">
                <img src="/assets/logo-with-text.svg" alt="LiveReview Logo" className="h-10 w-auto" />
            </div>
            <div className="text-right mt-4 md:mt-0">
                <p className="text-sm text-gray-400">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
            </div>
        </div>
    </footer>
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
        <div className="min-h-screen flex flex-col">
            <Navbar
                title="LiveReview"
                activePage={page}
                onNavigate={setPage}
            />
            <div className="flex-grow">
                {renderPage()}
            </div>
            <Footer />
        </div>
    );
};

export default App;
