import apiClient from './apiClient';

export interface GitHubAppInstallation {
  connector_id: number;
  account_login: string;
  account_type: string;
  repository_selection: string;
  settings_url: string;
  /** "user": the App was installed by someone else, so this org gets only the repos its GitHub user can access. */
  repo_scope: 'installer' | 'user';
  /** How many other LiveReview accounts use this installation. */
  other_orgs: number;
  /** Their owners' emails, masked by the server (e.g. "at•••@gmail.com"). */
  other_accounts: string[];
}

/** An installation the org isn't connected to but can connect in one click (org owners). */
export interface GitHubAppAvailableInstallation {
  connector_id: number;
  account_login: string;
  account_type: string;
  /** "offered": someone in another LiveReview account installed it; "disconnected": this account disconnected it. */
  state: 'offered' | 'disconnected';
  /** The repositories Connect gives: all of them ("installer"), or only those your GitHub account can access ("user"). */
  repo_scope: 'installer' | 'user';
  other_orgs: number;
  other_accounts: string[];
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
  /** Installations this org can connect but hasn't (no repositories or pull requests are listed for them). */
  available: GitHubAppAvailableInstallation[];
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
 * Stops LiveReview using one GitHub App installation for the current org (org owners only). By default only this
 * org is disconnected: the App stays installed on GitHub and other LiveReview accounts using it keep working.
 * `uninstall` also uninstalls the App from GitHub, which removes it for every account using it.
 */
export const disconnectGitHubInstallation = (connectorId: number, uninstall = false): Promise<{ status: string }> =>
  apiClient.delete<{ status: string }>(`/api/v1/github-app/installations/${connectorId}${uninstall ? '?uninstall=true' : ''}`);

/**
 * Connects an installation the current org has available but not connected (org owners only). It keeps the repository
 * scope from the last GitHub sign-in and syncs the repositories in the background.
 */
export const connectGitHubInstallation = (connectorId: number): Promise<{ status: string }> =>
  apiClient.post<{ status: string }>(`/api/v1/github-app/installations/${connectorId}/connect`, {});

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
 * Where "Continue with GitHub" (/gh-login) starts on the server; it redirects on to GitHub and signs in as the
 * GitHub user's email. Inside the app use startGitHubAppConnect instead, which keeps the current session.
 */
export const githubAppSignInUrl = (src: string): string =>
  `/api/v1/auth/github-app/start?src=${encodeURIComponent(src)}`;

/**
 * "install": GitHub's install page (on `targetId`, the numeric GitHub id of an account or organization, when set).
 * "discover": GitHub's sign-in, which connects or offers every account your GitHub user can already see the App on.
 */
export type GitHubAppConnectMode = 'install' | 'discover';

/** The page a GitHub round trip started inside the app comes back to. */
export type GitHubAppReturn = 'git' | 'get-started' | `review/${number}`;

/**
 * Starts a GitHub round trip for the current org and resolves to the GitHub URL to open. The round trip connects
 * whatever the GitHub account can access to this org and comes back to `returnTo` signed in as the same user,
 * whatever email the GitHub account uses.
 */
export const startGitHubAppConnect = async (
  mode: GitHubAppConnectMode,
  src: string,
  returnTo: GitHubAppReturn,
  targetId?: number,
): Promise<string> => {
  const res = await apiClient.post<{ url: string }>('/api/v1/github-app/start', {
    mode,
    src,
    return: returnTo,
    ...(targetId ? { target_id: targetId } : {}),
  });
  return res.url;
};

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
