import React from 'react';
import { Alert, Icons } from '../UIPrimitives';
import { DomainInfo } from './useDomainValidation';

interface DomainWarningProps {
  domainInfo: DomainInfo;
  className?: string;
}

/**
 * A reusable component to display domain configuration warnings
 * Can be used across different connector components
 */
const DomainWarning: React.FC<DomainWarningProps> = ({ domainInfo, className = 'mb-4' }) => {
  if (domainInfo.isLoading) {
    return (
      <Alert
        variant="info"
        icon={<Icons.Info />}
        className={className}
      >
        <div className="flex items-center">
          <div className="animate-spin mr-2">
            <svg className="w-4 h-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </div>
          <div>
            <p className="font-medium">Checking domain configuration...</p>
            <p className="text-sm">Please wait while we verify your domain settings.</p>
          </div>
        </div>
      </Alert>
    );
  }
  
  if (domainInfo.error) {
    return (
      <Alert
        variant="error"
        icon={<Icons.Warning />}
        className={className}
      >
        <div>
          <p className="font-medium">Error checking domain</p>
          <p className="text-sm">
            {domainInfo.error}. Please try again or contact support.
          </p>
        </div>
      </Alert>
    );
  }
  
  if (!domainInfo.isConfigured) {
    return (
      <Alert
        variant="warning"
        icon={<Icons.Warning />}
        className={className}
      >
        <div>
          <p className="font-medium">Domain not configured</p>
          <p className="text-sm">
            Please configure your application domain in the{' '}
            <a href="#settings" className="font-medium underline">Settings</a>{' '}
            page before continuing.
          </p>
        </div>
      </Alert>
    );
  }
  
  return null;
};

export default DomainWarning;
