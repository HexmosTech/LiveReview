import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAppSelector } from '../../store/configureStore';
import { 
    PageHeader, 
    Card, 
    Button, 
    Icons, 
    Badge,
    Avatar,
    Spinner,
    Alert
} from '../../components/UIPrimitives';
import { Connector } from '../../store/Connector/reducer';
import { deleteConnector } from '../../api/connectors';

const ConnectorDetails: React.FC = () => {
    const { connectorId } = useParams<{ connectorId: string }>();
    const navigate = useNavigate();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    const [connector, setConnector] = useState<Connector | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [isDeleting, setIsDeleting] = useState(false);

    useEffect(() => {
        if (connectorId && connectors.length > 0) {
            const foundConnector = connectors.find(c => c.id === connectorId);
            if (foundConnector) {
                setConnector(foundConnector);
                setError(null);
            } else {
                setError('Connector not found');
            }
            setIsLoading(false);
        } else if (connectors.length === 0) {
            // Still loading connectors
            setIsLoading(true);
        }
    }, [connectorId, connectors]);

    const formatConnectorType = (type: string) => {
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

    const getProviderIcon = (type: string) => {
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

    const handleDeleteConnector = async () => {
        if (!connector) return;
        
        if (!confirm(`Are you sure you want to delete "${connector.name}"? This action cannot be undone.`)) {
            return;
        }

        try {
            setIsDeleting(true);
            await deleteConnector(connector.id);
            navigate('/git');
        } catch (err) {
            console.error('Error deleting connector:', err);
            setError('Failed to delete connector. Please try again.');
        } finally {
            setIsDeleting(false);
        }
    };

    const handleTestConnection = () => {
        if (connector) {
            alert(`Testing connection to ${connector.name}`);
        }
    };

    if (isLoading) {
        return (
            <div className="container mx-auto px-4 py-8">
                <div className="flex justify-center items-center py-8">
                    <Spinner size="md" color="text-blue-400" />
                    <span className="ml-3 text-slate-300">Loading connector details...</span>
                </div>
            </div>
        );
    }

    if (error || !connector) {
        return (
            <div className="container mx-auto px-4 py-8">
                <div className="flex items-center mb-6">
                    <Button 
                        variant="ghost" 
                        icon={<Icons.Dashboard />} 
                        onClick={() => navigate('/git')} 
                        iconPosition="left" 
                        className="text-sm"
                    >
                        Back to Git Providers
                    </Button>
                </div>
                <Alert
                    variant="error"
                    icon={<Icons.Error />}
                >
                    {error || 'Connector not found'}
                </Alert>
            </div>
        );
    }

    return (
        <div className="container mx-auto px-4 py-8">
            {/* Header with back button */}
            <div className="flex items-center mb-6">
                <Button 
                    variant="ghost" 
                    icon={<Icons.Dashboard />} 
                    onClick={() => navigate('/git')} 
                    iconPosition="left" 
                    className="text-sm"
                >
                    Back to Git Providers
                </Button>
            </div>

            <PageHeader 
                title={connector.name}
                description={`${formatConnectorType(connector.type)} connection details and management`}
            />

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                {/* Main connector info */}
                <div className="lg:col-span-2">
                    <Card title="Connection Information">
                        <div className="space-y-6">
                            {/* Basic Info */}
                            <div className="flex items-center">
                                <div className="flex-shrink-0 mr-4">
                                    <Avatar 
                                        size="lg"
                                        initials={connector.name.charAt(0).toUpperCase()}
                                    />
                                </div>
                                <div className="flex-grow">
                                    <div className="flex items-center mb-2">
                                        <h3 className="text-xl font-semibold text-white mr-3">
                                            {connector.name}
                                        </h3>
                                        <Badge variant="primary" size="md">
                                            {formatConnectorType(connector.type)}
                                        </Badge>
                                    </div>
                                    <p className="text-slate-300 font-mono text-sm">
                                        {connector.url}
                                    </p>
                                </div>
                            </div>

                            {/* Connection Details */}
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-1">
                                        Provider Type
                                    </label>
                                    <div className="flex items-center">
                                        {getProviderIcon(connector.type)}
                                        <span className="ml-2 text-slate-200">
                                            {formatConnectorType(connector.type)}
                                        </span>
                                    </div>
                                </div>
                                
                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-1">
                                        Created
                                    </label>
                                    <span className="text-slate-200">
                                        {connector.createdAt ? 
                                            new Date(connector.createdAt).toLocaleDateString() : 
                                            'Unknown'
                                        }
                                    </span>
                                </div>

                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-1">
                                        Connection ID
                                    </label>
                                    <span className="text-slate-200 font-mono text-sm">
                                        {connector.id}
                                    </span>
                                </div>

                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-1">
                                        API Key (Last 4 chars)
                                    </label>
                                    <span className="text-slate-200 font-mono text-sm">
                                        {connector.apiKey ? 
                                            '****' + connector.apiKey.slice(-4) : 
                                            'Not available'
                                        }
                                    </span>
                                </div>
                            </div>

                            {/* Metadata */}
                            {connector.metadata && Object.keys(connector.metadata).length > 0 && (
                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-2">
                                        Additional Information
                                    </label>
                                    <div className="bg-slate-800 rounded-lg p-3">
                                        <pre className="text-xs text-slate-300 overflow-x-auto">
                                            {JSON.stringify(connector.metadata, null, 2)}
                                        </pre>
                                    </div>
                                </div>
                            )}
                        </div>
                    </Card>

                    {/* Repository Access - Placeholder */}
                    <Card title="Repository Access" className="mt-6">
                        <div className="text-center py-8">
                            <Icons.EmptyState />
                            <p className="text-slate-400 mt-4">
                                Repository scope and access information will be displayed here.
                            </p>
                            <p className="text-slate-500 text-sm mt-2">
                                This feature is coming soon.
                            </p>
                        </div>
                    </Card>
                </div>

                {/* Actions sidebar */}
                <div>
                    <Card title="Actions">
                        <div className="space-y-3">
                            <Button
                                variant="primary"
                                size="md"
                                onClick={handleTestConnection}
                                className="w-full"
                                icon={<Icons.Success />}
                            >
                                Test Connection
                            </Button>
                            
                            <Button
                                variant="outline"
                                size="md"
                                onClick={() => alert('Edit functionality coming soon')}
                                className="w-full"
                                icon={<Icons.Edit />}
                            >
                                Edit Connection
                            </Button>
                            
                            <Button
                                variant="outline"
                                size="md"
                                onClick={() => alert('Disable functionality coming soon')}
                                className="w-full"
                                icon={<Icons.Warning />}
                            >
                                Disable Connection
                            </Button>
                            
                            <div className="border-t border-slate-600 pt-3 mt-4">
                                <Button
                                    variant="outline"
                                    size="md"
                                    onClick={handleDeleteConnector}
                                    disabled={isDeleting}
                                    className="w-full !text-red-400 hover:!text-red-300 hover:!border-red-400"
                                    icon={isDeleting ? <Spinner size="sm" color="text-red-400" /> : <Icons.Delete />}
                                >
                                    {isDeleting ? 'Deleting...' : 'Delete Connection'}
                                </Button>
                            </div>
                        </div>
                    </Card>

                    {/* Activity - Placeholder */}
                    <Card title="Recent Activity" className="mt-6">
                        <div className="text-center py-8">
                            <Icons.Info />
                            <p className="text-slate-400 mt-4">
                                Connection activity and usage statistics will be displayed here.
                            </p>
                            <p className="text-slate-500 text-sm mt-2">
                                This feature is coming soon.
                            </p>
                        </div>
                    </Card>
                </div>
            </div>
        </div>
    );
};

export default ConnectorDetails;
