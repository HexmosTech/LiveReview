import React, { useState, useEffect } from 'react';
import { Card, Button, Alert, Icons } from '../UIPrimitives';
import apiClient from '../../api/apiClient';

interface DomainValidatorProps {
  children: React.ReactNode;
}

/**
 * Domain validator specifically for the Git Providers page
 * Blocks access to connector creation until domain is configured
 * Allows access in development mode (localhost) even without domain configuration
 */
const DomainValidator: React.FC<DomainValidatorProps> = ({ children }) => {
  const [state, setState] = useState({
    isLoading: true,
    isConfigured: false,
    error: '',
    url: ''
  });

  // Check if we're running in development mode
  const isDevelopment = window.location.hostname === 'localhost' || 
                       window.location.hostname === '127.0.0.1' ||
                       window.location.hostname.includes('localhost');

  useEffect(() => {
    const checkDomain = async () => {
      try {
        const response = await apiClient.get<{url: string, success: boolean, message: string}>('/api/v1/production-url');
        setState({
          isLoading: false,
          isConfigured: !!response.url && response.success,
          error: '',
          url: response.url || ''
        });
      } catch (error) {
        setState({
          isLoading: false,
          isConfigured: false,
          error: error instanceof Error ? error.message : 'Unknown error occurred',
          url: ''
        });
      }
    };

    checkDomain();
  }, []);

  if (state.isLoading) {
    return (
      <Card className="p-6">
        <div className="flex flex-col items-center justify-center py-10">
          <div className="animate-spin mb-4">
            <svg className="w-10 h-10 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </div>
          <p className="text-lg font-medium text-white">Checking domain configuration...</p>
          <p className="text-sm text-slate-300 mt-2">Please wait while we verify your domain settings.</p>
        </div>
      </Card>
    );
  }

  if (state.error) {
    return (
      <Card className="p-6">
        <Alert
          variant="error"
          icon={<Icons.Warning />}
          className="mb-6"
        >
          <div>
            <p className="font-medium">Error checking domain</p>
            <p className="text-sm">
              {state.error}. Please try again or contact support.
            </p>
          </div>
        </Alert>
        
        <div className="text-center mt-4">
          <Button 
            variant="primary"
            onClick={() => window.location.hash = 'settings'}
          >
            Go to Settings
          </Button>
        </div>
      </Card>
    );
  }

  // Allow access in development mode even if domain is not configured
  if (!state.isConfigured && isDevelopment) {
    console.log('DomainValidator: Allowing access in development mode');
    return <>{children}</>;
  }

  if (!state.isConfigured) {
    return (
      <Card className="p-6">
        <Alert
          variant="warning"
          icon={<Icons.Warning />}
          className="mb-6"
        >
          <div>
            <p className="font-medium">Domain not configured</p>
            <p className="text-sm">
              You need to configure your application domain before creating Git provider connections.
              This is required for OAuth callback URLs to work correctly.
            </p>
          </div>
        </Alert>
        
        <div className="text-center mt-4">
          <Button 
            variant="primary"
            onClick={() => window.location.hash = 'settings'}
          >
            Configure Domain in Settings
          </Button>
        </div>
      </Card>
    );
  }

  return <>{children}</>;
};

export default DomainValidator;
