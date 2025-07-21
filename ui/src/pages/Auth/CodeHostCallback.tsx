import React, { useEffect, useState } from 'react';
import { Card } from '../../components/UIPrimitives';
import { useAppSelector } from '../../store/configureStore';
import { useNavigate } from 'react-router-dom';
import { getGitLabAccessToken } from '../../api/gitlab';

interface CodeHostCallbackProps {
  code?: string;
  error?: string;
}

interface ConnectorDetails {
  name: string;
  type: string;
  url: string;
  applicationId: string;
  applicationSecret: string;
}

interface TokenDetails {
  message: string;
  integration_id: number; // The backend returns integration_id instead of token_id
  username: string;
  connection_name: string;
}

const CodeHostCallback: React.FC<CodeHostCallbackProps> = ({ code: propCode, error: propError }) => {
  const [callbackData, setCallbackData] = useState<{ code?: string, state?: string, error?: string }>({});
  const [loading, setLoading] = useState(true);
  const [connectorDetails, setConnectorDetails] = useState<ConnectorDetails | null>(null);
  const [tokenDetails, setTokenDetails] = useState<TokenDetails | null>(null);
  const [tokenError, setTokenError] = useState<string | null>(null);
  const [tokenLoading, setTokenLoading] = useState(false);
  const connectors = useAppSelector((state) => state.Connector.connectors);
  const navigate = useNavigate();

  useEffect(() => {
    // Retrieve connector details from localStorage
    const storedDetails = localStorage.getItem('pendingGitLabConnector');
    if (storedDetails) {
      try {
        const parsedDetails = JSON.parse(storedDetails);
        setConnectorDetails(parsedDetails);
        console.log("Retrieved connector details:", parsedDetails);
      } catch (e) {
        console.error("Error parsing connector details:", e);
      }
    }

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
  }, [propCode, propError]);

  // Exchange authorization code for access token when we have both code and connector details
  useEffect(() => {
    const exchangeCodeForToken = async () => {
      if (!callbackData.code || !connectorDetails || tokenLoading) {
        return;
      }

      setTokenLoading(true);
      setTokenError(null);

      try {
        // Get domain info for redirect URI
        let redirectUri = window.location.origin;
        
        // Exchange the code for an access token
        const response = await getGitLabAccessToken(
          callbackData.code,
          connectorDetails.url,
          connectorDetails.applicationId,
          connectorDetails.applicationSecret,
          redirectUri,
          connectorDetails.name
        );

        setTokenDetails(response);
        console.log("Token exchange successful:", response);
        
        // No automatic redirection - let the user control when to proceed
        // Just clear the stored connector details as they're no longer needed
        localStorage.removeItem('pendingGitLabConnector');
        
      } catch (error: any) {
        console.error("Error exchanging code for token:", error);
        setTokenError(error.message || "Failed to exchange authorization code for access token");
      } finally {
        setTokenLoading(false);
      }
    };

    exchangeCodeForToken();
  }, [callbackData.code, connectorDetails, navigate]);

  const handleBackToGitProviders = () => {
    // Clear the stored connector details when navigating away
    localStorage.removeItem('pendingGitLabConnector');
    navigate('/git');
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card className="p-6 max-w-3xl mx-auto">
          <div className="flex flex-col items-center justify-center py-6">
            <svg className="animate-spin h-8 w-8 text-blue-500 mb-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <h2 className="text-lg font-medium text-white">Processing...</h2>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-2 sm:px-4 py-4 sm:py-8">
      <Card className="p-4 sm:p-6 max-w-3xl mx-auto overflow-hidden">
        <h2 className="text-xl sm:text-2xl font-bold text-white mb-4 sm:mb-6">Authentication Result</h2>
        
        {callbackData.error ? (
          <div className="bg-red-900/50 border border-red-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-red-400 mb-2">Authentication Error</h3>
            <p className="text-white">{callbackData.error}</p>
          </div>
        ) : tokenError ? (
          <div className="bg-red-900/50 border border-red-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-red-400 mb-2">Token Exchange Error</h3>
            <p className="text-white">{tokenError}</p>
          </div>
        ) : tokenDetails ? (
          <div className="bg-green-900/50 border border-green-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-green-400 mb-2">Authentication Complete!</h3>
            <p className="text-white mb-3">{tokenDetails.message}</p>
            <p className="text-white mb-4">
              Your GitLab integration is now successfully set up. You can now either return to the Git Providers page to manage your connections 
              or go directly to the Dashboard to start using LiveReview with GitLab.
            </p>
            <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="md:col-span-2">
                <p className="text-white text-sm mb-1 font-medium">Integration ID:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{tokenDetails.integration_id}</p>
              </div>
              <div>
                <p className="text-white text-sm mb-1 font-medium">Username:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{tokenDetails.username || 'Not available'}</p>
              </div>
              <div>
                <p className="text-white text-sm mb-1 font-medium">Connection Name:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{tokenDetails.connection_name}</p>
              </div>
            </div>
          </div>
        ) : callbackData.code ? (
          <div className="bg-yellow-900/50 border border-yellow-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-yellow-400 mb-2">
              {tokenLoading ? "Exchanging Code for Token..." : "Authorization Code Received"}
            </h3>
            <p className="text-white">
              {tokenLoading ? (
                <span className="flex items-center">
                  <svg className="animate-spin h-5 w-5 text-yellow-400 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  Exchanging authorization code for access token...
                </span>
              ) : (
                "Successfully received authorization code. Waiting to exchange for access token."
              )}
            </p>
          </div>
        ) : (
          <div className="bg-orange-900/50 border border-orange-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-orange-400 mb-2">Missing Data</h3>
            <p className="text-white">No authorization code or error was received.</p>
          </div>
        )}
        
        {/* Connector Details Section */}
        {connectorDetails && (
          <div className="bg-blue-900/50 border border-blue-700 rounded-md p-4 mb-6">
            <h3 className="text-lg font-medium text-blue-400 mb-2">Connector Details</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <p className="text-white text-sm mb-1 font-medium">Connector Name:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{connectorDetails.name}</p>
              </div>
              <div>
                <p className="text-white text-sm mb-1 font-medium">Connector Type:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{connectorDetails.type}</p>
              </div>
              <div>
                <p className="text-white text-sm mb-1 font-medium">GitLab URL:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm">{connectorDetails.url}</p>
              </div>
              <div className="md:col-span-2">
                <p className="text-white text-sm mb-1 font-medium">Application ID:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm whitespace-normal break-all">{connectorDetails.applicationId}</p>
              </div>
              <div className="md:col-span-2">
                <p className="text-white text-sm mb-1 font-medium">Application Secret:</p>
                <p className="bg-slate-800 p-2 rounded text-slate-300 overflow-auto text-sm whitespace-normal break-all">{connectorDetails.applicationSecret}</p>
              </div>
            </div>
          </div>
        )}
        
        <div className="bg-slate-800 p-4 rounded-md mb-6">
          <h3 className="text-lg font-medium text-white mb-2">Response Data</h3>
          <pre className="bg-slate-900 p-3 rounded-md overflow-auto text-sm text-green-400 max-h-32 md:max-h-48">
            {JSON.stringify(callbackData, null, 2)}
          </pre>
        </div>
        
        <div className="flex justify-center gap-4 mt-4">
          {tokenDetails ? (
            <>
              <button
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-md text-white font-medium"
                onClick={() => navigate('/git')}
              >
                Return to Git Providers
              </button>
              <button
                className="px-4 py-2 bg-green-600 hover:bg-green-700 rounded-md text-white font-medium"
                onClick={() => navigate('/dashboard')}
              >
                Go to Dashboard
              </button>
            </>
          ) : (
            <button
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-md text-white font-medium"
              onClick={handleBackToGitProviders}
            >
              Back to Git Providers
            </button>
          )}
        </div>
      </Card>
    </div>
  );
};

export default CodeHostCallback;
