import React, { useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import CodeHostCallback from './CodeHostCallback';

const OAuthCallbackHandler: React.FC = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  
  // Get code/error from search params
  const code = searchParams.get('code');
  const error = searchParams.get('error');

  useEffect(() => {
    console.log("OAuthCallbackHandler mounted with parameters:", { code, error });
    
    // If no code in search params, check window.location.search directly
    // This is needed because React Router HashRouter might not correctly parse
    // params when redirecting from the OAuth provider
    if (!code && !error) {
      const urlParams = new URLSearchParams(window.location.search);
      const urlCode = urlParams.get('code');
      const urlError = urlParams.get('error');
      
      if (urlCode || urlError) {
        console.log("Found OAuth parameters in URL:", { code: urlCode, error: urlError });
        // Just render the CodeHostCallback component directly with the params
        return;
      }
    }
  }, [code, error]);

  // When we have a code, show the CodeHostCallback component
  if (code || error) {
    return <CodeHostCallback code={code || undefined} error={error || undefined} />;
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
        </div>
      </div>
    </div>
  );
};

export default OAuthCallbackHandler;
