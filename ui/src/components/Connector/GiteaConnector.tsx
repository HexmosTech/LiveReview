import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import { useNavigate, Routes, Route, Navigate } from 'react-router-dom';
import ManualGiteaConnector from './ManualGiteaConnector';

const GiteaConnector: React.FC = () => {
    const navigate = useNavigate();

    return (
        <div className="space-y-4">
            <div className="flex items-center">
                <Button variant="ghost" icon={<Icons.Add />} onClick={() => navigate('/git')} iconPosition="left" className="text-sm">
                    Back to providers
                </Button>
            </div>

            <Routes>
                <Route index element={<Navigate to="manual" replace />} />
                <Route path="manual" element={<ManualGiteaConnector />} />
                <Route path="*" element={<Navigate to="manual" replace />} />
            </Routes>
        </div>
    );
};

export default GiteaConnector;
