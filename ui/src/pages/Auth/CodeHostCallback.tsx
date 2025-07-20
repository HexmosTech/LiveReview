import React, { useEffect, useState } from 'react';
import { Card } from '../../components/UIPrimitives';
import { useAppSelector } from '../../store/configureStore';
import { useNavigate } from 'react-router-dom';

interface CodeHostCallbackProps {
  code?: string;
  error?: string;
}

const CodeHostCallback: React.FC<CodeHostCallbackProps> = ({ code: propCode, error: propError }) => {
  const [callbackData, setCallbackData] = useState<{ code?: string, state?: string, error?: string }>({});
  const [loading, setLoading] = useState(true);
  const connectors = useAppSelector((state) => state.Connector.connectors);
  const navigate = useNavigate();

  useEffect(() => {
    // If code and error are passed as props, use them
    if (propCode || propError) {
      setCallbackData({
        code: propCode,
        error: propError
      });
      setLoading(false);
      return;
    }
    
    // Extract query parameters from URL search params
    // This is a fallback for when props aren't provided
    let code, state, error;
    
    const urlParams = new URLSearchParams(window.location.search);
    code = urlParams.get('code');
    state = urlParams.get('state');
    error = urlParams.get('error');
    
    console.log("Extracted callback data:", { code, state, error });
    console.log("Original URL:", window.location.href);
    
    setCallbackData({
      code: code || undefined,
      state: state || undefined,
      error: error || undefined
    });
    
    setLoading(false);
    
    // In a real implementation, you would:
    // 1. Exchange the code for an access token
    // 2. Store the token securely
    // 3. Redirect to the appropriate page
  }, [propCode, propError]);

  const handleBackToGitProviders = () => {
    navigate('/git');
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card className="p-6 max-w-3xl mx-auto">
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="animate-spin h-10 w-10 text-blue-500 mb-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <h2 className="text-xl font-medium text-white">Processing authentication...</h2>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <Card className="p-6 max-w-3xl mx-auto">
        <h2 className="text-2xl font-bold text-white mb-6">Authentication Result</h2>
        
        {callbackData.error ? (
          <div className="bg-red-900/50 border border-red-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-red-400 mb-2">Authentication Error</h3>
            <p className="text-white">{callbackData.error}</p>
          </div>
        ) : callbackData.code ? (
          <div className="bg-green-900/50 border border-green-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-green-400 mb-2">Authentication Successful!</h3>
            <p className="text-white">Successfully received authorization code.</p>
          </div>
        ) : (
          <div className="bg-orange-900/50 border border-orange-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-orange-400 mb-2">Missing Data</h3>
            <p className="text-white">No authorization code or error was received.</p>
          </div>
        )}
        
        <div className="bg-slate-800 p-4 rounded-md mb-6">
          <h3 className="text-lg font-medium text-white mb-2">Response Data</h3>
          <pre className="bg-slate-900 p-3 rounded-md overflow-auto text-sm text-green-400">
            {JSON.stringify(callbackData, null, 2)}
          </pre>
        </div>
        
        <div className="bg-slate-800 p-4 rounded-md mb-6">
          <h3 className="text-lg font-medium text-white mb-2">Available Connectors</h3>
          <pre className="bg-slate-900 p-3 rounded-md overflow-auto text-sm text-blue-400">
            {JSON.stringify(connectors, null, 2)}
          </pre>
        </div>
        
        <div className="flex justify-center mt-4">
          <button
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-md text-white font-medium"
            onClick={handleBackToGitProviders}
          >
            Back to Git Providers
          </button>
        </div>
      </Card>
    </div>
  );
};

export default CodeHostCallback;
