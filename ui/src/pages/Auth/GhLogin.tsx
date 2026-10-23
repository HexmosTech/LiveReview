import React, { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { FcGoogle } from 'react-icons/fc';
import { FaCheck, FaEnvelope, FaLinkedinIn } from 'react-icons/fa6';
import { Icons } from '../../components/UIPrimitives';
import { FullScreenLoader } from '../../components/FullScreenLoader';
import { getPublicUIConfig, githubAppStartUrl } from '../../api/githubApp';
import { isCloudMode } from '../../utils/deploymentMode';

// "Continue with GitHub": one GitHub screen installs the LiveReview GitHub App and signs the user in. This page
// only starts that flow; the server (see /api/v1/auth/github-app/*) does the rest and ends in the normal Cloud
// login. It is shown to logged-out visitors at /gh-login (see App.tsx); logged-in visitors are sent on to
// /get-started/github instead.

const SRC_PATTERN = /^[a-z0-9_-]{1,32}$/;

const POINTS = [
    'Risk scoring decides which PRs are safe to ship without a human',
    'Your team only spends review time on risky and critical changes',
    'Connect GitHub and see it on a real pull request in about 2 minutes',
];

// The other sign-in methods, as brand tiles. Each one opens the normal Hexmos sign-in, which offers all of them.
const OTHER_SIGN_INS = [
    { label: 'Google', icon: <FcGoogle className="h-4 w-4" />, className: 'bg-white hover:bg-gray-200' },
    { label: 'LinkedIn', icon: <FaLinkedinIn className="h-4 w-4 text-white" />, className: 'bg-[#0A66C2] hover:bg-[#004182]' },
    { label: 'Email', icon: <FaEnvelope className="h-4 w-4 text-white" />, className: 'bg-gray-600 hover:bg-gray-500' },
];

const GhLogin: React.FC = () => {
    const location = useLocation();
    const navigate = useNavigate();
    // null = still asking the server whether the GitHub App is configured.
    const [enabled, setEnabled] = useState<boolean | null>(null);

    const src = useMemo(() => {
        const params = new URLSearchParams(location.search);
        const rawSrc = (params.get('src') || '').toLowerCase();
        return SRC_PATTERN.test(rawSrc) ? rawSrc : 'direct';
    }, [location.search]);

    useEffect(() => {
        let cancelled = false;
        getPublicUIConfig()
            .then((config) => {
                if (cancelled) return;
                setEnabled(Boolean(config.githubAppEnabled) && isCloudMode());
            })
            .catch(() => {
                if (!cancelled) setEnabled(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    // Feature off (or not Cloud): behave as if /gh-login did not exist and show the normal login.
    useEffect(() => {
        if (enabled === false) navigate('/', { replace: true });
    }, [enabled, navigate]);

    // Always starts with GitHub sign-in: returning users are recognised from their installations, and the server
    // sends users with no installation on to the install page (see GitHubAppCallback), so no separate
    // "already installed" path is needed here.
    // Nothing is stored before leaving: the callback says where to land (get-started) only when sign-in succeeds,
    // so a failed attempt followed by another sign-in method doesn't end up on get-started.
    const start = () => {
        window.location.href = githubAppStartUrl('signin', src);
    };

    if (enabled === null || enabled === false) {
        return <FullScreenLoader text="Loading LiveReview…" />;
    }

    return (
        <div className="min-h-screen flex items-center justify-center bg-gray-900 px-4 py-10 text-gray-100">
            <div className="w-full max-w-xl">
                <div className="flex justify-center mb-8">
                    <img src="/assets/logo-horizontal.svg" alt="LiveReview" className="h-16 w-auto" />
                </div>
                <div className="bg-gray-800 rounded-lg p-10 shadow-xl shadow-black/20">
                    <h1 className="text-2xl font-bold text-white leading-snug">
                        Auto-approve up to 50% of your pull requests, safely
                    </h1>

                    <ul className="mt-8 space-y-2.5 text-sm text-gray-200 leading-relaxed">
                        {POINTS.map((point) => (
                            <li key={point} className="flex items-start gap-2.5">
                                <FaCheck className="mt-1 h-3.5 w-3.5 flex-shrink-0 text-green-400" aria-hidden="true" />
                                <span>{point}</span>
                            </li>
                        ))}
                    </ul>

                    <div className="mt-8">
                        <button
                            type="button"
                            onClick={start}
                            className="w-full inline-flex items-center justify-center gap-2 rounded-lg bg-black px-5 py-3.5 text-base font-medium text-white transition-colors hover:bg-gray-950 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-2 focus:ring-offset-gray-800"
                        >
                            <Icons.GitHub />
                            Continue with GitHub
                        </button>
                    </div>

                    <div className="mt-6">
                        <p className="text-sm text-gray-400">Or sign in with</p>
                        <div className="mt-2 flex items-center gap-2.5">
                            {OTHER_SIGN_INS.map(({ label, icon, className }) => (
                                <button
                                    key={label}
                                    type="button"
                                    title={label}
                                    aria-label={`Sign in with ${label}`}
                                    onClick={() => navigate('/', { replace: true })}
                                    className={`inline-flex h-9 w-9 items-center justify-center rounded-md transition focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-2 focus:ring-offset-gray-800 ${className}`}
                                >
                                    {icon}
                                </button>
                            ))}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default GhLogin;
