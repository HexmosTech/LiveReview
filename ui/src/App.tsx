import React, { useEffect, useState } from 'react';
import { HashRouter as Router, Routes, Route, Navigate, useNavigate, useLocation, Link } from 'react-router-dom';
import { Navbar } from './components/Navbar/Navbar';
import { Dashboard } from './components/Dashboard/Dashboard';
import { DemoModeBanner } from './components/DemoModeBanner';
import { URLMismatchBanner } from './components/URLMismatchBanner';
import GitProviders from './pages/GitProviders/GitProviders';
import AIProviders from './pages/AIProviders/AIProviders';
import Settings from './pages/Settings/Settings';
import ReviewsRoutes from './pages/Reviews/ReviewsRoutes';
import Login from './pages/Auth/Login';
import SelfHosted from './pages/Auth/SelfHosted';
import Setup from './pages/Setup/Setup';
import CodeHostCallback from './pages/Auth/CodeHostCallback';
import OAuthCallbackHandler from './pages/Auth/OAuthCallbackHandler';
import HomeWithOAuthCheck from './pages/Home/HomeWithOAuthCheck';
import { MiddlewareTestPage } from './pages/MiddlewareTestPage';
import Subscribe from './pages/Subscribe/Subscribe';
import TeamCheckout from './pages/Checkout/TeamCheckout';
import LicenseManagement from './pages/Licenses/LicenseManagement';
import LicenseAssignment from './pages/Licenses/LicenseAssignment';
import { useAppDispatch, useAppSelector } from './store/configureStore';
import { logout, checkSetupStatus, fetchUser } from './store/Auth/reducer';
import { fetchLicenseStatus, openModal as openLicenseModal, closeModal as closeLicenseModal } from './store/License/slice';
import LicenseModal from './components/License/LicenseModal';
import LicenseStatusBar from './components/License/LicenseStatusBar';
import { Toaster } from 'react-hot-toast';
import UserForm from './components/UserManagement/UserForm';
// import { usePostHog } from '@posthog/react'

const Footer = () => (
    <footer className="bg-slate-900 border-t border-slate-700 py-8 mt-auto">
        <div className="container mx-auto px-4">
            <div className="flex flex-col md:flex-row justify-between items-center space-y-4 md:space-y-0">
                <div className="flex items-center">
                    <Link to="/" className="cursor-pointer" aria-label="Go to home">
                        <img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-10 w-auto" />
                    </Link>
                </div>
                <div className="flex flex-col items-end space-y-3">
                    <div className="flex flex-wrap gap-3 justify-end">
                        <a
                            href="https://github.com/HexmosTech/LiveReview/wiki"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center space-x-2 bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-blue-500"
                        >
                            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                                <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                            </svg>
                            <span>📚 Documentation</span>
                        </a>
                        <a
                            href="https://github.com/HexmosTech/LiveReview/issues"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center space-x-2 bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-orange-500"
                        >
                            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                                <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                            </svg>
                            <span>🐛 Report Issue</span>
                        </a>
                        <a
                            href="https://github.com/HexmosTech/LiveReview/discussions"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center space-x-2 bg-emerald-600 hover:bg-emerald-700 text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-emerald-500"
                        >
                            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                                <path fillRule="evenodd" d="M18 13V5a2 2 0 00-2-2H4a2 2 0 00-2 2v8a2 2 0 002 2h3l3 3 3-3h3a2 2 0 002-2zM5 7a1 1 0 011-1h8a1 1 0 110 2H6a1 1 0 01-1-1zm1 3a1 1 0 100 2h3a1 1 0 100-2H6z" clipRule="evenodd" />
                            </svg>
                            <span>💡 Suggest Improvement</span>
                        </a>
                    </div>
                    <p className="text-sm text-slate-200">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
                </div>
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
    // Subtle fade-in for main content to make initial paint feel smoother
    const [uiReady, setUiReady] = useState(false);
    useEffect(() => {
        console.info('[LiveReview][AppContent] mounted');
        const id = requestAnimationFrame(() => setUiReady(true));
        return () => {
            cancelAnimationFrame(id);
            console.info('[LiveReview][AppContent] unmounted');
        };
    }, []);

    // Extract the current page from the path
    const getCurrentPage = (): string => {
        const path = location.pathname;
        if (path.startsWith('/dashboard')) return 'dashboard';
        if (path.startsWith('/reviews')) return 'reviews';
        if (path.startsWith('/git')) return 'git';
        if (path.startsWith('/ai')) return 'ai';
        if (path.startsWith('/settings')) return 'settings';
        return 'dashboard';
    };

    const [activePage, setActivePage] = useState(getCurrentPage());

    // Update active page when location changes
    useEffect(() => {
        const nextPage = getCurrentPage();
        console.info('[LiveReview][AppContent] location changed', {
            pathname: location.pathname,
            hash: location.hash,
            search: location.search,
            nextPage,
        });
        setActivePage(nextPage);
    }, [location]);

    // Redirect from /admin to dashboard when authenticated
    useEffect(() => {
        if (isAuthenticated && location.pathname === '/admin') {
            // Clean URL and navigate smoothly to dashboard
            window.history.replaceState(null, '', '/');
            navigate('/dashboard', { replace: true });
        }
    }, [isAuthenticated, location.pathname, navigate]);

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
        console.info('[LiveReview][Auth]', {
            isAuthenticated,
            isSetupRequired,
            isLoading,
        });
    }, [isAuthenticated, isSetupRequired, isLoading]);

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
        // Reset URL to base path
        navigate('/');
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
        if (['missing', 'invalid', 'expired'].includes(licenseStatus)) {
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
        body = location.pathname === '/admin' ? <SelfHosted /> : <Login />;
    } else {
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
                        <Route path="/subscribe" element={<Subscribe />} />
                        <Route path="/subscribe/manage" element={<LicenseManagement />} />
                        <Route path="/subscribe/subscriptions/:id/assign" element={<LicenseAssignment />} />
                        <Route path="/checkout/team" element={<TeamCheckout />} />
                        <Route path="/reviews/*" element={<ReviewsRoutes />} />
                        <Route path="/git/*" element={<GitProviders />} />
                        <Route path="/ai" element={<AIProviders />} />
                        <Route path="/ai/:provider" element={<AIProviders />} />
                        <Route path="/ai/:provider/:action" element={<AIProviders />} />
                        <Route path="/ai/:provider/:action/:connectorId" element={<AIProviders />} />
                        <Route path="/settings/*" element={<Settings />} />
                        <Route path="/settings/users/add" element={<UserForm />} />
                        <Route path="/settings/users/edit/:userId" element={<UserForm />} />
                        <Route path="/test-middleware" element={<MiddlewareTestPage />} />
                        <Route path="/oauth-callback" element={<OAuthCallbackHandler />} />
                        <Route path="*" element={<Navigate to="/" replace />} />
                    </Routes>
                </div>
                <Footer />
                <LicenseModal open={licenseOpen} onClose={() => dispatch(closeLicenseModal())} strictMode={['missing', 'invalid', 'expired'].includes(licenseStatus)} />
            </div>
        );
    }

    return <>{body}</>;
};

// Main App component with Router
const App: React.FC = () => {
    // const posthog = usePostHog()
    useEffect(() => {
        console.info('[LiveReview][App] mounted');
        return () => {
            console.info('[LiveReview][App] unmounted');
        };
    }, []);

    // useEffect(() => {
    //     console.info('[LiveReview][App] posthog hook updated', {
    //         hasPosthog: Boolean(posthog),
    //     });
    // }, [posthog]);
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
