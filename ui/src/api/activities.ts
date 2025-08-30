import apiClient from './apiClient';
import { Icons } from '../components/UIPrimitives';
import React from 'react';

export interface ActivityEntry {
  id: number;
  activity_type: string;
  event_data: Record<string, any>;
  created_at: string;
}

export interface ActivitiesResponse {
  activities: ActivityEntry[];
  total_count: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

/**
 * Fetch recent activities with pagination
 */
export async function fetchRecentActivities(
  limit: number = 20,
  offset: number = 0
): Promise<ActivitiesResponse> {
  const endpoint = `/api/v1/activities?limit=${limit}&offset=${offset}`;
  return apiClient.get<ActivitiesResponse>(endpoint);
}

/**
 * Format activity for display
 */
export function formatActivity(activity: ActivityEntry): {
  title: string;
  description: string;
  icon: React.ReactElement;
  color: string;
  actionUrl?: string;
} {
  const { activity_type, event_data } = activity;

  switch (activity_type) {
    case 'review_triggered':
      const repository = event_data.repository || 'repository';
      // Use stored provider first, fallback to detection from repository name
      const provider = event_data.provider || getProviderFromRepository(repository);
      const branch = event_data.branch;
      const triggerType = event_data.trigger_type || 'manual';
      const originalUrl = event_data.original_url;
      
      // Put the specific repository in the title (main emphasis)
      const title = repository;
      
      // Action and context in description
      let description = `${capitalizeProvider(provider)} review ${triggerType === 'manual' ? 'triggered' : 'auto-triggered'}`;
      
      // Add branch info if meaningful and available
      if (branch && branch.trim() !== '' && branch !== 'unknown' && branch !== 'latest') {
        description += ` on ${branch}`;
      }
      
      return {
        title,
        description,
        icon: getProviderIcon(provider),
        color: 'text-blue-400',
        actionUrl: originalUrl, // Link to the MR/PR
      };

    case 'connector_created':
      const connectorProvider = event_data.provider || 'provider';
      const repoCount = event_data.repository_count || 0;
      const providerUrl = event_data.provider_url;
      
      return {
        title: `${capitalizeProvider(connectorProvider)} Connected`,
        description: `New connector created${
          repoCount > 0 ? ` with ${repoCount} repositories` : ''
        }`,
        icon: getProviderIcon(connectorProvider),
        color: 'text-green-400',
        actionUrl: providerUrl,
      };

    case 'webhook_installed':
      const webhookRepo = event_data.repository || 'repository';
      const webhookProvider = event_data.provider || getProviderFromRepository(webhookRepo);
      const success = event_data.success;
      
      return {
        title: webhookRepo,
        description: `${capitalizeProvider(webhookProvider)} webhook ${success ? 'installed' : 'installation failed'}`,
        icon: success ? React.createElement(Icons.Success) : React.createElement(Icons.Error),
        color: success ? 'text-green-400' : 'text-red-400',
      };

    default:
      return {
        title: activity_type.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase()),
        description: 'Activity completed',
        icon: React.createElement(Icons.Git),
        color: 'text-gray-400',
      };
  }
}

/**
 * Helper function to determine provider from repository name or URL
 */
function getProviderFromRepository(repository: string): string {
  const repoLower = repository.toLowerCase();
  
  if (repoLower.includes('gitlab') || repoLower.includes('gl-')) {
    return 'gitlab';
  }
  if (repoLower.includes('github') || repoLower.includes('gh-')) {
    return 'github';
  }
  if (repoLower.includes('bitbucket') || repoLower.includes('bb-')) {
    return 'bitbucket';
  }
  
  // Default to git for generic repositories
  return 'git';
}

/**
 * Helper function to get provider-specific icons
 */
function getProviderIcon(provider: string): React.ReactElement {
  const providerLower = provider.toLowerCase();
  
  switch (providerLower) {
    case 'gitlab':
      return React.createElement(Icons.GitLab);
    case 'github':
      return React.createElement(Icons.GitHub);
    case 'bitbucket':
      return React.createElement(Icons.Bitbucket);
    case 'git':
    default:
      return React.createElement(Icons.Git);
  }
}

/**
 * Helper function to capitalize provider names properly
 */
function capitalizeProvider(provider: string): string {
  const providerLower = provider.toLowerCase();
  
  switch (providerLower) {
    case 'gitlab':
      return 'GitLab';
    case 'github':
      return 'GitHub';
    case 'bitbucket':
      return 'Bitbucket';
    default:
      return provider.charAt(0).toUpperCase() + provider.slice(1);
  }
}
