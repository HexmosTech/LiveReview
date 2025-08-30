import apiClient from './apiClient';

export async function validateBitbucketProfile(email: string, apiToken: string) {
  try {
    const result = await apiClient.post('/api/v1/bitbucket/validate-profile', { 
      email, 
      api_token: apiToken 
    });
    return result;
  } catch (error: any) {
    if (error && error.message) {
      throw new Error(error.message);
    }
    throw new Error('Network error');
  }
}
