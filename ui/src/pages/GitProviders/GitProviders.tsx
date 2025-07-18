import React from 'react';
import { ConnectorForm, ConnectorData } from '../../components/Connector/ConnectorForm';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { addConnector, ConnectorType } from '../../store/Connector/reducer';
import { 
    PageHeader, 
    Card, 
    EmptyState, 
    Button, 
    Icons, 
    Badge,
    Avatar
} from '../../components/UIPrimitives';

// Spec for GitProviderKit
// This system will manage Git providers, allowing users to add, edit, and remove Git provider configurations.
// It will include components for:
// - Listing all configured Git providers.
// - Adding a new Git provider.
// - Editing an existing Git provider.
// - Removing a Git provider.
// - Validating Git provider credentials.
//
// The system will also include a context provider for managing state and a set of utility functions for interacting with APIs.
//
// Components:
// - GitProviderList: Displays a list of all configured Git providers.
// - GitProviderForm: A form for adding or editing a Git provider.
// - GitProviderItem: Represents a single Git provider in the list.
//
// Context:
// - GitProviderContext: Provides state and actions for managing Git providers.
//
// Utilities:
// - validateGitProvider: Validates the credentials of a Git provider.
// - apiClient: A utility for making API requests related to Git providers.
//
// Update to GitProviderForm component spec:
// - Add a "Connector Type" dropdown with two options: "Gitlab.com" and "Self-Hosted Gitlab".
// - For "Gitlab.com":
//   - The URL field is pre-filled with "https://gitlab.com" and is not editable.
// - For "Self-Hosted Gitlab":
//   - The URL field has a placeholder text like "https://gitlab.mycompany.com" and is editable.

const GitProviders: React.FC = () => {
    const dispatch = useAppDispatch();
    const connectors = useAppSelector((state) => state.Connector.connectors);

    const handleAddConnector = (connectorData: ConnectorData) => {
        dispatch(addConnector(connectorData));
    };

    const formatConnectorType = (type: ConnectorType) => {
        switch (type) {
            case 'gitlab-com':
                return 'GitLab.com';
            case 'gitlab-self-hosted':
                return 'Self-Hosted GitLab';
            default:
                return type;
        }
    };

    const getProviderIcon = (type: ConnectorType) => {
        switch (type) {
            case 'gitlab-com':
            case 'gitlab-self-hosted':
                return <Icons.GitLab />;
            case 'github':
                return <Icons.GitHub />;
            default:
                return <Icons.Git />;
        }
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <PageHeader 
                title="Git Providers" 
                description="Connect and manage your Git repositories"
            />
            
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                <div>
                    <ConnectorForm onSubmit={handleAddConnector} />
                    
                    {/* Brand Showcase */}
                    <div className="mt-6 card-brand rounded-lg bg-white p-5 shadow-sm">
                        <h3 className="text-lg font-medium text-gray-900 mb-3">Why Connect Git Providers?</h3>
                        <div className="flex items-center">
                            <img src={require("../../../assets/logo.svg")} alt="LiveReview Eye" className="h-12 w-auto mr-4 logo-animation" />
                            <p className="text-sm text-gray-500">
                                LiveReview connects to your repositories to provide real-time AI-powered code reviews, 
                                helping your team ship better code faster.
                            </p>
                        </div>
                    </div>
                </div>
                
                <div>
                    <Card 
                        title="Your Connectors" 
                        badge={`${connectors.length}`}
                    >
                        {connectors.length === 0 ? (
                            <EmptyState
                                icon={<Icons.EmptyState />}
                                title="No connectors yet"
                                description="Create your first connector to start integrating with your code repositories"
                            />
                        ) : (
                            <ul className="space-y-4">
                                {connectors.map((connector) => (
                                    <li
                                        key={connector.id}
                                        className="border border-gray-100 rounded-lg hover:bg-gray-50 transition-colors"
                                    >
                                        <div className="p-4">
                                            <div className="flex justify-between items-center">
                                                <div className="flex items-center">
                                                    <div className="flex-shrink-0 mr-3">
                                                        <Avatar 
                                                            size="md"
                                                            initials={connector.name.charAt(0).toUpperCase()}
                                                        />
                                                    </div>
                                                    <div>
                                                        <div className="flex items-center">
                                                            <h3 className="font-medium text-gray-900">
                                                                {connector.name}
                                                            </h3>
                                                            <Badge 
                                                                variant="primary" 
                                                                size="sm" 
                                                                className="ml-2"
                                                            >
                                                                {formatConnectorType(connector.type)}
                                                            </Badge>
                                                        </div>
                                                        <p className="text-sm text-gray-500">
                                                            {connector.url}
                                                        </p>
                                                    </div>
                                                </div>
                                                <div className="flex items-center space-x-2">
                                                    {connector.createdAt && (
                                                        <span className="text-xs text-gray-500">
                                                            {new Date(connector.createdAt).toLocaleDateString()}
                                                        </span>
                                                    )}
                                                    <Button
                                                        variant="secondary"
                                                        size="sm"
                                                        onClick={() => {
                                                            alert(`Testing connection to ${connector.name}`);
                                                        }}
                                                    >
                                                        Test Connection
                                                    </Button>
                                                </div>
                                            </div>
                                        </div>
                                    </li>
                                ))}
                            </ul>
                        )}
                    </Card>
                    
                    {connectors.length > 0 && (
                        <Card title="Connection Status" className="mt-6">
                            <div className="flex items-center justify-between">
                                <div className="flex items-center">
                                    <span className="flex h-3 w-3 relative mr-2">
                                        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                                        <span className="relative inline-flex rounded-full h-3 w-3 bg-green-500"></span>
                                    </span>
                                    <p className="text-sm text-gray-700">All connectors working properly</p>
                                </div>
                                <Button 
                                    variant="outline" 
                                    size="sm"
                                    icon={<Icons.Dashboard />}
                                >
                                    View Details
                                </Button>
                            </div>
                        </Card>
                    )}
                </div>
            </div>
        </div>
    );
};

export default GitProviders;
