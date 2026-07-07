import apiClient from './apiClient';

export async function validateAzureDevOpsProfile(org_url: string, pat: string) {
  try {
    const result = await apiClient.post('/api/v1/azuredevops/validate-profile', { org_url, pat });
    return result;
  } catch (error: any) {
    if (error && error.message) {
      throw new Error(error.message);
    }
    throw new Error('Network error');
  }
}
