import apiClient from './apiClient';

export async function validateGitHubProfile(pat: string) {
  try {
    const result = await apiClient.post('/api/v1/github/validate-profile', { pat });
    return result;
  } catch (error: any) {
    if (error && error.message) {
      throw new Error(error.message);
    }
    throw new Error('Network error');
  }
}
