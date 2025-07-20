import React, { useState, useEffect } from 'react';
import { Card, Input, Button, Icons } from '../UIPrimitives';
import { ConnectorType, addConnector } from '../../store/Connector/reducer';
import { useAppDispatch } from '../../store/configureStore';
import apiClient from '../../api/apiClient';

type GitLabConnectorProps = {
  type: 'gitlab-com' | 'gitlab-self-hosted';
  onSubmit: (data: any) => void;
};

const GitLabConnector: React.FC<GitLabConnectorProps> = ({ type, onSubmit }) => {
  const dispatch = useAppDispatch();
  const [step, setStep] = useState<number>(1);
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
    
    // Store connector data in Redux
    const connectorData = {
      name: formData.name,
      type,
      url: formData.url,
      apiKey: formData.applicationId,
      apiSecret: formData.applicationSecret
    };
    
    // Save to Redux store
    dispatch(addConnector({
      name: formData.name,
      type,
      url: formData.url,
      apiKey: formData.applicationId
    }));
    
    // Store complete data (including secret) in component state for authorization
    setTempConnectorData(connectorData);
    
    // Redirect to GitLab authorization
    redirectToGitLabAuth();
  };

  const redirectToGitLabAuth = () => {
    const SCOPES = ['api'];
    const gitProviderBaseURL = type === 'gitlab-com' ? 'https://gitlab.com' : formData.url;
    const gitClientID = formData.applicationId;
    
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
    
    const authUrl = `${gitProviderBaseURL}/oauth/authorize?client_id=${gitClientID}&redirect_uri=${encodeURIComponent(REDIRECT_URI)}&scope=${encodeURIComponent(SCOPES.join(' '))}&response_type=code`;
    
    console.log("Redirecting to:", authUrl);
    window.location.href = authUrl;
  };

  // Format the callback URL correctly for display and copy
  const getCallbackUrl = () => {
    if (!domainInfo.url) {
      return '';
    }

    // Check if domainInfo.url already includes https://
    if (domainInfo.url.startsWith('https://')) {
      return domainInfo.url;
    } else {
      return `https://${domainInfo.url}`;
    }
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
              // Store step 1 data in Redux
              dispatch(addConnector({
                name: formData.name,
                type,
                url: formData.url,
                apiKey: '' // Will be updated in step 2
              }));
              setStep(2);
            }}
            disabled={type === 'gitlab-self-hosted' && !formData.url}
          >
            Continue to OAuth Setup
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
                    <code className="text-green-400">{callbackUrl || 'https://your-domain.com/create-docs'}</code>
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
              onClick={() => setStep(1)}
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
