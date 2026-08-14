import React, { Suspense, useEffect, useState } from 'react';
import { HashRouter as Router, Routes, Route, Navigate, useNavigate, useLocation, Link } from 'react-router-dom';
import { Navbar } from './components/Navbar/Navbar';
import { URLMismatchBanner } from './components/URLMismatchBanner';
import { useAppDispatch, useAppSelector } from './store/configureStore';
import { logout, checkSetupStatus, fetchUser } from './store/Auth/reducer';
import { fetchLicenseStatus, openModal as openLicenseModal, closeModal as closeLicenseModal } from './store/License/slice';
import LicenseModal from './components/License/LicenseModal';
import LicenseStatusBar from './components/License/LicenseStatusBar';
import { isCloudMode } from './utils/deploymentMode';
import { SubscriptionGuard } from './components/SubscriptionGuard';
import { Toaster } from 'react-hot-toast';
import { useBottomRightBlockers } from './store/uiLayout';
import { ToastBridge } from './components/Notifications/ToastBridge';

const Dashboard = React.lazy(() => import('./components/Dashboard/Dashboard').then((m) => ({ default: m.Dashboard })));
const GitProviders = React.lazy(() => import('./pages/GitProviders/GitProviders'));
const AIProviders = React.lazy(() => import('./pages/AIProviders/AIProviders'));
const Settings = React.lazy(() => import('./pages/Settings/Settings'));
const ReviewsRoutes = React.lazy(() => import('./pages/Reviews/ReviewsRoutes'));
const ExploreRoutes = React.lazy(() => import('./pages/Explore/ExploreRoutes'));
const Login = React.lazy(() => import('./pages/Auth/Login'));
const SelfHosted = React.lazy(() => import('./pages/Auth/SelfHosted'));
const Setup = React.lazy(() => import('./pages/Setup/Setup'));
const CodeHostCallback = React.lazy(() => import('./pages/Auth/CodeHostCallback'));
const OAuthCallbackHandler = React.lazy(() => import('./pages/Auth/OAuthCallbackHandler'));
const HomeWithOAuthCheck = React.lazy(() => import('./pages/Home/HomeWithOAuthCheck'));
const MiddlewareTestPage = React.lazy(() => import('./pages/MiddlewareTestPage').then((m) => ({ default: m.MiddlewareTestPage })));
const Subscribe = React.lazy(() => import('./pages/Subscribe/Subscribe'));
const TeamCheckout = React.lazy(() => import('./pages/Checkout/TeamCheckout'));
const LicenseManagement = React.lazy(() => import('./pages/Licenses/LicenseManagement'));
const LicenseAssignment = React.lazy(() => import('./pages/Licenses/LicenseAssignment'));
const UserForm = React.lazy(() => import('./components/UserManagement/UserForm'));
const BillingPortfolio = React.lazy(() => import('./pages/Admin/BillingPortfolio'));
const TaxonomyReports = React.lazy(() => import('./pages/Reports/TaxonomyReports'));
const Chatbot = React.lazy(() => import('./pages/Chatbot/Chatbot'));
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
                            className="flex items-center space-x-2 bg-slate-800/70 hover:bg-slate-700/60 text-slate-300 hover:text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-slate-700"
                        >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
                            </svg>
                            <span>Documentation</span>
                        </a>
                        <a
                            href="https://github.com/HexmosTech/LiveReview/issues"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center space-x-2 bg-slate-800/70 hover:bg-slate-700/60 text-slate-300 hover:text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-slate-700"
                        >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008v.008H12v-.008zM21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                            </svg>
                            <span>Report Issue</span>
                        </a>
                        <a
                            href="https://github.com/HexmosTech/LiveReview/discussions"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center space-x-2 bg-slate-800/70 hover:bg-slate-700/60 text-slate-300 hover:text-white px-4 py-2 rounded-lg transition-colors font-medium text-sm border border-slate-700"
                        >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.456-2.456L14.25 6l1.035-.259a3.375 3.375 0 002.456-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456z" />
                            </svg>
                            <span>Suggest Improvement</span>
                        </a>
                    </div>
                    <p className="text-sm text-slate-200">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
                </div>
            </div>
        </div>
    </footer>
);

