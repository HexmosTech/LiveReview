import React, { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import toast from 'react-hot-toast';

const MCPAuth: React.FC = () => {
    const [searchParams] = useSearchParams();
    const navigate = useNavigate();
    const requestId = searchParams.get('id');
    const { isAuthenticated, accessToken, refreshToken } = useAppSelector((state) => state.Auth);
    const [status, setStatus] = useState<'loading' | 'submitting' | 'success' | 'error'>('loading');
    const [errorMessage, setErrorMessage] = useState<string | null>(null);

    useEffect(() => {
        if (!requestId) {
            setErrorMessage('Missing request ID');
            setStatus('error');
            return;
        }

        if (!isAuthenticated) {
            // Store redirect URL and go to login
            // We use the full hash path for HashRouter compatibility
            const currentPath = window.location.hash.slice(1); // remove #
            sessionStorage.setItem('redirectAfterLogin', currentPath);
            navigate('/login');
            return;
        }

        const completeAuth = async () => {
            setStatus('submitting');
            try {
                const response = await fetch(`/api/v1/auth/mcp/complete?id=${requestId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        Authorization: `Bearer ${accessToken}`,
                    },
                    body: JSON.stringify({
                        access_token: accessToken,
                        refresh_token: refreshToken,
                        token_type: 'Bearer',
                    }),
                });

                if (response.ok) {
                    setStatus('success');
                    toast.success('MCP Authentication successful!');
                } else {
                    const data = await response.json();
                    setErrorMessage(data.error || 'Failed to complete authentication');
                    setStatus('error');
                }
            } catch (error) {
                setErrorMessage('An unexpected error occurred');
                setStatus('error');
                console.error('MCP auth error:', error);
            }
        };

        completeAuth();
    }, [requestId, isAuthenticated, accessToken, refreshToken, navigate]);

    return (
        <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
            <div className="max-w-md w-full bg-slate-900 border border-slate-800 rounded-2xl p-8 shadow-2xl backdrop-blur-xl bg-opacity-80">
                <div className="text-center mb-8">
                    <img src="/assets/logo-horizontal.svg" alt="LiveReview" className="h-12 mx-auto mb-4" />
                    <h2 className="text-2xl font-bold text-white">MCP Authentication</h2>
                    <p className="text-slate-400 mt-2">Authorizing your Model Context Protocol server</p>
                </div>

                {status === 'loading' || status === 'submitting' ? (
                    <div className="flex flex-col items-center py-8">
                        <div className="w-12 h-12 border-4 border-indigo-500 border-t-transparent rounded-full animate-spin mb-4"></div>
                        <p className="text-slate-300">Completing authorization...</p>
                    </div>
                ) : status === 'success' ? (
                    <div className="text-center py-8">
                        <div className="w-16 h-16 bg-emerald-500/20 text-emerald-500 rounded-full flex items-center justify-center mx-auto mb-4">
                            <svg className="w-8 h-8" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                            </svg>
                        </div>
                        <h3 className="text-xl font-semibold text-white mb-2">Success!</h3>
                        <p className="text-slate-400">Your MCP server is now authorized. You can close this window and return to your AI client.</p>
                        <button 
                            onClick={() => navigate('/dashboard')}
                            className="mt-8 w-full py-3 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-medium transition-colors"
                        >
                            Back to Dashboard
                        </button>
                    </div>
                ) : (
                    <div className="text-center py-8">
                        <div className="w-16 h-16 bg-rose-500/20 text-rose-500 rounded-full flex items-center justify-center mx-auto mb-4">
                            <svg className="w-8 h-8" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        </div>
                        <h3 className="text-xl font-semibold text-white mb-2">Authorization Failed</h3>
                        <p className="text-slate-400">{errorMessage}</p>
                        <button 
                            onClick={() => window.location.reload()}
                            className="mt-8 w-full py-3 bg-slate-800 hover:bg-slate-700 text-white rounded-xl font-medium transition-colors"
                        >
                            Try Again
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
};

export default MCPAuth;
