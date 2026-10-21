import { useQuery } from '@tanstack/react-query';
import { getGitHubAppDemo } from '../api/githubApp';
import { useAppSelector } from '../store/configureStore';
import { isCloudMode } from '../utils/deploymentMode';

export const GITHUB_APP_CONNECTED_QUERY_KEY = ['github-app', 'connected'] as const;

/**
 * Whether the current org has a LiveReview GitHub App connection. Uses the light variant of the endpoint (no
 * GitHub calls). Any failure - including a 404 when the server has no GitHub App configured - means "not connected".
 */
export const useGitHubAppConnected = (): boolean => {
  const isAuthenticated = useAppSelector((state) => state.Auth.isAuthenticated);
  const { data } = useQuery({
    queryKey: GITHUB_APP_CONNECTED_QUERY_KEY,
    queryFn: () => getGitHubAppDemo(true),
    enabled: isCloudMode() && isAuthenticated,
    staleTime: 60_000,
    retry: false,
  });
  return Boolean(data?.connected);
};
