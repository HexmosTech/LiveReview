import React from 'react';
import { PageHeader, Card } from '../UIPrimitives';

interface ConnectorLayoutProps {
    children: React.ReactNode;
    title?: string;
    description?: string;
    showCard?: boolean;
}

const ConnectorLayout: React.FC<ConnectorLayoutProps> = ({ 
    children, 
    title = "Git Providers", 
    description = "Connect and manage your Git repositories",
    showCard = true
}) => {
    return (
        <div className="container mx-auto px-4 py-8">
            <div className="max-w-3xl mx-auto">
                <div className="text-center mb-8">
                    <h1 className="text-3xl font-bold text-white mb-2">{title}</h1>
                    <p className="text-slate-300">{description}</p>
                </div>
                
                {showCard ? (
                    <Card>
                        {children}
                    </Card>
                ) : (
                    children
                )}
            </div>
        </div>
    );
};

export default ConnectorLayout; 