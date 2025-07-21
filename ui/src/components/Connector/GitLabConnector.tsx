import React, { useState, useEffect } from 'react';
import { Card, Input, Button, Icons, Alert } from '../UIPrimitives';
import { ConnectorType, addConnector } from '../../store/Connector/reducer';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import apiClient from '../../api/apiClient';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { isDuplicateConnector } from './checkConnectorDuplicate';

type GitLabConnectorProps = {
  type: 'gitlab-com' | 'gitlab-self-hosted';
  onSubmit: (data: any) => void;
};

const GitLabConnector: React.FC<GitLabConnectorProps> = ({ type, onSubmit }) => {
  const dispatch = useAppDispatch();
  const navigate = useNavigate();
  const location = useLocation();
  const { step: urlStep } = useParams<{ step?: string }>();
  
  const [step, setStep] = useState<number>(urlStep === 'step2' ? 2 : 1);
  const [domainInfo, setDomainInfo] = useState({
    url: '',
    isConfigured: false
  });
  const [formData, setFormData] = useState({
    name: type === 'gitlab-com' ? 'GitLab.com' : 'Self-hosted GitLab',
    url: type === 'gitlab-com' ? 'https://gitlab.com' : '',
    applicationId: '',
    applicationSecret: ''
  });
  const [tempConnectorData, setTempConnectorData] = useState<any>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  
  // Get connectors from Redux state
  const connectors = useAppSelector((state) => state.Connector.connectors);

  // Set the URL path based on the current step
  useEffect(() => {
    if (step === 1 && urlStep !== 'step1') {
      navigate(`/git/${type}/step1`, { replace: true });
    } else if (step === 2 && urlStep !== 'step2') {
      navigate(`/git/${type}/step2`, { replace: true });
    }
  }, [step, type, navigate, urlStep]);

  // Check the URL path on component mount to determine the step
  useEffect(() => {
    if (urlStep === 'step2') {
      setStep(2);
    } else {
      setStep(1);
    }
  }, [urlStep]);

  // Fetch domain info when component mounts
  useEffect(() => {
    const fetchDomainInfo = async () => {
      try {
        const response = await apiClient.get<{url: string, success: boolean, message: string}>('/api/v1/production-url');
        console.log("Domain info response:", response);
        setDomainInfo({
          url: response.url || '',
          isConfigured: !!response.url && response.success
        });
        console.log("Set domain info to:", {
          url: response.url || '',
          isConfigured: !!response.url && response.success
        });
      } catch (error) {
        console.error('Failed to fetch domain info:', error);
        setDomainInfo({
          url: '',
          isConfigured: false
        });
      }
    };

    fetchDomainInfo();
  }, []);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    // Store connector data
    const connectorData = {
      name: formData.name,
      type,
      url: formData.url,
      apiKey: formData.applicationId,
      apiSecret: formData.applicationSecret
    };
    
    // Save to Redux store - include application secret this time
    const connector = {
      name: formData.name,
      type,
      url: formData.url,
      apiKey: formData.applicationId,
      apiSecret: formData.applicationSecret // Include the secret
    };
    
    dispatch(addConnector(connector));
    
    // Call the onSubmit prop to notify parent component
    onSubmit(connectorData);
    
    // Store complete data in component state for authorization
    setTempConnectorData(connectorData);
    
    // Show a loading indicator that covers the screen
    const loadingDiv = document.createElement('div');
    loadingDiv.id = 'gitlab-redirect-overlay';
    loadingDiv.style.position = 'fixed';
    loadingDiv.style.top = '0';
    loadingDiv.style.left = '0';
    loadingDiv.style.width = '100%';
    loadingDiv.style.height = '100%';
    loadingDiv.style.backgroundColor = 'rgba(15, 23, 42, 0.9)'; // Tailwind slate-900 with opacity
    loadingDiv.style.zIndex = '9999';
    loadingDiv.style.display = 'flex';
    loadingDiv.style.alignItems = 'center';
    loadingDiv.style.justifyContent = 'center';
    
    // Add a loading spinner and text
    loadingDiv.innerHTML = `
      <div style="display: flex; flex-direction: column; align-items: center;">
        <svg class="animate-spin h-10 w-10 text-blue-500 mb-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" style="color: #3b82f6; animation: spin 1s linear infinite;">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        <p style="color: white; font-weight: 500; margin-bottom: 8px;">Redirecting to GitLab...</p>
        <p style="color: #94a3b8; font-size: 14px;">You'll be redirected to GitLab for authorization.</p>
        <p style="color: #94a3b8; font-size: 14px;">After authorization, you'll be returned to LiveReview.</p>
      </div>
    `;
    
    // Add the loading overlay to the body
    document.body.appendChild(loadingDiv);
    
    // Add animation for spinner
    const style = document.createElement('style');
    style.innerHTML = `
      @keyframes spin {
        from { transform: rotate(0deg); }
        to { transform: rotate(360deg); }
      }
    `;
    document.head.appendChild(style);
    
    // Redirect to GitLab after a very short delay to ensure the loading state is visible
    setTimeout(() => {
      redirectToGitLabAuth();
    }, 50);
  };

  const redirectToGitLabAuth = () => {
    const SCOPES = ['api'];
    const gitProviderBaseURL = type === 'gitlab-com' ? 'https://gitlab.com' : formData.url;
    const gitClientID = formData.applicationId;
    
    // Store connector details in localStorage for use after OAuth callback
    const connectorDetails = {
      name: formData.name,
      type,
      url: formData.url,
      applicationId: formData.applicationId,
      applicationSecret: formData.applicationSecret
    };
    localStorage.setItem('pendingGitLabConnector', JSON.stringify(connectorDetails));
    
    // Use the homepage as redirect URI since GitLab doesn't allow fragments
    let REDIRECT_URI = '';
    if (domainInfo.url) {
      // Check if domainInfo.url already includes https://
      if (domainInfo.url.startsWith('https://')) {
        REDIRECT_URI = domainInfo.url;
      } else {
        REDIRECT_URI = `https://${domainInfo.url}`;
      }
    } else {
      REDIRECT_URI = window.location.origin;
    }
    
    // Instead of constructing a full URL with query parameters, create a form with hidden inputs
    // This ensures all parameters are properly passed when redirecting
    const form = document.createElement('form');
    form.method = 'GET';
    form.action = `${gitProviderBaseURL}/oauth/authorize`;
    form.style.display = 'none';
    
    // Add all required parameters as hidden form fields
    const params = {
      'client_id': gitClientID,
      'redirect_uri': REDIRECT_URI,
      'scope': SCOPES.join(' '),
      'response_type': 'code',
      'state': 'livereview-gitlab-integration' // Add a state parameter for security
    };
    
    // Create hidden input fields for each parameter
    Object.entries(params).forEach(([key, value]) => {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = key;
      input.value = value;
      form.appendChild(input);
    });
    
    // Add the form to the body and submit it
    document.body.appendChild(form);
    
    console.log("Redirecting to GitLab with params:", params);
    
    // Submit the form immediately, without user interaction
    form.submit();
  };

  // Format the callback URL correctly for display and copy
  const getCallbackUrl = () => {
    if (!domainInfo.url) {
      return window.location.origin;
    }

    // Check if domainInfo.url already includes https://
    let baseUrl = '';
    if (domainInfo.url.startsWith('https://')) {
      baseUrl = domainInfo.url;
    } else {
      baseUrl = `https://${domainInfo.url}`;
    }
    
    // Return the root URL without any path
    return baseUrl;
  };

  const callbackUrl = getCallbackUrl();

  const renderStep1 = () => (
    <>
      <h3 className="text-lg font-medium text-white mb-4">
        {type === 'gitlab-com' ? 'Connect to GitLab.com' : 'Connect to Self-hosted GitLab'}
      </h3>
      
      <p className="text-sm text-slate-300 mb-6">
        To connect with GitLab, you need to create an OAuth application in GitLab and obtain an Application ID.
      </p>

      {errorMessage && (
        <Alert 
          variant="error" 
          className="mb-6"
          onClose={() => setErrorMessage(null)}
        >
          {errorMessage}
        </Alert>
      )}

      <div className="space-y-5">
        <Input
          id="name"
          name="name"
          label="Connector Name"
          type="text"
          value={formData.name}
          onChange={handleChange}
          placeholder={type === 'gitlab-com' ? 'GitLab.com' : 'My GitLab Instance'}
          icon={<Icons.GitLab />}
          required
        />

        {type === 'gitlab-self-hosted' && (
          <Input
            id="url"
            name="url"
            label="GitLab URL"
            type="url"
            value={formData.url}
            onChange={handleChange}
            placeholder="https://gitlab.your-company.com"
            required
          />
        )}

        <div className="pt-2">
          <Button
            variant="primary"
            fullWidth
            onClick={() => {
              // Normalize the URL by removing trailing slashes before navigating to step 2
              if (type === 'gitlab-self-hosted' && formData.url) {
                const normalizedUrl = formData.url.replace(/\/+$/, ''); // Remove trailing slashes
                
                // Check if this URL already exists in connections
                if (isDuplicateConnector(connectors, normalizedUrl)) {
                  // Show error message
                  setErrorMessage(`A connection to "${normalizedUrl}" already exists`);
                  return; // Don't proceed
                }
                
                // Clear any previous errors
                setErrorMessage(null);
                
                // Update URL without trailing slashes
                setFormData(prev => ({
                  ...prev,
                  url: normalizedUrl
                }));
              }
              
              // Navigate to step 2 using React Router
              navigate(`/git/${type}/step2`);
              setStep(2);
            }}
            disabled={type === 'gitlab-self-hosted' && !formData.url}
          >
            Continue to Setup
          </Button>
        </div>
      </div>
    </>
  );

  const renderStep2 = () => {
    const gitlabURL = type === 'gitlab-com' ? 'https://gitlab.com' : formData.url;
    const gitlabApplicationsUrl = `${gitlabURL}/-/user_settings/applications`;
    
    return (
      <>
        <h3 className="text-lg font-medium text-white mb-4">Step 2: Enter the Application ID</h3>
        <div className="bg-slate-700 p-4 rounded-lg mb-6">
          <ol className="list-decimal pl-5 space-y-3 text-slate-300">
            <li>
              <span className="font-medium text-white">Go to </span>
              <a 
                href={gitlabApplicationsUrl} 
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-400 hover:text-blue-300 underline"
              >
                GitLab Applications
              </a>
            </li>
            <li>
              <span className="font-medium text-white">Under the </span>
              <span className="px-2 py-0.5 bg-slate-600 rounded text-white">Add New Application</span>
              <span className="font-medium text-white"> section:</span>
              <ul className="list-disc pl-6 mt-2 space-y-2">
                <li>Enter the name of the application (e.g., <span className="px-2 py-0.5 bg-slate-600 rounded">LiveAPI</span>)</li>
                <li>
                  Set the Redirect URI to:
                  <div className="mt-1 bg-slate-800 p-2 rounded border border-slate-600 flex items-center">
                    <code className="text-green-400">{callbackUrl || 'https://your-domain.com/oauth-callback'}</code>
                    <button 
                      className="ml-2 text-blue-400 hover:text-blue-300"
                      onClick={() => {
                        if (callbackUrl) {
                          navigator.clipboard.writeText(callbackUrl);
                        }
                      }}
                      disabled={!domainInfo.isConfigured}
                    >
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                      </svg>
                    </button>
                  </div>
                </li>
                <li>Uncheck the <span className="px-2 py-0.5 bg-slate-600 rounded">Confidential</span> checkbox</li>
                <li>Under <span className="font-medium">Scopes</span>, check the <span className="px-2 py-0.5 bg-slate-600 rounded">api</span> checkbox</li>
              </ul>
            </li>
            <li>
              <span className="font-medium text-white">Click </span>
              <span className="px-2 py-0.5 bg-slate-600 rounded text-white">Save Application</span>
              <span className="font-medium text-white"> and copy the </span>
              <span className="font-medium text-blue-400">Application ID</span>
              <span className="font-medium text-white"> and </span>
              <span className="font-medium text-blue-400">Secret</span>
            </li>
            <li>
              <span className="font-medium text-white">Paste the Application ID and Secret in the fields below</span>
            </li>
          </ol>
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          <Input
            id="applicationId"
            name="applicationId"
            label="Application ID"
            type="text"
            value={formData.applicationId}
            onChange={handleChange}
            placeholder="Enter your GitLab Application ID"
            required
          />

          <Input
            id="applicationSecret"
            name="applicationSecret"
            label="Application Secret"
            type="text"
            value={formData.applicationSecret}
            onChange={handleChange}
            placeholder="Enter your GitLab Application Secret"
            required
          />

          <div className="flex space-x-3">
            <Button
              variant="ghost"
              onClick={() => {
                // Navigate back to step 1 using React Router
                navigate(`/git/${type}/step1`);
                setStep(1);
              }}
              className="flex-1"
            >
              Back
            </Button>
            <Button
              type="submit"
              variant="primary"
              className="flex-1"
              disabled={!formData.applicationId || !formData.applicationSecret}
            >
              Connect
            </Button>
          </div>
        </form>
      </>
    );
  };

  return (
    <Card className="p-4">
      {step === 1 ? renderStep1() : renderStep2()}
    </Card>
  );
};

export default GitLabConnector;
