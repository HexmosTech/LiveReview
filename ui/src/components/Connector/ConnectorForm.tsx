import React from 'react';
import { useLocation } from 'react-router-dom';
import ProviderSelection from './ProviderSelection';
import ManualGitLabCom from './ManualGitLabCom';
import ManualSelfHostedGitLab from './ManualSelfHostedGitLab';

const ConnectorForm: React.FC = () => {
    const location = useLocation();
    const path = location.pathname;
    
    // Route based on current path
    if (path === '/git') {
        return <ProviderSelection />;
    } else if (path.includes('/git/gitlab-com')) {
        return <ManualGitLabCom />;
    } else if (path.includes('/git/gitlab-self-hosted')) {
        return <ManualSelfHostedGitLab />;
    } else {
        return <ProviderSelection />;
    }
};

export default ConnectorForm;
