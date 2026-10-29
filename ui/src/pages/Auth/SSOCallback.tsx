import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAppDispatch } from '../../store/configureStore';
import { setTokens } from '../../store/Auth/reducer';
import { notify } from '../../utils/notify';

/**
 * Lands here after a successful OIDC/SAML login - the backend's
 * completeSSOLogin redirects the browser to
 * /sso/callback#access_token=...&refresh_token=...&expires_at=...&token_type=...
 * (a URL fragment, not a query string, so the tokens are never sent to or
 * logged by any server). This mirrors the normal password-login flow by
 * dispatching the same setTokens action, which flips Auth.isAuthenticated,
 * then hands off to the dashboard like any other login.
 */
const SSOCallback: React.FC = () => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();

    useEffect(() => {
        const params = new URLSearchParams(window.location.hash.replace(/^#/, ''));
        const access_token = params.get('access_token');
        const refresh_token = params.get('refresh_token');
        const expires_at = params.get('expires_at') || '';
        const token_type = params.get('token_type') || 'Bearer';

        if (!access_token || !refresh_token) {
            notify.error('Single sign-on login failed: no tokens received');
            navigate('/?sso_error=missing_tokens', { replace: true });
            return;
        }

        dispatch(setTokens({ access_token, refresh_token, expires_at, token_type }));
        window.history.replaceState(null, '', '/dashboard');
        notify.success('Login successful!');
        navigate('/dashboard', { replace: true });
    }, [dispatch, navigate]);

    return (
        <div className="min-h-screen bg-gray-900 flex items-center justify-center">
            <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
        </div>
    );
};

export default SSOCallback;
