import apiClient from './apiClient';

export interface GitHubAppInstallation {
  connector_id: number;
  account_login: string;
  account_type: string;
  repository_selection: string;
  settings_url: string;
}

export interface GitHubAppRepository {
  full_name: string;
  web_url: string;
  private: boolean;
}

export interface GitHubAppPullRequest {
  repo_full_name: string;
  number: number;
  title: string;
  web_url: string;
  author: string;
  updated_at: string;
}

export interface GitHubAppDemo {
  connected: boolean;
  installations: GitHubAppInstallation[];
  repositories: GitHubAppRepository[];
  open_pull_requests: GitHubAppPullRequest[];
}

/**
 * What the GitHub App get-started page shows for the current org: installations, repositories and open pull
 * requests. `light` skips the GitHub calls and only answers whether an App is connected (nav/banner checks).
 */
export const getGitHubAppDemo = (light = false): Promise<GitHubAppDemo> =>
  apiClient.get<GitHubAppDemo>(`/api/v1/github-app/demo${light ? '?light=1' : ''}`);

/**
 * Stops LiveReview using one GitHub App installation for the current org (org owners only). The App stays
 * installed on GitHub; signing in with GitHub again reconnects it.
 */
export const disconnectGitHubInstallation = (connectorId: number): Promise<{ status: string }> =>
  apiClient.delete<{ status: string }>(`/api/v1/github-app/installations/${connectorId}`);

export interface PublicUIConfig {
  isCloud: boolean;
  githubAppEnabled?: boolean;
  githubAppBaseUrl?: string;
}

/** Public, unauthenticated server config. The UI bundle itself carries no GitHub App ids or URLs. */
export const getPublicUIConfig = async (): Promise<PublicUIConfig> => {
  const response = await fetch('/api/v1/ui-config');
  if (!response.ok) {
    throw new Error(`Failed to load UI config (${response.status})`);
  }
  return response.json();
};

/**
 * Where the "Continue with GitHub" flow starts on the server; it redirects on to GitHub. `targetId` (install mode)
 * is the numeric GitHub id of the account or organization to install on, so GitHub opens straight at it.
 */
export const githubAppStartUrl = (mode: 'install' | 'signin', src: string, targetId?: number): string =>
  `/api/v1/auth/github-app/start?mode=${mode}&src=${encodeURIComponent(src)}${targetId ? `&target_id=${targetId}` : ''}`;

export interface GitHubAppInstallCheck {
  /** False when the input is not a pull request (or repository) on the App's GitHub host. */
  applicable: boolean;
  /** True when one of the org's GitHub connectors already lists the repository. */
  installed: boolean;
  owner?: string;
  repository?: string;
  /** The owner's numeric GitHub id, for opening GitHub's install page already on that owner. */
  target_id?: number;
}

/** Whether LiveReview can already post to the repository of a GitHub PR URL (`url`) or `owner/name` (`repo`). */
export const getGitHubAppInstallCheck = (input: { url?: string; repo?: string }): Promise<GitHubAppInstallCheck> => {
  const params = new URLSearchParams();
  if (input.url) params.set('url', input.url);
  if (input.repo) params.set('repo', input.repo);
  return apiClient.get<GitHubAppInstallCheck>(`/api/v1/github-app/install-check?${params.toString()}`);
};