const RouteFallback = () => (
    <div className="min-h-screen flex items-center justify-center bg-slate-900 text-slate-100">
        <div className="flex items-center gap-3">
            <div className="h-10 w-10 rounded-full border-2 border-indigo-500 border-t-transparent animate-spin" aria-hidden />
            <span>Loading…</span>
        </div>
    </div>
);

const BootScreen: React.FC<{ visible: boolean }> = ({ visible }) => (
    <div
        className={`fixed inset-0 z-40 flex items-center justify-center bg-slate-950 transition-opacity duration-200 ${visible ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none'}`}
        aria-busy={visible}
    >
        <div className="flex flex-col items-center gap-4">
            <img src="/assets/logo-horizontal.svg" alt="LiveReview" className="h-12 w-auto" width="240" height="64" loading="eager" />
            <div className="h-10 w-10 rounded-full border-2 border-indigo-500 border-t-transparent animate-spin" aria-hidden />
        </div>
    </div>
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
    const blockers = useBottomRightBlockers();
    const nudgeOccupying = (blockers & 1) !== 0;
    const commentNavOccupying = (blockers & 2) !== 0;
    // Subtle fade-in for main content to make initial paint feel smoother
    const [uiReady, setUiReady] = useState(false);
    const [bootVisible, setBootVisible] = useState(true);
    useEffect(() => {
        console.info('[LiveReview][AppContent] mounted');
        const id = requestAnimationFrame(() => setUiReady(true));
        return () => {
            cancelAnimationFrame(id);
            console.info('[LiveReview][AppContent] unmounted');
        };
    }, []);

    // Hide boot overlay once we have auth state resolved or after a short timeout
    useEffect(() => {
        if (!bootVisible) return;
        const timeout = setTimeout(() => setBootVisible(false), isLoading ? 1200 : 150);
        if (!isLoading) {
            setBootVisible(false);
        }
        return () => clearTimeout(timeout);
    }, [bootVisible, isLoading]);

    // CRITICAL: Capture the intended destination URL immediately on mount, before any navigation occurs
    // This runs once when the component mounts to preserve the user's originally requested URL
    useEffect(() => {
        // Only run once on mount
        const hashPath = window.location.hash;
        const isProtectedRoute = 
            hashPath && 
            hashPath !== '' && 
            hashPath !== '#/' && 
            !hashPath.startsWith('#/oauth-callback') && 
            !hashPath.startsWith('#/admin');
        
        if (isProtectedRoute && !isAuthenticated) {
            // Store immediately before any other code can change the hash
            sessionStorage.setItem('redirectAfterLogin', hashPath);
            console.info('[LiveReview][Mount] Captured initial redirect URL:', hashPath);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []); // Run only once on mount

    // Extract the current page from the path
    const getCurrentPage = (): string => {
        const path = location.pathname;
        if (path.startsWith('/dashboard')) return 'dashboard';
        if (path.startsWith('/reviews')) return 'reviews';
        if (path.startsWith('/explore')) return 'explore';
        if (path.startsWith('/git') || path.startsWith('/ai')) return 'providers';
        if (path.startsWith('/admin/billing-portfolio')) return 'admin-billing';
        if (path.startsWith('/reports')) return 'reports';
        if (path.startsWith('/chat')) return 'chat';
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
    const handleNavigate = (target: string) => {
        if (target.startsWith('/')) {
            navigate(target);
            return;
        }
        navigate(`/${target}`);
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
    // License modal should NOT appear in cloud mode (only for self-hosted)
    // Also, don't auto-open modal for missing license - allow general usage without a license
    // Users can still open the modal manually via LicenseStatusBar to add a license
    // Feature-specific restrictions are handled by LicenseUpgradeDialog
    useEffect(() => {
        // Skip license modal entirely in cloud mode
        if (isCloudMode()) {
            dispatch(closeLicenseModal());
            return;
        }
        
        if (!isAuthenticated) {
            dispatch(closeLicenseModal());
            return;
        }
        // Don't auto-open modal - let users use the app freely
        // They can manually open it via LicenseStatusBar if they want to add a license
    }, [isAuthenticated, dispatch]);

    // Ctrl+I opens the chatbot from anywhere in the app.
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'i') {
                e.preventDefault();
                navigate('/chat');
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [navigate]);

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
        // Allow email/admin login screen only when explicitly visiting /admin in cloud
        // (needed for initial setup or troubleshooting); otherwise use cloud login.
        if (isCloudMode() && location.pathname === '/admin') {
            body = <SelfHosted />;
        } else {
            body = <Login />;
        }
    } else {
        body = (
            <div className={`min-h-screen flex flex-col transition-opacity duration-200 ${uiReady ? 'opacity-100' : 'opacity-0'}`}>
                <Navbar
                    title="LiveReview"
                    activePage={activePage}
                    onNavigate={handleNavigate}
                    onLogout={handleLogout}
                />
                <URLMismatchBanner />
                {!isCloudMode() && location.pathname !== '/chat' && <LicenseStatusBar onOpenModal={() => dispatch(openLicenseModal())} />}
                <div className="flex-grow">
                    <SubscriptionGuard>
                        <Suspense fallback={<RouteFallback />}>
                            <Routes>
                                <Route path="/" element={<HomeWithOAuthCheck />} />
                                <Route path="/dashboard" element={<Dashboard />} />
                                <Route path="/subscribe" element={<Subscribe />} />
                                <Route path="/subscribe/manage" element={<LicenseManagement />} />
                                <Route path="/subscribe/subscriptions/:id/assign" element={<LicenseAssignment />} />
                                <Route path="/checkout/team" element={<TeamCheckout />} />
                                <Route path="/reviews/*" element={<ReviewsRoutes />} />
                                <Route path="/explore/*" element={<ExploreRoutes />} />
                                <Route path="/git/*" element={<GitProviders />} />
                                <Route path="/ai" element={<AIProviders />} />
                                <Route path="/ai/:provider" element={<AIProviders />} />
                                <Route path="/ai/:provider/:action" element={<AIProviders />} />
                                <Route path="/ai/:provider/:action/:connectorId" element={<AIProviders />} />
                                <Route path="/settings/*" element={<Settings />} />
                                <Route path="/settings-subscriptions-overview" element={<Settings />} />
                                <Route path="/settings-subscriptions-breakdown" element={<Settings />} />
                                <Route path="/settings-subscriptions-assign" element={<Settings />} />
                                <Route path="/settings-subscriptions-portfolio" element={<Settings />} />
                                <Route path="/settings/users/add" element={<UserForm />} />
                                <Route path="/settings/users/add/bulk" element={<UserForm />} />
                                <Route path="/settings/users/edit/:userId" element={<UserForm />} />
                                <Route path="/admin/billing-portfolio" element={<BillingPortfolio />} />
                                <Route path="/reports/*" element={<TaxonomyReports />} />
                                <Route path="/chat" element={<Chatbot />} />
                                <Route path="/test-middleware" element={<MiddlewareTestPage />} />
                                <Route path="/oauth-callback" element={<OAuthCallbackHandler />} />
                                <Route path="*" element={<Navigate to="/" replace />} />
                            </Routes>
                        </Suspense>
                    </SubscriptionGuard>
                </div>
                {location.pathname !== '/chat' && <Footer />}
                {!isCloudMode() && <LicenseModal open={licenseOpen} onClose={() => dispatch(closeLicenseModal())} />}
                {location.pathname !== '/chat' && (
                <div className={`fixed right-6 z-50 flex items-center gap-3 transition-all duration-300 ${nudgeOccupying ? 'bottom-20' : 'bottom-6'}`}>
                    {!commentNavOccupying && (
                    <span className="hidden lg:flex items-center gap-2 text-sm font-semibold text-slate-100 px-4 py-2 rounded-full bg-slate-800/90 border border-slate-600 shadow-lg pointer-events-none">
                        Chat with Livi
                        <span className="text-[11px] font-semibold text-slate-100 bg-slate-700 px-1.5 py-0.5 rounded">Ctrl + I</span>
                    </span>
                    )}
                    <button
                        onClick={() => navigate('/chat')}
                        className="relative w-14 h-14 bg-indigo-600 hover:bg-indigo-700 text-white rounded-full shadow-lg flex items-center justify-center"
                        title="LivereviewBot"
                        aria-label="Open LivereviewBot Chat"
                    >
                        <span className="absolute top-1 right-1 w-2.5 h-2.5 rounded-full bg-emerald-400 border-2 border-white" aria-hidden="true" />
                        <svg className="w-7 h-7" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
                        </svg>
                    </button>
                </div>
                )}
            </div>
        );
    }

    return (
        <>
            <BootScreen visible={bootVisible} />
            {body}
        </>
    );
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
            <ToastBridge />
        </Router>
    );
};

export default App;
