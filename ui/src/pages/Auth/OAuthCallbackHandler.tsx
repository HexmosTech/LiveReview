import React, { useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import CodeHostCallback from './CodeHostCallback';

const OAuthCallbackHandler: React.FC = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const code = searchParams.get('code');
  const error = searchParams.get('error');

  useEffect(() => {
    // Process the code or error and then redirect
    if (code) {
      console.log("OAuth code received:", code);
      // Here you would typically exchange the code for an access token
      // But for now, we'll just redirect to the dashboard after a short delay
      setTimeout(() => {
        navigate('/dashboard');
      }, 2000);
    } else if (error) {
      console.error("OAuth error:", error);
      // Redirect to git providers page after a short delay to show the error
      setTimeout(() => {
        navigate('/git');
      }, 2000);
    } else {
      // If there's no code or error, redirect to git providers page immediately
      navigate('/git');
    }
  }, [code, error, navigate]);

  // Show a simple loading/processing screen
  return (
    <div className="container mx-auto px-4 py-8">
      <div className="bg-slate-800 p-6 rounded-lg max-w-3xl mx-auto">
        <div className="flex flex-col items-center justify-center py-12">
          <svg className="animate-spin h-10 w-10 text-blue-500 mb-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <h2 className="text-xl font-medium text-white">
            {code ? "Authentication successful! Redirecting..." : 
             error ? "Authentication error! Redirecting..." : 
             "Processing..."}
          </h2>
        </div>
      </div>
    </div>
  );
};

export default OAuthCallbackHandler;
