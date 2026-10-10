import React from 'react';
import { SiGitlab } from 'react-icons/si';
import { Icons } from './UIPrimitives';

// Git provider logo; GitLab in its brand orange (the shared Icons.GitLab is monochrome).
const ProviderIcon: React.FC<{ provider: string }> = ({ provider }) => {
  const normalized = provider.toLowerCase();
  if (normalized.startsWith('github')) return <Icons.GitHub />;
  if (normalized.startsWith('gitlab')) return <SiGitlab className="w-5 h-5" style={{ color: '#FC6D26' }} />;
  if (normalized.startsWith('bitbucket')) return <Icons.Bitbucket />;
  if (normalized.startsWith('gitea')) return <Icons.Gitea />;
  if (normalized.startsWith('azuredevops')) return <Icons.AzureDevOps />;
  return null;
};

export default ProviderIcon;
