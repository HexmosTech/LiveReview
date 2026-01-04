import apiClient from './apiClient';

export async function validateGiteaProfile(base_url: string, pat: string) {
  try {
    const result = await apiClient.post('/api/v1/gitea/validate-profile', { base_url, pat });
    return result;
  } catch (error: any) {
    if (error && error.message) {
      throw new Error(error.message);
    }
    throw new Error('Network error');
  }
}
