import React, { useState, useEffect } from 'react';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';
import { PageHeader, Card, Section, Button, Icons, Input, Alert } from './components/UIPrimitives';
import { useAppDispatch, useAppSelector } from './store/configureStore';
import { updateDomain } from './store/Settings/reducer';

const Settings = () => {
    const dispatch = useAppDispatch();
    const { domain, isConfigured } = useAppSelector((state) => state.Settings);
    const [localDomain, setLocalDomain] = useState(domain);
    const [showSaved, setShowSaved] = useState(false);

    const handleSaveDomain = () => {
        dispatch(updateDomain(localDomain));
        setShowSaved(true);
        setTimeout(() => setShowSaved(false), 3000);
    };

    return (
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
                                variant="primary" 
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
                        <Card>
                            <div className="flex items-center mb-6">
                                <img src="/assets/logo.svg" alt="LiveReview Logo" className="h-8 w-auto mr-3" />
                                <div>
                                    <h3 className="font-medium text-white">LiveReview v1.0.0</h3>
                                    <p className="text-sm text-slate-300">Automated code reviews powered by AI</p>
                                </div>
                            </div>

                            {showSaved && (
                                <Alert 
                                    variant="success" 
                                    icon={<Icons.Success />}
                                    className="mb-4"
                                    onClose={() => setShowSaved(false)}
                                >
                                    Settings saved successfully!
                                </Alert>
                            )}
                            
                            <div className="space-y-6">
                                <div>
                                    <h3 className="text-lg font-medium text-white mb-2">Application Domain</h3>
                                    <p className="text-sm text-slate-300 mb-4">
                                        Configure your application's domain. This is required for setting up OAuth 
                                        connections with services like GitLab and GitHub.
                                    </p>
                                    
                                    <div className="flex flex-col sm:flex-row gap-3">
                                        <div className="flex-grow">
                                            <Input
                                                label="Domain"
                                                placeholder="livereview.your-company.com"
                                                value={localDomain}
                                                onChange={(e) => setLocalDomain(e.target.value)}
                                                helperText="Enter the domain where your LiveReview instance is hosted"
                                            />
                                        </div>
                                        <div className="flex items-end">
                                            <Button 
                                                onClick={handleSaveDomain}
                                                variant="primary"
                                            >
                                                Save
                                            </Button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </Card>
                    </Section>
                </div>
            </div>
        </div>
    );
};

const Footer = ({ onNavigateToHome }: { onNavigateToHome: () => void }) => (
    <footer className="bg-slate-900 border-t border-slate-700 py-8 mt-auto">
        <div className="container mx-auto px-4 flex flex-col md:flex-row justify-between items-center">
            <div className="flex items-center py-2">
                <a 
                    href="#dashboard"
                    onClick={(e) => {
                        e.preventDefault();
                        onNavigateToHome();
                    }} 
                    className="cursor-pointer"
                    role="button"
                    aria-label="Go to home"
                >
                    <img src="/assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-10 w-auto" />
                </a>
            </div>
            <div className="text-right mt-4 md:mt-0">
                <p className="text-sm text-slate-200">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
            </div>
        </div>
    </footer>
);

const App: React.FC = () => {
    // Function to get the initial page from URL hash
    const getInitialPage = (): string => {
        const hash = window.location.hash.replace('#', '');
        return ['dashboard', 'git', 'ai', 'settings'].includes(hash) ? hash : 'dashboard';
    };

    const [page, setPage] = useState(getInitialPage());

    // Update URL hash when page changes
    useEffect(() => {
        window.location.hash = page;
    }, [page]);

    // Listen for hash changes (e.g. when user uses browser navigation)
    useEffect(() => {
        const handleHashChange = () => {
            const newPage = getInitialPage();
            if (newPage !== page) {
                setPage(newPage);
            }
        };

        window.addEventListener('hashchange', handleHashChange);
        return () => window.removeEventListener('hashchange', handleHashChange);
    }, [page]);

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
            <Footer onNavigateToHome={() => setPage('dashboard')} />
        </div>
    );
};

export default App;
