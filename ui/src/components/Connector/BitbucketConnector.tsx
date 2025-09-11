import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import { useNavigate, Routes, Route, Navigate } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { setConnectors } from '../../store/Connector/reducer';
import { getConnectors } from '../../api/connectors';
import ManualBitbucketConnector from './ManualBitbucketConnector';

const BitbucketConnector: React.FC = () => {
    const navigate = useNavigate();
    const dispatch = useDispatch();

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
            

            <Routes>
                <Route index element={<Navigate to="manual" replace />} />
                <Route path="manual" element={<ManualBitbucketConnector />} />
                <Route path="*" element={<Navigate to="manual" replace />} />
            </Routes>
        </div>
    );
};

export default BitbucketConnector;
