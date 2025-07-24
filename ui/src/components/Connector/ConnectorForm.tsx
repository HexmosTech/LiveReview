import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import ProviderSelection from './ProviderSelection';
import GitLabComConnector from './GitLabComConnector';
import GitLabSelfHostedConnector from './GitLabSelfHostedConnector';
import GitHubConnector from './GitHubConnector';

const ConnectorForm: React.FC = () => {
    return (
        <Routes>
            <Route index element={<ProviderSelection />} />
            <Route path="gitlab-com/*" element={<GitLabComConnector />} />
            <Route path="gitlab-self-hosted/*" element={<GitLabSelfHostedConnector />} />
            <Route path="github/*" element={<GitHubConnector />} />
            <Route path="*" element={<Navigate to="/git" replace />} />
        </Routes>
    );
};

export default ConnectorForm;
