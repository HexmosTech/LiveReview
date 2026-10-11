import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Repositories from './Repositories';
import MergeRequests from './MergeRequests';
import RepositorySettings from './RepositorySettings';

const ExploreRoutes: React.FC = () => {
    return (
        <Routes>
            <Route index element={<Navigate to="repositories" replace />} />
            <Route path="/repositories" element={<Repositories />} />
            <Route path="/repositories/:repoId/settings" element={<RepositorySettings />} />
            <Route path="/merge-requests" element={<MergeRequests />} />
            <Route path="*" element={<Navigate to="/explore/repositories" replace />} />
        </Routes>
    );
};

export default ExploreRoutes;
