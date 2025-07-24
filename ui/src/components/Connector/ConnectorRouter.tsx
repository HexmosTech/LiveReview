import React from 'react';
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import ProviderSelection from './ProviderSelection';
import ManualGitLabCom from './ManualGitLabCom';
import ManualSelfHostedGitLab from './ManualSelfHostedGitLab';

const ConnectorRouter: React.FC = () => {
    const location = useLocation();
    
    return (
        <Routes>
            <Route path="/" element={<ProviderSelection />} />
            <Route path="/gitlab-com" element={<ManualGitLabCom />} />
            <Route path="/gitlab-com/step1" element={<ManualGitLabCom />} />
            <Route path="/gitlab-self-hosted" element={<ManualSelfHostedGitLab />} />
            <Route path="/gitlab-self-hosted/step1" element={<ManualSelfHostedGitLab />} />
            {/* Add more routes for automated, etc. */}
            <Route path="*" element={<Navigate to="/git" replace />} />
        </Routes>
    );
};

export default ConnectorRouter;
