import apiClient from './apiClient';

interface GitLabTokenResponse {
  message: string;
  token_id: number;
  integration_id: number;
  username: string;
  connection_name: string;
}

interface GitLabTokenRefreshResponse {
  message: string;
  token_id: number;
  expires_in: number;
}

/**
 * Get GitLab access token using authorization code
 * @param code Authorization code from GitLab
 * @param gitlabUrl GitLab URL (e.g., https://gitlab.com)
 * @param clientId GitLab application client ID
 * @param clientSecret GitLab application client secret
 * @param redirectUri Redirect URI used in the authorization request
 * @param connectionName Optional name for this connection
 * @returns Promise with GitLab token response
 */
export const getGitLabAccessToken = async (
  code: string,
  gitlabUrl: string,
  clientId: string,
  clientSecret: string,
  redirectUri: string,
  connectionName?: string
): Promise<GitLabTokenResponse> => {
  try {
    console.log('Getting GitLab access token');
    
    const response = await apiClient.post<GitLabTokenResponse>('/api/v1/gitlab/token', {
      code,
      gitlab_url: gitlabUrl,
      gitlab_client_id: clientId,
      gitlab_client_secret: clientSecret,
      redirect_uri: redirectUri,
      connection_name: connectionName
    });
    
    console.log('GitLab token response:', response);
    return response;
  } catch (error) {
    console.error('Error getting GitLab access token:', error);
    throw error;
  }
};

/**
 * Refresh GitLab access token
 * @param tokenId ID of the token to refresh
 * @param clientId GitLab application client ID
 * @param clientSecret GitLab application client secret
 * @returns Promise with GitLab token refresh response
 */
export const refreshGitLabToken = async (
  tokenId: number,
  clientId: string,
  clientSecret: string
): Promise<GitLabTokenRefreshResponse> => {
  try {
    console.log('Refreshing GitLab token');
    
    const response = await apiClient.post<GitLabTokenRefreshResponse>('/api/v1/gitlab/refresh', {
      token_id: tokenId,
      gitlab_client_id: clientId,
      gitlab_client_secret: clientSecret
    });
    
    console.log('GitLab token refresh response:', response);
    return response;
  } catch (error) {
    console.error('Error refreshing GitLab token:', error);
    throw error;
  }
};
