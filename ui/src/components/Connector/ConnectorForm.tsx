import React from 'react';
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import ProviderSelection from './ProviderSelection';
import GitLabComConnector from './GitLabComConnector';
import GitLabSelfHostedConnector from './GitLabSelfHostedConnector';
import GitHubConnector from './GitHubConnector';
import GitHubAppConnector from './GitHubAppConnector';
import BitbucketConnector from './BitbucketConnector';
import GiteaConnector from './GiteaConnector';
import AzureDevOpsConnector from './AzureDevOpsConnector';
import ConnectorLayout from './ConnectorLayout';

const ConnectorForm: React.FC = () => {
    const isGitHubApp = useLocation().pathname.startsWith('/git/github-app');
    return (
        // Props stay undefined off the GitHub App page: spreading `false` crashes React 19 dev builds.
        <ConnectorLayout
            title={isGitHubApp ? 'GitHub App' : undefined}
            description={isGitHubApp ? 'Choose which GitHub accounts and repositories LiveReview can review' : undefined}
        >
            <Routes>
                <Route index element={<ProviderSelection />} />
                <Route path="gitlab-com/*" element={<GitLabComConnector />} />
                <Route path="gitlab-self-hosted/*" element={<GitLabSelfHostedConnector />} />
                <Route path="github-app/*" element={<GitHubAppConnector />} />
                <Route path="github/*" element={<GitHubConnector />} />
                <Route path="bitbucket/*" element={<BitbucketConnector />} />
                <Route path="gitea/*" element={<GiteaConnector />} />
                <Route path="azuredevops/*" element={<AzureDevOpsConnector />} />
                <Route path="*" element={<Navigate to="/git" replace />} />
            </Routes>
        </ConnectorLayout>
    );
};

export default ConnectorForm;
