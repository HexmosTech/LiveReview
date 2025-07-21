import React, { useState, useEffect } from 'react';
import { Card, Input, Select, Button, Icons, Alert } from '../UIPrimitives';
import { ConnectorType } from '../../store/Connector/reducer';
import GitLabConnector from './GitLabConnector';
import DomainValidator from './DomainValidator';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import { isDuplicateConnector, normalizeUrl } from './checkConnectorDuplicate';

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
    const navigate = useNavigate();
    const location = useLocation();
    const { providerType } = useParams<{ providerType?: string }>();
    
    const [selectedConnectorType, setSelectedConnectorType] = useState<ConnectorType>('gitlab-com');
    const [showConnectorForm, setShowConnectorForm] = useState<boolean>(false);
    const [errorMessage, setErrorMessage] = useState<string | null>(null);
    
    // Get connectors from Redux state
    const connectors = useAppSelector((state) => state.Connector.connectors);
    
  // Check URL path on mount to determine view state
  useEffect(() => {
    // If URL contains a specific connector type, show that connector's form
    if (providerType === 'gitlab-com') {
      setSelectedConnectorType('gitlab-com');
      setShowConnectorForm(true);
    } else if (providerType === 'gitlab-self-hosted') {
      setSelectedConnectorType('gitlab-self-hosted');
      setShowConnectorForm(true);
    } else {
      // Reset to provider selection if no connector type in URL
      setShowConnectorForm(false);
    }
    }, [providerType]);
    const handleConnectorSelect = (type: ConnectorType) => {
        if (type === 'gitlab-com') {
            // Check if any connector has gitlab.com in provider_url
            const hasGitlabComConnector = connectors.some((connector: any) => {
                const connectorUrl = normalizeUrl(connector.url || '');
                const metadataProviderUrl = connector.metadata?.provider_url ? 
                    normalizeUrl(connector.metadata.provider_url) : '';
                
                return connectorUrl.includes('gitlab.com') || metadataProviderUrl.includes('gitlab.com');
            });

            console.log("The connectors are: ", connectors);
            
            if (hasGitlabComConnector) {
                // Show error message instead of alert
                setErrorMessage("You already have a GitLab.com connection");
                return; // Don't proceed with the connection
            }
            
            // Clear any previous error
            setErrorMessage(null);
        }
        setSelectedConnectorType(type);
        setShowConnectorForm(true);
        // Navigate to the connector setup page using React Router
        navigate(`/git/${type}/step1`);
    }
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
        
        // Navigate back to provider selection using React Router
        navigate('/git');
    };

    // Show connector selection screen
    if (!showConnectorForm) {
        return (
            <DomainValidator>
                <Card title="Create New Connector">
                    <div className="space-y-5">
                        <h3 className="text-lg font-medium text-white">Select Git Provider</h3>
                        <p className="text-slate-300 text-sm">Choose a Git provider to connect with LiveReview</p>
                        
                        {errorMessage && (
                            <Alert 
                                variant="error" 
                                title="Connection Error"
                                onClose={() => setErrorMessage(null)}
                            >
                                {errorMessage}
                            </Alert>
                        )}
                        
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
