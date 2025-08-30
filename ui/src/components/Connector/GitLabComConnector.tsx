import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import { useNavigate, useLocation, Routes, Route, Navigate } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { getConnectors } from '../../api/connectors';
import ManualGitLabCom from './ManualGitLabCom';
import GitLabConnector from './GitLabConnector';

const GitLabComConnector: React.FC = () => {
    const navigate = useNavigate();
    const location = useLocation();
    const dispatch = useDispatch();
    
    // More precise path detection
    const currentPath = location.pathname;
    const isManual = currentPath.endsWith('/manual');
    const isAutomated = currentPath.endsWith('/automated');
    
    console.log('Current path:', currentPath, 'isManual:', isManual, 'isAutomated:', isAutomated);

    const handleAutomatedSubmit = async (data: any) => {
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
            
            {/* Tab Switcher */}
            <div className="flex space-x-2 mb-4">
                <Button
                    variant={isManual ? 'primary' : 'outline'}
                    onClick={() => navigate('/git/gitlab-com/manual')}
                >
                    Manual
                </Button>
                <Button
                    variant={isAutomated ? 'primary' : 'outline'}
                    onClick={() => navigate('/git/gitlab-com/automated')}
                >
                    Automated
                </Button>
            </div>

            <Routes>
                <Route index element={<Navigate to="manual" replace />} />
                <Route path="manual" element={<ManualGitLabCom />} />
                <Route path="automated" element={
                    <GitLabConnector 
                        type="gitlab-com" 
                        onSubmit={handleAutomatedSubmit}
                        disableRouting={true}
                    />
                } />
                <Route path="*" element={<Navigate to="manual" replace />} />
            </Routes>
        </div>
    );
};

export default GitLabComConnector;
