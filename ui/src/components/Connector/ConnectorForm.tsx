import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import ProviderSelection from './ProviderSelection';
import GitLabComConnector from './GitLabComConnector';
import GitLabSelfHostedConnector from './GitLabSelfHostedConnector';
import GitHubConnector from './GitHubConnector';
import BitbucketConnector from './BitbucketConnector';
import ConnectorLayout from './ConnectorLayout';

const ConnectorForm: React.FC = () => {
    return (
        <ConnectorLayout>
            <Routes>
                <Route index element={<ProviderSelection />} />
                <Route path="gitlab-com/*" element={<GitLabComConnector />} />
                <Route path="gitlab-self-hosted/*" element={<GitLabSelfHostedConnector />} />
                <Route path="github/*" element={<GitHubConnector />} />
                <Route path="bitbucket/*" element={<BitbucketConnector />} />
                <Route path="*" element={<Navigate to="/git" replace />} />
            </Routes>
        </ConnectorLayout>
    );
};

export default ConnectorForm;
