import React, { useEffect, useState } from 'react';
import { HashRouter as Router, Routes, Route, Navigate, useNavigate, useLocation, Link } from 'react-router-dom';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import { DemoModeBanner } from './components/DemoModeBanner';
import { URLMismatchBanner } from './components/URLMismatchBanner';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';
import Settings from './pages/Settings/Settings';
import NewReview from './pages/Reviews/NewReview';
import Login from './pages/Auth/Login';
import Setup from './pages/Setup/Setup';
import CodeHostCallback from './pages/Auth/CodeHostCallback';
import OAuthCallbackHandler from './pages/Auth/OAuthCallbackHandler';
import HomeWithOAuthCheck from './pages/Home/HomeWithOAuthCheck';
import { MiddlewareTestPage } from './pages/MiddlewareTestPage';
import { useAppDispatch, useAppSelector } from './store/configureStore';
import { logout, checkSetupStatus, fetchUser } from './store/Auth/reducer';
import { fetchLicenseStatus, openModal as openLicenseModal, closeModal as closeLicenseModal } from './store/License/slice';
import LicenseModal from './components/License/LicenseModal';
import LicenseStatusBar from './components/License/LicenseStatusBar';
import { Toaster } from 'react-hot-toast';
import UserForm from './components/UserManagement/UserForm';

const Footer = () => (
    <footer className="bg-slate-900 border-t border-slate-700 py-8 mt-auto">
        <div className="container mx-auto px-4 flex flex-col md:flex-row justify-between items-center">
            <div className="flex items-center py-2">
                <Link to="/" className="cursor-pointer" aria-label="Go to home">
                    <img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-10 w-auto" />
                </Link>
            </div>
            <div className="text-right mt-4 md:mt-0">
                <p className="text-sm text-slate-200">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
            </div>
        </div>
    </footer>
);

