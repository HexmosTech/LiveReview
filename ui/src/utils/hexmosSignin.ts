// The Hexmos sign-in page (Google, LinkedIn, email, ...). After sign-in it returns to `returnUrl` with ?data=, which
// the Cloud login page (pages/Auth/Cloud.tsx) turns into a LiveReview session. On-prem Hexmos serves its own copy.
export const hexmosSigninUrl = (returnUrl: string): string => {
    const hostEnv = (window as any).__ENV__?.VITE_HOST_ENV;
    if (hostEnv === 'onprem') {
        return `/signin/auth/index.html?preselectProvider=authentik&app=livereview&appRedirectURI=${encodeURIComponent(returnUrl)}`;
    }
    return `https://hexmos.com/signin?app=livereview&appRedirectURI=${encodeURIComponent(returnUrl)}`;
};
