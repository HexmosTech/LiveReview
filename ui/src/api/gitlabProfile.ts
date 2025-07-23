import apiClient from './apiClient';

export async function validateGitLabProfile(baseUrl: string, pat: string) {
  try {
    const result = await apiClient.post('/api/v1/gitlab/validate-profile', { base_url: baseUrl, pat });
    return result;
  } catch (error: any) {
    if (error && error.message) {
      throw new Error(error.message);
    }
    throw new Error('Network error');
  }
}
