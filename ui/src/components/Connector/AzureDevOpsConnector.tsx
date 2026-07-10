import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import { useNavigate, Routes, Route, Navigate } from 'react-router-dom';
import ManualAzureDevOpsConnector from './ManualAzureDevOpsConnector';

const AzureDevOpsConnector: React.FC = () => {
    const navigate = useNavigate();

    return (
        <div className="space-y-4">
            <div className="flex items-center">
                <Button variant="ghost" icon={<Icons.Add />} onClick={() => navigate('/git')} iconPosition="left" className="text-sm">
                    Back to providers
                </Button>
            </div>

            {/* Info about Azure DevOps connection */}
            <div className="mb-4 rounded-md bg-blue-900 border border-blue-700 px-4 py-3">
                <div className="flex items-start">
                    <div className="ml-3 flex-1">
                        <h3 className="text-sm font-medium text-blue-200">Azure DevOps Connection</h3>
                        <div className="mt-1 text-sm text-blue-300">
                            Currently only manual PAT (Personal Access Token) connection is supported for Azure DevOps.
                            OAuth support will be added in a future update.
                        </div>
                    </div>
                </div>
            </div>

            <Routes>
                <Route index element={<Navigate to="manual" replace />} />
                <Route path="manual" element={<ManualAzureDevOpsConnector />} />
                <Route path="*" element={<Navigate to="manual" replace />} />
            </Routes>
        </div>
    );
};

export default AzureDevOpsConnector;
