import React, { useState, useEffect } from 'react';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';
import Settings from './pages/Settings/Settings';
import Login from './pages/Auth/Login';
import SetPassword from './pages/Auth/SetPassword';
import CodeHostCallback from './pages/Auth/CodeHostCallback';
import { useAppDispatch, useAppSelector } from './store/configureStore';
import { logout, checkPasswordStatus } from './store/Auth/reducer';

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
    const dispatch = useAppDispatch();
    const { isAuthenticated, isPasswordSet, isLoading } = useAppSelector((state) => state.Auth);
    const [oauthCode, setOauthCode] = useState<string | null>(null);
    
    // Function to get the initial page from URL hash
    const getInitialPage = (): string => {
        const hashPath = window.location.hash.replace('#', '').replace(/^\//, '');
        
        // Handle OAuth callback detection in URL parameters
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.has('code')) {
            return 'oauth-callback';
        }
        
        return ['dashboard', 'git', 'ai', 'settings'].includes(hashPath) ? hashPath : 'dashboard';
    };

    const [page, setPage] = useState(getInitialPage());

    // Check for OAuth code in URL on load
    useEffect(() => {
        const urlParams = new URLSearchParams(window.location.search);
        const code = urlParams.get('code');
        if (code) {
            console.log('App.tsx - Found OAuth code in URL:', code);
            setOauthCode(code);
            
            // Optionally clean up the URL to remove the code parameter
            if (window.history && window.history.replaceState) {
                const cleanUrl = window.location.href.split('?')[0];
                window.history.replaceState({}, document.title, cleanUrl);
            }
        }
    }, []);

    // Check password status on app load
    useEffect(() => {
        console.log('App.tsx - Checking password status on load');
        dispatch(checkPasswordStatus());
    }, [dispatch]);
    
    // Debug listener for Auth state changes
    useEffect(() => {
        console.log('Auth state changed - isAuthenticated:', isAuthenticated, 'isPasswordSet:', isPasswordSet);
    }, [isAuthenticated, isPasswordSet]);

    // Update URL hash when page changes
    useEffect(() => {
        console.log('App.tsx - Page changed to:', page);
        // Don't update hash for callback page since it uses path
        if (page !== 'codehost-callback') {
            window.location.hash = page;
        }
    }, [page]);

    // Listen for hash changes (e.g. when user uses browser navigation)
    useEffect(() => {
        const handleHashChange = () => {
            const newPage = getInitialPage();
            if (newPage !== page) {
                console.log('App.tsx - Hash changed to:', newPage);
                setPage(newPage);
            }
        };

        window.addEventListener('hashchange', handleHashChange);
        
        // Also handle the initial hash on component mount
        // This is especially important for handling redirects from external services
        handleHashChange();
        
        return () => window.removeEventListener('hashchange', handleHashChange);
    }, [page]);

    // Handle logout
    const handleLogout = () => {
        dispatch(logout());
        // After logout, check the password status to determine which page to show
        dispatch(checkPasswordStatus());
    };

    // Show loading state while checking password status
    if (isLoading) {
        console.log('App.tsx - Rendering loading state');
        return (
            <div className="min-h-screen flex items-center justify-center">
                <div className="text-center">
                    <svg className="w-12 h-12 mx-auto mb-4 text-blue-500 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    <h2 className="text-xl font-medium text-white">Loading LiveReview...</h2>
                </div>
            </div>
        );
    }

    // If not authenticated and password is not set, show set password page
    if (!isAuthenticated && !isPasswordSet) {
        console.log('App.tsx - Showing SetPassword page - isAuthenticated:', isAuthenticated, 'isPasswordSet:', isPasswordSet);
        return <SetPassword />;
    }

    // If not authenticated but password is set, show login page
    if (!isAuthenticated && isPasswordSet) {
        console.log('App.tsx - Showing Login page - isAuthenticated:', isAuthenticated, 'isPasswordSet:', isPasswordSet);
        return <Login />;
    }
    
    console.log('App.tsx - Showing main application - isAuthenticated:', isAuthenticated, 'isPasswordSet:', isPasswordSet);

    const renderPage = () => {
        // If we have an OAuth code, show the callback page
        if (oauthCode) {
            return <CodeHostCallback code={oauthCode} />;
        }
        
        switch (page) {
            case 'dashboard':
                return <Dashboard />;
            case 'git':
                return <GitProviders />;
            case 'ai':
                return <AIProviders />;
            case 'settings':
                return <Settings />;
            case 'oauth-callback':
                return <CodeHostCallback />;
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
                onLogout={handleLogout}
            />
            <div className="flex-grow">
                {renderPage()}
            </div>
            <Footer onNavigateToHome={() => setPage('dashboard')} />
        </div>
    );
};

export default App;
