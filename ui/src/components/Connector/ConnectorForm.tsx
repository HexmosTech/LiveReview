import React, { useState, useEffect } from 'react';
import { Card, Input, Select, Button, Icons } from '../UIPrimitives';
import { ConnectorType } from '../../store/Connector/reducer';
import GitLabConnector from './GitLabConnector';
import DomainValidator from './DomainValidator';

type ConnectorFormProps = {
    onSubmit: (connector: ConnectorData) => void;
};

export type ConnectorData = {
    name: string;
    type: ConnectorType;
    url: string;
    apiKey: string;
    id?: string;
    createdAt?: number;
};

export const ConnectorForm: React.FC<ConnectorFormProps> = ({ onSubmit }) => {
    const [selectedConnectorType, setSelectedConnectorType] = useState<ConnectorType>('gitlab-com');
    const [showConnectorForm, setShowConnectorForm] = useState<boolean>(false);
    
    // Check URL fragment on mount to determine view state
    useEffect(() => {
        const fragment = window.location.hash;
        
        // If URL contains a specific connector type, show that connector's form
        if (fragment.includes('/gitlab-com')) {
            setSelectedConnectorType('gitlab-com');
            setShowConnectorForm(true);
        } else if (fragment.includes('/gitlab-self-hosted')) {
            setSelectedConnectorType('gitlab-self-hosted');
            setShowConnectorForm(true);
        } else {
            // Reset to provider selection if no connector type in URL
            setShowConnectorForm(false);
        }
        
        // Handle browser navigation (back/forward buttons)
        const handlePopState = () => {
            const currentFragment = window.location.hash;
            
            if (currentFragment.includes('/gitlab-com')) {
                setSelectedConnectorType('gitlab-com');
                setShowConnectorForm(true);
            } else if (currentFragment.includes('/gitlab-self-hosted')) {
                setSelectedConnectorType('gitlab-self-hosted');
                setShowConnectorForm(true);
            } else if (currentFragment === '#git') {
                setShowConnectorForm(false);
            }
        };
        
        window.addEventListener('popstate', handlePopState);
        
        return () => {
            window.removeEventListener('popstate', handlePopState);
        };
    }, []);
    
    const handleConnectorSelect = (type: ConnectorType) => {
        setSelectedConnectorType(type);
        setShowConnectorForm(true);
        
        // Update URL fragment to reflect selected connector
        window.history.pushState(
            null, 
            '', 
            `#git/gitlab-${type}/step1`
        );
    };

    const handleGitLabSubmit = (data: ConnectorData) => {
        // Add ID and timestamp
        const connectorWithMeta = {
            ...data,
            id: `connector-${Date.now()}`,
            createdAt: Date.now(),
        };
        
        onSubmit(connectorWithMeta);
        setShowConnectorForm(false);
    };

    const handleBackToSelection = () => {
        setShowConnectorForm(false);
        
        // Update URL fragment to go back to provider selection
        window.history.pushState(
            null, 
            '', 
            `#git`
        );
    };

    // Show connector selection screen
    if (!showConnectorForm) {
        return (
            <DomainValidator>
                <Card title="Create New Connector">
                    <div className="space-y-5">
                        <h3 className="text-lg font-medium text-white">Select Git Provider</h3>
                        <p className="text-slate-300 text-sm">Choose a Git provider to connect with LiveReview</p>
                        
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                            <Button 
                                variant="outline" 
                                icon={<Icons.GitLab />}
                                className="h-24 flex-col"
                                onClick={() => handleConnectorSelect('gitlab-com')}
                            >
                                <span className="text-base mt-2">GitLab.com</span>
                            </Button>
                            
                            <Button 
                                variant="outline" 
                                icon={<Icons.GitLab />}
                                className="h-24 flex-col"
                                onClick={() => handleConnectorSelect('gitlab-self-hosted')}
                            >
                                <span className="text-base mt-2">Self-Hosted GitLab</span>
                            </Button>
                            
                            <Button 
                                variant="outline" 
                                icon={<Icons.GitHub />}
                                className="h-24 flex-col"
                                disabled
                            >
                                <span className="text-base mt-2">GitHub</span>
                                <span className="text-xs mt-1">Coming Soon</span>
                            </Button>
                            
                            <Button 
                                variant="outline" 
                                icon={<Icons.Git />}
                                className="h-24 flex-col"
                                disabled
                            >
                                <span className="text-base mt-2">Custom</span>
                                <span className="text-xs mt-1">Coming Soon</span>
                            </Button>
                        </div>
                    </div>
                </Card>
            </DomainValidator>
        );
    }

    // Show specific connector form based on type
    if (selectedConnectorType === 'gitlab-com' || selectedConnectorType === 'gitlab-self-hosted') {
        return (
            <div className="space-y-4">
                <div className="flex items-center">
                    <Button
                        variant="ghost"
                        icon={<Icons.Add />}
                        onClick={handleBackToSelection}
                        iconPosition="left"
                        className="text-sm"
                    >
                        Back to providers
                    </Button>
                </div>
                <GitLabConnector 
                    type={selectedConnectorType}
                    onSubmit={handleGitLabSubmit}
                />
            </div>
        );
    }

    // Placeholder for other connector types (GitHub, Custom, etc.)
    return (
        <Card title="Coming Soon">
            <div className="space-y-4">
                <p className="text-slate-300">This connector type is not yet available.</p>
                <Button
                    variant="primary"
                    onClick={handleBackToSelection}
                >
                    Back to Selection
                </Button>
            </div>
        </Card>
    );
};
