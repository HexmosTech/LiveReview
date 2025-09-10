import apiClient from './apiClient';

export interface SystemInfo {
  deployment_mode: 'demo' | 'production';
  capabilities: {
    webhooks_enabled: boolean;
    manual_triggers_only: boolean;
    external_access: boolean;
    proxy_mode: boolean;
  };
}

export const getSystemInfo = async (): Promise<SystemInfo> => {
  try {
    return await apiClient.get<SystemInfo>('/system/info');
  } catch (error) {
    // Fallback heuristic for local dev
    const hostname = window.location.hostname;
    if (hostname === 'localhost' || hostname === '127.0.0.1') {
      return {
        deployment_mode: 'demo',
        capabilities: {
          webhooks_enabled: false,
          manual_triggers_only: true,
          external_access: false,
          proxy_mode: false,
        },
      };
    }
    throw error;
  }
};
