import React, { useState, useEffect } from 'react';
import { Routes, Route, useNavigate } from 'react-router-dom';
import ConnectorForm from '../../components/Connector/ConnectorForm';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { addConnector, setConnectors, ConnectorType, Connector } from '../../store/Connector/reducer';
import { 
    PageHeader, 
    Card, 
    EmptyState, 
    Button, 
    Icons, 
    Badge,
    Avatar,
    Spinner
} from '../../components/UIPrimitives';
import { getConnectors, ConnectorResponse, deleteConnector } from '../../api/connectors';
import ConnectorDetails from './ConnectorDetails';

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
    const navigate = useNavigate();
    const storeConnectors = useAppSelector((state) => state.Connector.connectors);
    
    return (
        <Routes>
            <Route index element={<GitProvidersList />} />
            <Route path="connector/:connectorId" element={<ConnectorDetails />} />
        </Routes>
    );
};

const GitProvidersList: React.FC = () => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();
    const storeConnectors = useAppSelector((state) => state.Connector.connectors);
    
        // Use redux state only for connectors
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    
    // Fetch connectors from API on component mount
    useEffect(() => {
        const fetchConnectors = async () => {
            try {
                setIsLoading(true);
                const data = await getConnectors();
                // Transform API data to Connector[] shape
                const connectorsFromApi = data.map(apiConnector => {
                    const connectorType = apiConnector.provider as ConnectorType;
                    return {
                        id: apiConnector.id.toString(),
                        name: apiConnector.connection_name || `${apiConnector.provider} Connection`,
                        type: connectorType,
                        url: apiConnector.provider_url || (apiConnector.metadata && apiConnector.metadata.provider_url) || '',
                        apiKey: apiConnector.provider_app_id || '',
                        createdAt: apiConnector.created_at,
                        metadata: apiConnector.metadata || {} // Store the complete metadata
                    };
                });
                dispatch(setConnectors(connectorsFromApi));
                setError(null);
            } catch (err) {
                console.error('Error fetching connectors:', err);
                setError('Failed to load connectors. Please try again.');
            } finally {
                setIsLoading(false);
            }
        };
        fetchConnectors();
    }, []);
    
        // Use connectors from redux state only
        const connectors = storeConnectors;

    const handleDeleteConnector = async (connectorId: string, connectorName: string) => {
        if (!confirm(`Are you sure you want to delete "${connectorName}"? This action cannot be undone.`)) {
            return;
        }

        try {
            setIsLoading(true);
            await deleteConnector(connectorId);
            
            // Update the redux state by removing the deleted connector
            const updatedConnectors = connectors.filter(c => c.id !== connectorId);
            dispatch(setConnectors(updatedConnectors));
            
            setError(null);
        } catch (err) {
            console.error('Error deleting connector:', err);
            setError('Failed to delete connector. Please try again.');
        } finally {
            setIsLoading(false);
        }
    };

    const formatConnectorType = (type: ConnectorType) => {
        switch (type) {
            case 'gitlab-com':
                return 'GitLab.com';
            case 'gitlab-self-hosted':
                return 'Self-Hosted GitLab';
            case 'gitlab':
                return 'GitLab';
            case 'github':
                return 'GitHub';
            case 'bitbucket':
                return 'Bitbucket';
            default:
                return type.charAt(0).toUpperCase() + type.slice(1);
        }
    };

    const getProviderIcon = (type: ConnectorType) => {
        switch (type) {
            case 'gitlab':
            case 'gitlab-com':
            case 'gitlab-self-hosted':
                return <Icons.GitLab />;
            case 'github':
                return <Icons.GitHub />;
            case 'bitbucket':
                return <Icons.Bitbucket />;
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
                    <ConnectorForm />
                    
                    {/* Brand Showcase */}
                    <div className="mt-6 card-brand rounded-lg bg-slate-700 border border-slate-600 p-5 shadow-md">
                        <h3 className="text-lg font-medium text-white mb-3">Why Connect Git Providers?</h3>
                        <div className="flex items-center">
                            <img src={require("../../../assets/logo.svg")} alt="LiveReview Eye" className="h-12 w-auto mr-4 logo-animation" />
                            <p className="text-sm text-slate-300">
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
                        {isLoading ? (
                            <div className="flex justify-center items-center py-8">
                                <Spinner size="md" color="text-blue-400" />
                                <span className="ml-3 text-slate-300">Loading connectors...</span>
                            </div>
                        ) : error ? (
                            <div className="p-4 text-center">
                                <Icons.Error />
                                <p className="text-red-400 mt-2">{error}</p>
                                <Button 
                                    variant="outline" 
                                    size="sm" 
                                    className="mt-3"
                                    onClick={() => window.location.reload()}
                                >
                                    Retry
                                </Button>
                            </div>
                        ) : connectors.length === 0 ? (
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
                                        className="border border-slate-600 rounded-lg bg-slate-700 hover:bg-slate-600 transition-colors"
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
                                                            <h3 className="font-medium text-white">
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
                                                        <p className="text-sm text-slate-300">
                                                            {connector.url}
                                                        </p>
                                                    </div>
                                                </div>
                                                <div className="flex items-center space-x-2">
                                                    {connector.createdAt && (
                                                        <span className="text-xs text-slate-300">
                                                            {new Date(connector.createdAt).toLocaleDateString()}
                                                        </span>
                                                    )}
                                                    <Button
                                                        variant="outline"
                                                        size="sm"
                                                        onClick={() => navigate(`/git/connector/${connector.id}`)}
                                                        title="Connector details"
                                                        className="!px-2.5"
                                                    >
                                                        <Icons.Settings />
                                                    </Button>
                                                    <Button
                                                        variant="outline"
                                                        size="sm"
                                                        onClick={() => handleDeleteConnector(connector.id, connector.name)}
                                                        disabled={isLoading}
                                                        title="Delete connector"
                                                        className="!px-2.5 !text-red-400 hover:!text-red-300 hover:!border-red-400"
                                                    >
                                                        <Icons.Delete />
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
                                    <p className="text-sm text-slate-200">All connectors working properly</p>
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
