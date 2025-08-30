import React, { useEffect } from 'react';
import { useSearchParams, useNavigate, useLocation } from 'react-router-dom';
import CodeHostCallback from './CodeHostCallback';

const OAuthCallbackHandler: React.FC = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const location = useLocation();
  
  // Get code/error from search params
  const code = searchParams.get('code');
  const error = searchParams.get('error');
  const state = searchParams.get('state');

  useEffect(() => {
    console.log("OAuthCallbackHandler mounted with parameters:", { code, error, state });
    
    // Check URL parameters first
    if (window.location.search) {
      console.log("Found URL search parameters:", window.location.search);
    }
    
    // If we have code or error parameters, clean up the URL by removing them
    if (code || error) {
      console.log("Processing OAuth parameters from URL:", { code, error, state });
      
      // First, store the code/error in sessionStorage so we can access it after URL cleanup
      if (code) sessionStorage.setItem('oauth_code', code);
      if (error) sessionStorage.setItem('oauth_error', error);
      if (state) sessionStorage.setItem('oauth_state', state);
      
      // Replace the URL with a clean version (without query parameters)
      navigate('/oauth-callback', { replace: true });
    }
    
    // If no code in search params, check if we have stored values in sessionStorage
    if (!code && !error) {
      const storedCode = sessionStorage.getItem('oauth_code');
      const storedError = sessionStorage.getItem('oauth_error');
      const storedState = sessionStorage.getItem('oauth_state');
      
      // If we have stored values, use them and then clear storage
      if (storedCode || storedError) {
        console.log("Using stored OAuth parameters:", { code: storedCode, error: storedError, state: storedState });
        
        // The CodeHostCallback component will be rendered with these values
        // Clean up sessionStorage after processing
        setTimeout(() => {
          sessionStorage.removeItem('oauth_code');
          sessionStorage.removeItem('oauth_error');
          sessionStorage.removeItem('oauth_state');
        }, 100);
      } else {
        console.log("No OAuth parameters found in URL or sessionStorage");
      }
    }
  }, [code, error, state, navigate, location]);

  // Get the code/error either from URL params or sessionStorage
  const finalCode = code || sessionStorage.getItem('oauth_code');
  const finalError = error || sessionStorage.getItem('oauth_error');
  const finalState = state || sessionStorage.getItem('oauth_state');

  // When we have a code, show the CodeHostCallback component
  if (finalCode || finalError) {
    return <CodeHostCallback 
      code={finalCode || undefined} 
      error={finalError || undefined} 
      state={finalState || undefined} 
    />;
  }

  // Show a simple loading/processing screen when no code/error found
  return (
    <div className="container mx-auto px-4 py-8">
      <div className="bg-slate-800 p-6 rounded-lg max-w-3xl mx-auto">
        <div className="flex flex-col items-center justify-center py-12">
          <svg className="animate-spin h-10 w-10 text-blue-500 mb-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <h2 className="text-xl font-medium text-white">
            Waiting for authentication data...
          </h2>
          <p className="text-slate-400 text-sm mt-2">
            If you've been redirected from GitLab but see this screen, please check your browser's URL and make sure it contains the authorization code.
          </p>
          <div className="mt-4">
            <button 
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-md text-white font-medium"
              onClick={() => {
                // Try to check URL parameters manually
                const urlParams = new URLSearchParams(window.location.search);
                const urlCode = urlParams.get('code');
                const urlError = urlParams.get('error');
                const urlState = urlParams.get('state');
                
                if (urlCode || urlError) {
                  console.log("Found URL parameters in manual check:", { urlCode, urlError, urlState });
                  
                  // Store them and reload
                  if (urlCode) sessionStorage.setItem('oauth_code', urlCode);
                  if (urlError) sessionStorage.setItem('oauth_error', urlError);
                  if (urlState) sessionStorage.setItem('oauth_state', urlState);
                  
                  // Refresh the page to try again
                  window.location.reload();
                } else {
                  console.log("No URL parameters found in manual check");
                  // Go back to the Git Providers page
                  navigate('/git');
                }
              }}
            >
              Try Again
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default OAuthCallbackHandler;