// Main application content with routing
const AppContent: React.FC = () => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();
    const location = useLocation();
    const { isAuthenticated, isSetupRequired, isLoading, accessToken } = useAppSelector((state) => state.Auth);
    const licenseStatus = useAppSelector(s => s.License.status);
    const licenseOpen = useAppSelector(s => s.License.modalOpen);
    const licenseLoadedOnce = useAppSelector(s => s.License.loadedOnce);
    
    // Extract the current page from the path
    const getCurrentPage = (): string => {
        const path = location.pathname;
        if (path.startsWith('/dashboard')) return 'dashboard';
        if (path.startsWith('/git')) return 'git';
        if (path.startsWith('/ai')) return 'ai';
        if (path.startsWith('/settings')) return 'settings';
        return 'dashboard';
    };
    
    const [activePage, setActivePage] = useState(getCurrentPage());
    
    // Update active page when location changes
    useEffect(() => {
        setActivePage(getCurrentPage());
    }, [location]);

    // Check setup status or fetch user data on app load
    useEffect(() => {
        if (accessToken) {
            // If we have a token, fetch user data to validate the session
            dispatch(fetchUser());
        } else {
            // Otherwise, check if the initial setup is required
            dispatch(checkSetupStatus());
        }
    }, [dispatch, accessToken]);

    // Kick off initial license status load (non-blocking UI)
    useEffect(() => {
        // Only attempt after authentication established to avoid 401 noise
        if (isAuthenticated) {
            dispatch(fetchLicenseStatus());
        }
    }, [dispatch, isAuthenticated]);
    
    // Debug listener for Auth state changes
    useEffect(() => {
        console.log('Auth state changed - isAuthenticated:', isAuthenticated, 'isSetupRequired:', isSetupRequired);
    }, [isAuthenticated, isSetupRequired]);

    // Handle navigation
    const handleNavigate = (page: string) => {
        navigate(`/${page}`);
    };

    // Handle logout
    const handleLogout = async () => {
        try {
            await dispatch(logout()).unwrap();
        } catch (error) {
            // Logout should never really fail in our implementation
            console.warn('Logout completed with warning:', error);
        }
        // After logout, check the setup status to determine which page to show
        dispatch(checkSetupStatus());
    };

    // Enforce license: open when status requires token, but ONLY after initial load to avoid flash
    useEffect(() => {
        if (!isAuthenticated) {
            dispatch(closeLicenseModal());
            return;
        }
        if (!licenseLoadedOnce) {
            // Avoid opening modal until we know the real status
            return;
        }
        if (['missing','invalid','expired'].includes(licenseStatus)) {
            dispatch(openLicenseModal());
        } else {
            dispatch(closeLicenseModal());
        }
    }, [isAuthenticated, licenseStatus, licenseLoadedOnce, dispatch]);

    // (Removed old keyboard shortcut & placeholder strict effect to prevent events firing after unmount)

    // Decide what to render based on auth/setup states AFTER all hooks declared (avoid hook order issues)
    let body: React.ReactNode;
    if (isLoading) {
        body = (
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
    } else if (isSetupRequired) {
        body = <Setup />;
    } else if (!isAuthenticated) {
        body = <Login />;
    } else {
        // Subtle fade-in for main content to make initial paint feel smoother
        const [uiReady, setUiReady] = useState(false);
        useEffect(() => {
            const id = requestAnimationFrame(() => setUiReady(true));
            return () => cancelAnimationFrame(id);
        }, []);
        body = (
            <div className={`min-h-screen flex flex-col transition-opacity duration-200 ${uiReady ? 'opacity-100' : 'opacity-0'}`}> 
                <Navbar
                    title="LiveReview"
                    activePage={activePage}
                    onNavigate={handleNavigate}
                    onLogout={handleLogout}
                />
                {/* DemoModeBanner kept for compatibility; now mostly replaced by status bar badge */}
                {/* <DemoModeBanner /> */}
                <URLMismatchBanner />
                <LicenseStatusBar onOpenModal={() => dispatch(openLicenseModal())} />
                <div className="flex-grow">
                    <Routes>
                        <Route path="/" element={<HomeWithOAuthCheck />} />
                        <Route path="/dashboard" element={<Dashboard />} />
                        <Route path="/git/*" element={<GitProviders />} />
                        <Route path="/ai" element={<AIProviders />} />
                        <Route path="/ai/:provider" element={<AIProviders />} />
                        <Route path="/ai/:provider/:action" element={<AIProviders />} />
                        <Route path="/ai/:provider/:action/:connectorId" element={<AIProviders />} />
                        <Route path="/settings/*" element={<Settings />} />
                        <Route path="/settings/users/add" element={<UserForm />} />
                        <Route path="/settings/users/edit/:userId" element={<UserForm />} />
                        <Route path="/reviews/new" element={<NewReview />} />
                        <Route path="/test-middleware" element={<MiddlewareTestPage />} />
                        <Route path="/oauth-callback" element={<OAuthCallbackHandler />} />
                        <Route path="*" element={<Navigate to="/" replace />} />
                    </Routes>
                </div>
                <Footer />
                <LicenseModal open={licenseOpen} onClose={() => dispatch(closeLicenseModal())} strictMode={['missing','invalid','expired'].includes(licenseStatus)} />
            </div>
        );
    }

    return <>{body}</>;
};

// Main App component with Router
const App: React.FC = () => {
    // Check if we have OAuth parameters in the URL (for GitLab redirect)
    // This runs before the router setup
    React.useEffect(() => {
        const handleOAuthRedirect = () => {
            // Get all URL parameters
            const urlParams = new URLSearchParams(window.location.search);
            const code = urlParams.get('code');
            const error = urlParams.get('error');
            const state = urlParams.get('state');
            
            console.log("Checking for OAuth parameters in URL:", { 
                code: code ? "present" : "absent", 
                error: error ? "present" : "absent", 
                state: state ? "present" : "absent",
                fullUrl: window.location.href
            });
            
            // If we have OAuth parameters and we're at the root URL
            if ((code || error) && window.location.hash === '') {
                console.log("Detected OAuth redirect parameters:", { code, error, state });
                
                // Check if there's a redirect overlay from previous navigation and remove it
                const overlay = document.getElementById('gitlab-redirect-overlay');
                if (overlay) {
                    console.log("Removing gitlab-redirect-overlay");
                    overlay.remove();
                }
                
                // Store OAuth parameters in sessionStorage
                if (code) sessionStorage.setItem('oauth_code', code);
                if (error) sessionStorage.setItem('oauth_error', error);
                if (state) sessionStorage.setItem('oauth_state', state);
                
                // Redirect to the OAuth callback route with clean URL
                console.log("Redirecting to OAuth callback route");
                window.location.href = '/#/oauth-callback';
            }
        };

        handleOAuthRedirect();
    }, []);

    return (
        <Router>
            <AppContent />
            <Toaster />
        </Router>
    );
};

export default App;
