import React, { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { Dashboard } from '../../components/Dashboard/Dashboard';
import CodeHostCallback from '../Auth/CodeHostCallback';

const HomeWithOAuthCheck: React.FC = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const code = searchParams.get('code');
  const error = searchParams.get('error');
  const [showOAuthResult, setShowOAuthResult] = useState(false);

  useEffect(() => {
    // If there's an OAuth code or error parameter, handle it
    if (code || error) {
      console.log("OAuth code received:", code);
      console.log("OAuth error:", error);
      
      // Show the OAuth result UI
      setShowOAuthResult(true);
      
      // Clear the URL parameters without reloading the page
      // This is important to prevent the code from being used multiple times
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  }, [code, error]);

  // If we have OAuth parameters, show the OAuth result screen
  if (showOAuthResult) {
    return (
      <CodeHostCallback 
        code={code || undefined} 
        error={error || undefined} 
      />
    );
  }

  // Otherwise, render the regular dashboard component
  return <Dashboard />;
};

export default HomeWithOAuthCheck;
