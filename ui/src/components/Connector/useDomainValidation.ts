import { useState, useEffect } from 'react';
import apiClient from '../../api/apiClient';

export interface DomainInfo {
  url: string;
  isConfigured: boolean;
  isLoading: boolean;
  error: string;
}

/**
 * Custom hook to validate domain configuration across connectors
 * This ensures we have a consistent approach to domain validation
 */
export function useDomainValidation(): DomainInfo {
  const [domainInfo, setDomainInfo] = useState<DomainInfo>({
    url: '',
    isConfigured: false,
    isLoading: true,
    error: ''
  });

  useEffect(() => {
    const fetchProductionUrl = async () => {
      try {
        const response = await apiClient.get<{url: string, success: boolean, message: string}>('/api/v1/production-url');
        setDomainInfo({
          url: response.url || '',
          isConfigured: !!response.url && response.success,
          isLoading: false,
          error: ''
        });
      } catch (error) {
        setDomainInfo({
          url: '',
          isConfigured: false,
          isLoading: false,
          error: error instanceof Error ? error.message : 'Unknown error occurred'
        });
      }
    };

    fetchProductionUrl();
  }, []);

  return domainInfo;
}

export default useDomainValidation;
