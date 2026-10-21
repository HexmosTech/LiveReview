// Side-effect module: must be imported before the store (and so the Auth reducer, which reads the saved tokens as
// it loads).
//
// The Cloud sign-in returns to "/?data=..." with a fresh Hexmos login. Only the logged-out Cloud login screen
// consumes that payload, so a session that is still saved would hide it and keep the previous user signed in
// (wrong user when someone returns from GitHub as a different account). Dropping the saved tokens here, at the
// moment a new login arrives, is what lets flows leave for GitHub without signing the user out first: if they
// come back without finishing, their session is untouched.
if (typeof window !== 'undefined') {
    try {
        if (new URLSearchParams(window.location.search).has('data')) {
            localStorage.removeItem('accessToken');
            localStorage.removeItem('refreshToken');
        }
    } catch {
        // localStorage unavailable: there is no saved session to drop.
    }
}
