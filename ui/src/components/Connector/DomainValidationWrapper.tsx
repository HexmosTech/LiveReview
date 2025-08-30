import React from 'react';
import { Card, Button } from '../UIPrimitives';
import DomainWarning from './DomainWarning';
import useDomainValidation from './useDomainValidation';

interface DomainValidationWrapperProps {
  children: React.ReactNode;
}

/**
 * A wrapper component that ensures domain is configured before
 * allowing children components to render.
 * 
 * This component is designed to be used at the "Create connector" level
 * to prevent users from creating any type of connector without a configured domain.
 */
const DomainValidationWrapper: React.FC<DomainValidationWrapperProps> = ({ children }) => {
  const domainInfo = useDomainValidation();
  
  // If loading, show a centered spinner
  if (domainInfo.isLoading) {
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
  
  // If there's an error or domain is not configured, show warning
  if (domainInfo.error || !domainInfo.isConfigured) {
    return (
      <Card className="p-6">
        <DomainWarning domainInfo={domainInfo} className="mb-6" />
        
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
  
  // If domain is configured, render children
  return <>{children}</>;
};

export default DomainValidationWrapper;
