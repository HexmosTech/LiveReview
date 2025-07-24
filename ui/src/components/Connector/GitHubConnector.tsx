import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import { useNavigate, useLocation, Routes, Route, Navigate } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { getConnectors } from '../../api/connectors';
import ManualGitHubConnector from './ManualGitHubConnector';

const GitHubConnector: React.FC = () => {
    const navigate = useNavigate();
    const location = useLocation();
    const dispatch = useDispatch();
    
    // More precise path detection
    const currentPath = location.pathname;
    const isManual = currentPath.endsWith('/manual');
    
    console.log('Current path:', currentPath, 'isManual:', isManual);

    const handleManualSubmit = async (data: any) => {
        try {
            // Refresh connector list and update Redux state
            const updatedConnectorsRaw = await getConnectors();
            const updatedConnectors = updatedConnectorsRaw.map((c: any) => ({
                id: c.id?.toString() || '',
                name: c.connection_name || '',
                type: c.provider || '',
                url: c.provider_url || '',
                apiKey: c.provider_app_id || '',
                createdAt: c.created_at || '',
                metadata: c.metadata || {},
            }));
            dispatch(setConnectors(updatedConnectors));
            navigate('/git');
        } catch (err: any) {
            console.error('Failed to update connectors:', err);
        }
    };

    return (
        <div className="space-y-4">
            <div className="flex items-center">
                <Button variant="ghost" icon={<Icons.Add />} onClick={() => navigate('/git')} iconPosition="left" className="text-sm">
                    Back to providers
                </Button>
            </div>
            
            {/* Info about GitHub connection */}
            <div className="mb-4 rounded-md bg-blue-900 border border-blue-700 px-4 py-3">
                <div className="flex items-start">
                    <div className="ml-3 flex-1">
                        <h3 className="text-sm font-medium text-blue-200">GitHub Connection</h3>
                        <div className="mt-1 text-sm text-blue-300">
                            Currently only manual PAT (Personal Access Token) connection is supported for GitHub. 
                            OAuth support will be added in a future update.
                        </div>
                    </div>
                </div>
            </div>

            <Routes>
                <Route index element={<Navigate to="manual" replace />} />
                <Route path="manual" element={<ManualGitHubConnector />} />
                <Route path="*" element={<Navigate to="manual" replace />} />
            </Routes>
        </div>
    );
};

export default GitHubConnector;
