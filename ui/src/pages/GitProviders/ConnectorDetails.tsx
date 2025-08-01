import React, { useState, useEffect, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { formatDistanceToNow, format } from 'date-fns';
import { useAppSelector } from '../../store/configureStore';
import { 
    PageHeader, 
    Card, 
    Button, 
    Icons, 
    Badge,
    Avatar,
    Spinner,
    Alert,
    Input
} from '../../components/UIPrimitives';
import { Connector } from '../../store/Connector/reducer';
import { deleteConnector, getRepositoryAccess, enableManualTriggerForAllProjects, ProjectWithStatus } from '../../api/connectors';
import type { RepositoryAccess } from '../../api/connectors';

// Extended interface to support both old and new API response formats
interface RepositoryAccessExtended {
    connector_id: number;
    provider: string;
    base_url: string;
    projects?: string[]; // Legacy field
    projects_with_status?: ProjectWithStatus[]; // New field
    project_count: number;
    error?: string;
    updated_at: string;
}

interface TreeNodeData {
    children: Record<string, TreeNodeData>;
    projects: string[];
    isLeaf: boolean;
}

// Helper function to get webhook status for a project
const getProjectWebhookStatus = (projectsWithStatus: ProjectWithStatus[] | undefined, projectName: string): { status: string; icon: React.ReactNode; className: string } => {
    if (!projectsWithStatus) {
        return { status: 'Not Ready', icon: <Icons.NotReady />, className: 'text-yellow-500' };
    }
    
    const project = projectsWithStatus.find(p => p.project_path === projectName);
    if (!project) {
        return { status: 'Not Ready', icon: <Icons.NotReady />, className: 'text-yellow-500' };
    }
    
    switch (project.webhook_status) {
        case 'automatic':
            return { status: 'Connected', icon: <Icons.Ready />, className: 'text-green-500' };
        case 'manual':
            return { status: 'Manual', icon: <Icons.Clock />, className: 'text-blue-500' };
        case 'unconnected':
        default:
            return { status: 'Not Connected', icon: <Icons.Error />, className: 'text-red-500' };
    }
};

interface TreeNodeProps {
    name: string;
    data: TreeNodeData;
    level: number;
    path: string;
    projectsWithStatus?: ProjectWithStatus[];
}

const TreeNode: React.FC<TreeNodeProps> = ({ name, data, level, path, projectsWithStatus }) => {
    const [isExpanded, setIsExpanded] = useState(level < 2); // Auto-expand first 2 levels
    const hasChildren = Object.keys(data.children).length > 0;
    const hasProjects = data.projects.length > 0;
    const currentPath = path ? `${path}/${name}` : name;
    
    // Calculate total project count recursively
    const getTotalProjectCount = (nodeData: TreeNodeData): number => {
        let count = nodeData.projects.length;
        Object.values(nodeData.children).forEach(child => {
            count += getTotalProjectCount(child);
        });
        return count;
    };
    
    const totalProjectCount = getTotalProjectCount(data);
    
    const toggleExpanded = () => {
        setIsExpanded(!isExpanded);
    };

    // Don't render root nodes that are just containers
    if (name === '_root') {
        return (
            <div>
                {data.projects.map((project) => {
                    const webhookStatus = getProjectWebhookStatus(projectsWithStatus, project);
                    return (
                        <div
                            key={project}
                            className="flex items-center justify-between py-2 px-3 bg-slate-700 rounded border border-slate-600 hover:border-slate-500 transition-colors mb-1"
                        >
                            <div className="flex items-center space-x-3">
                                <Icons.Git />
                                <span className="text-slate-200 font-mono text-sm">
                                    {project}
                                </span>
                            </div>
                            <div className={`flex items-center space-x-1 ${webhookStatus.className}`}>
                                {webhookStatus.icon}
                                <span className="text-xs">{webhookStatus.status}</span>
                            </div>
                        </div>
                    );
                })}
                {/* Render children */}
                {Object.entries(data.children)
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([childName, childData]) => (
                        <TreeNode
                            key={childName}
                            name={childName}
                            data={childData}
                            level={0}
                            path=""
                            projectsWithStatus={projectsWithStatus}
                        />
                    ))}
            </div>
        );
    }

    return (
        <div>
            {/* Namespace/Group node */}
            <div 
                className={`flex items-center space-x-2 py-1 px-2 rounded hover:bg-slate-600 transition-colors cursor-pointer`}
                style={{ paddingLeft: `${level * 16 + 8}px` }}
                onClick={toggleExpanded}
            >
                <span className="text-slate-400">
                    {isExpanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}
                </span>
                
                <span className="text-slate-300">
                    {isExpanded ? <Icons.FolderOpen /> : <Icons.Folder />}
                </span>
                
                <span className="text-slate-200 text-sm font-medium">
                    {name}
                </span>
                
                {/* Show total project count for namespaces */}
                {totalProjectCount > 0 && (
                    <Badge variant="default" size="sm">
                        {totalProjectCount}
                    </Badge>
                )}
            </div>
            
            {/* Render projects directly under this namespace */}
            {hasProjects && isExpanded && (
                <div style={{ marginLeft: `${(level + 1) * 16 + 8}px` }}>
                    {data.projects.map((project) => {
                        const webhookStatus = getProjectWebhookStatus(projectsWithStatus, project);
                        return (
                            <div 
                                key={project} 
                                className="flex items-center justify-between py-2 px-3 bg-slate-700 rounded border border-slate-600 hover:border-slate-500 transition-colors mb-1"
                            >
                                <div className="flex items-center space-x-3">
                                    <Icons.Git />
                                    <span className="text-slate-200 font-mono text-sm">
                                        {project}
                                    </span>
                                </div>
                                <div className={`flex items-center space-x-1 ${webhookStatus.className}`}>
                                    {webhookStatus.icon}
                                    <span className="text-xs">{webhookStatus.status}</span>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
            
            {/* Render child namespaces */}
            {hasChildren && isExpanded && (
                <div>
                    {Object.entries(data.children)
                        .sort(([a], [b]) => a.localeCompare(b))
                        .map(([childName, childData]) => (
                            <TreeNode
                                key={childName}
                                name={childName}
                                data={childData}
                                level={level + 1}
                                path={currentPath}
                                projectsWithStatus={projectsWithStatus}
                            />
                        ))}
                </div>
            )}
        </div>
    );
};

const ConnectorDetails: React.FC = () => {
    const { connectorId } = useParams<{ connectorId: string }>();
    const navigate = useNavigate();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    const [connector, setConnector] = useState<Connector | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [isDeleting, setIsDeleting] = useState(false);
    const [repositoryAccess, setRepositoryAccess] = useState<RepositoryAccessExtended | null>(null);
    const [isLoadingRepos, setIsLoadingRepos] = useState(false);
    const [isRefreshing, setIsRefreshing] = useState(false);
    const [isEnablingManualTrigger, setIsEnablingManualTrigger] = useState(false);
    
    // Repository filtering and grouping state
    const [searchTerm, setSearchTerm] = useState('');
    const [viewMode, setViewMode] = useState<'tree' | 'list'>('tree'); // Default to tree view

    // Process repositories for filtering and tree structure
    const processedRepositories = useMemo(() => {
        // Handle both old and new API response formats
        const projectsList = repositoryAccess?.projects_with_status 
            ? repositoryAccess.projects_with_status.map(p => p.project_path)
            : repositoryAccess?.projects || [];
            
        if (projectsList.length === 0) return { total: 0, filtered: 0, tree: {} };

        // Filter repositories based on search term
        const filtered = projectsList.filter(project =>
            project.toLowerCase().includes(searchTerm.toLowerCase())
        );        // Create tree structure for namespaces
        const buildTree = (projects: string[]) => {
            const result: Record<string, TreeNodeData> = {};
            
            projects.forEach(project => {
                const parts = project.split('/');
                let current = result;
                
                // Build the tree path, but don't create leaf nodes as folders
                for (let i = 0; i < parts.length - 1; i++) { // Stop before the last part (project name)
                    const part = parts[i];
                    if (!current[part]) {
                        current[part] = {
                            children: {},
                            projects: [],
                            isLeaf: false
                        };
                    }
                    current = current[part].children;
                }
                
                // Add the full project path to the appropriate namespace
                const namespacePath = parts.slice(0, -1);
                if (namespacePath.length > 0) {
                    const namespace = namespacePath[namespacePath.length - 1];
                    let targetNode = result;
                    
                    // Navigate to the correct namespace node
                    for (const part of namespacePath) {
                        if (!targetNode[part]) {
                            targetNode[part] = {
                                children: {},
                                projects: [],
                                isLeaf: false
                            };
                        }
                        targetNode = targetNode[part].children;
                    }
                    
                    // Go back to the namespace node to add the project
                    targetNode = result;
                    for (const part of namespacePath.slice(0, -1)) {
                        targetNode = targetNode[part].children;
                    }
                    targetNode[namespace].projects.push(project);
                } else {
                    // Handle projects without namespace (root level)
                    if (!result['_root']) {
                        result['_root'] = {
                            children: {},
                            projects: [],
                            isLeaf: false
                        };
                    }
                    result['_root'].projects.push(project);
                }
            });
            
            return result;
        };
        
        return {
            total: projectsList.length,
            filtered: filtered.length,
            tree: buildTree(filtered)
        };
    }, [repositoryAccess?.projects, repositoryAccess?.projects_with_status, searchTerm]);

    useEffect(() => {
        if (connectorId && connectors.length > 0) {
            const foundConnector = connectors.find(c => c.id === connectorId);
            if (foundConnector) {
                setConnector(foundConnector);
                setError(null);
                // Fetch repository access information
                fetchRepositoryAccess(connectorId);
            } else {
                setError('Connector not found');
            }
            setIsLoading(false);
        } else if (connectors.length === 0) {
            // Still loading connectors
            setIsLoading(true);
        }
    }, [connectorId, connectors]);

    const fetchRepositoryAccess = async (connectorId: string, refresh?: boolean) => {
        if (refresh) {
            setIsRefreshing(true);
        } else {
            setIsLoadingRepos(true);
        }
        try {
            const accessData = await getRepositoryAccess(connectorId, refresh);
            setRepositoryAccess(accessData as RepositoryAccessExtended);
        } catch (err) {
            console.error('Error fetching repository access:', err);
            // Don't show error for repository access, just log it
        } finally {
            setIsLoadingRepos(false);
            setIsRefreshing(false);
        }
    };

    const handleRefreshRepositories = async () => {
        if (!connectorId) return;
        
        // Show confirmation dialog for long operation
        if (!confirm('This operation may take a while as it will fetch fresh data from the Git provider. Continue?')) {
            return;
        }
        
        await fetchRepositoryAccess(connectorId, true);
    };

    const handleEnableManualTrigger = async () => {
        if (!connectorId) return;
        
        // Show confirmation dialog
        if (!confirm('This will enable manual trigger for all projects in this connector. You will be able to trigger AI reviews using "@liveapibot" mention or via the web UI. Continue?')) {
            return;
        }
        
        setIsEnablingManualTrigger(true);
        try {
            const result = await enableManualTriggerForAllProjects(connectorId);
            // Reset loading state immediately after successful API call
            setIsEnablingManualTrigger(false);
            
            alert(`Success! Manual trigger enabled for ${result.jobs_queued || result.total_projects || 'all'} projects. You can now trigger AI reviews using "@liveapibot" mention or via the web UI.`);
            
            // Refresh repository access to show updated status
            await fetchRepositoryAccess(connectorId, true);
        } catch (err) {
            console.error('Error enabling manual trigger:', err);
            setIsEnablingManualTrigger(false);
            alert('Failed to enable manual trigger. Please try again or contact support.');
        }
    };

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
                                    <span 
                                        className="text-slate-200 hover:text-slate-100 cursor-help transition-colors"
                                        title={connector.createdAt ? format(new Date(connector.createdAt), 'PPpp') : undefined}
                                    >
                                        {connector.createdAt ? 
                                            formatDistanceToNow(new Date(connector.createdAt), { addSuffix: true }) : 
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

                            {/* GitLab Profile Information */}
                            {(connector.type === 'gitlab' || connector.type === 'gitlab-com' || connector.type === 'gitlab-self-hosted') && 
                             connector.metadata?.gitlabProfile && (
                                <div>
                                    <label className="block text-sm font-medium text-slate-300 mb-3">
                                        GitLab Profile Information
                                    </label>
                                    <div className="bg-slate-800 rounded-lg p-4 border border-slate-600">
                                        <div className="flex items-center space-x-4">
                                            {connector.metadata.gitlabProfile.avatar_url && (
                                                <Avatar 
                                                    src={connector.metadata.gitlabProfile.avatar_url} 
                                                    size="lg"
                                                    initials={connector.metadata.gitlabProfile.name?.charAt(0).toUpperCase() || 'U'}
                                                />
                                            )}
                                            <div className="flex-grow">
                                                <div className="flex items-center space-x-3 mb-2">
                                                    <h4 className="text-lg font-semibold text-white">
                                                        {connector.metadata.gitlabProfile.name || 'Unknown User'}
                                                    </h4>
                                                    {connector.metadata.gitlabProfile.username && (
                                                        <span className="text-blue-300 font-medium">
                                                            @{connector.metadata.gitlabProfile.username}
                                                        </span>
                                                    )}
                                                </div>
                                                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
                                                    {connector.metadata.gitlabProfile.email && (
                                                        <div className="flex items-center space-x-2">
                                                            <Icons.Email />
                                                            <span className="text-slate-300">
                                                                {connector.metadata.gitlabProfile.email}
                                                            </span>
                                                        </div>
                                                    )}
                                                    {connector.metadata.gitlabProfile.id && (
                                                        <div className="flex items-center space-x-2">
                                                            <Icons.User />
                                                            <span className="text-slate-300">
                                                                User ID: {connector.metadata.gitlabProfile.id}
                                                            </span>
                                                        </div>
                                                    )}
                                                </div>
                                                {connector.metadata.manual && (
                                                    <div className="mt-3">
                                                        <Badge variant="info" size="sm">
                                                            Manual Connection
                                                        </Badge>
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            )}
                            
                            {/* Other Metadata (for non-GitLab providers or additional data) */}
                            {connector.metadata && Object.keys(connector.metadata).length > 0 && 
                             !(connector.type === 'gitlab' || connector.type === 'gitlab-com' || connector.type === 'gitlab-self-hosted') && (
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

                    {/* Repository Access */}
                    <Card className="mt-6">
                        {/* Custom Header */}
                        <div className="flex items-center justify-between mb-6 pb-4 border-b border-slate-700">
                            <div className="flex items-center space-x-3">
                                <h3 className="text-lg font-semibold text-slate-100">Repository Access</h3>
                                {repositoryAccess?.project_count && (
                                    <Badge variant="info" size="sm">
                                        {repositoryAccess.project_count} projects
                                    </Badge>
                                )}
                                <div className="flex items-center space-x-1 text-yellow-400">
                                    <Icons.NotReady />
                                    <span className="text-xs font-medium">Not Ready</span>
                                </div>
                            </div>
                            <div className="flex items-center space-x-4">
                                <div className="flex items-center space-x-2 text-slate-400 text-xs">
                                    <Icons.Clock />
                                    <span>
                                        {repositoryAccess?.updated_at 
                                            ? `Synced ${formatDistanceToNow(new Date(repositoryAccess.updated_at), { addSuffix: true })}`
                                            : 'Never synced'
                                        }
                                    </span>
                                </div>
                                <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={handleRefreshRepositories}
                                    disabled={isRefreshing}
                                    icon={isRefreshing ? <Spinner size="sm" /> : <Icons.Refresh />}
                                    iconPosition="left"
                                    className="text-xs"
                                >
                                    {isRefreshing ? 'Refreshing...' : 'Refresh'}
                                </Button>
                            </div>
                        </div>
                        {isLoadingRepos ? (
                            <div className="flex items-center justify-center py-8">
                                <Spinner size="md" color="text-blue-400" />
                                <span className="ml-3 text-slate-300">Loading repository access...</span>
                            </div>
                        ) : repositoryAccess?.error ? (
                            <div className="text-center py-8">
                                <Icons.Warning />
                                <p className="text-yellow-400 mt-4 font-medium">
                                    Repository Access Issue
                                </p>
                                <p className="text-slate-400 mt-2 text-sm">
                                    {repositoryAccess.error}
                                </p>
                            </div>
                        ) : repositoryAccess && repositoryAccess.projects.length > 0 ? (
                            <div className="space-y-6">
                                {/* Repository Summary */}
                                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                                    <div>
                                        <label className="block text-sm font-medium text-slate-300 mb-1">
                                            Provider
                                        </label>
                                        <span className="text-slate-200 capitalize">
                                            {repositoryAccess.provider}
                                        </span>
                                    </div>
                                    <div>
                                        <label className="block text-sm font-medium text-slate-300 mb-1">
                                            Total Projects
                                        </label>
                                        <span className="text-slate-200 font-semibold">
                                            {processedRepositories.total}
                                        </span>
                                    </div>
                                    <div>
                                        <label className="block text-sm font-medium text-slate-300 mb-1">
                                            Webhook Status
                                        </label>
                                        <div className="flex items-center space-x-2">
                                            <Icons.NotReady />
                                            <span className="text-yellow-400 text-sm font-medium">Setup Required</span>
                                        </div>
                                    </div>
                                </div>

                                {/* Trigger Configuration */}
                                <div className="bg-slate-800 rounded-lg p-4 border border-slate-600">
                                    <div className="flex items-center justify-between mb-4">
                                        <div>
                                            <h4 className="text-lg font-medium text-slate-200 mb-2">
                                                Review Trigger Configuration
                                            </h4>
                                            <p className="text-sm text-slate-400">
                                                Configure how AI reviews are triggered for projects in this connector.
                                            </p>
                                        </div>
                                    </div>
                                    
                                    <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-4">
                                        <div className="bg-slate-700 rounded-lg p-3 border border-slate-600">
                                            <div className="flex items-center space-x-2 mb-2">
                                                <Icons.NotReady />
                                                <span className="text-sm font-medium text-slate-300">Not Connected</span>
                                            </div>
                                            <p className="text-xs text-slate-400">
                                                Cannot trigger AI review on this project
                                            </p>
                                        </div>
                                        
                                        <div className="bg-slate-700 rounded-lg p-3 border border-slate-600">
                                            <div className="flex items-center space-x-2 mb-2">
                                                <Icons.Warning />
                                                <span className="text-sm font-medium text-yellow-400">Manual Trigger</span>
                                            </div>
                                            <p className="text-xs text-slate-400">
                                                Can manually trigger using "@liveapibot" mention or web UI
                                            </p>
                                        </div>
                                        
                                        <div className="bg-slate-700 rounded-lg p-3 border border-slate-600">
                                            <div className="flex items-center space-x-2 mb-2">
                                                <Icons.Success />
                                                <span className="text-sm font-medium text-green-400">Automatic Trigger</span>
                                            </div>
                                            <p className="text-xs text-slate-400">
                                                New MR versions automatically get reviewed
                                            </p>
                                        </div>
                                    </div>
                                    
                                    <div className="flex items-center justify-between pt-3 border-t border-slate-600">
                                        <div>
                                            <p className="text-sm text-slate-300 font-medium">
                                                Current Status: <span className="text-yellow-400">Not Connected</span>
                                            </p>
                                            <p className="text-xs text-slate-400 mt-1">
                                                Enable manual trigger to start using AI reviews with this connector
                                            </p>
                                        </div>
                                        <Button
                                            variant="primary"
                                            size="sm"
                                            onClick={handleEnableManualTrigger}
                                            disabled={isEnablingManualTrigger}
                                            icon={isEnablingManualTrigger ? <Spinner size="sm" /> : <Icons.Settings />}
                                            iconPosition="left"
                                        >
                                            {isEnablingManualTrigger ? 'Enabling...' : 'Enable Manual Trigger for All Projects'}
                                        </Button>
                                    </div>
                                </div>

                                {/* Search and Filter Controls */}
                                <div className="border-t border-slate-700 pt-4">
                                    <div className="mb-4">
                                        <Input
                                            placeholder="Search repositories..."
                                            value={searchTerm}
                                            onChange={(e) => setSearchTerm(e.target.value)}
                                            icon={<Icons.Search />}
                                            iconPosition="left"
                                        />
                                    </div>
                                    
                                    {/* Results Summary */}
                                    <div className="flex items-center justify-between text-sm text-slate-400 mb-3">
                                        <span>
                                            Showing {processedRepositories.filtered} of {processedRepositories.total} repositories
                                        </span>
                                        {searchTerm && (
                                            <span className="flex items-center space-x-1">
                                                <Icons.Filter />
                                                <span>Filtered results</span>
                                            </span>
                                        )}
                                    </div>
                                    
                                    {/* View Toggle */}
                                    <div className="flex items-center justify-between mb-3">
                                        <div className="flex items-center text-sm text-slate-400">
                                            {viewMode === 'tree' && (
                                                <>
                                                    <Icons.Grid />
                                                    <span className="ml-2">Tree view with namespace grouping</span>
                                                </>
                                            )}
                                            {viewMode === 'list' && (
                                                <>
                                                    <Icons.List />
                                                    <span className="ml-2">Flat list view</span>
                                                </>
                                            )}
                                        </div>
                                        <div className="flex items-center space-x-1 bg-slate-700 rounded-lg p-1">
                                            <button
                                                onClick={() => setViewMode('tree')}
                                                className={`p-2 rounded transition-colors ${
                                                    viewMode === 'tree' 
                                                        ? 'bg-blue-600 text-white' 
                                                        : 'text-slate-400 hover:text-slate-200'
                                                }`}
                                                title="Tree view"
                                            >
                                                <Icons.Grid />
                                            </button>
                                            <button
                                                onClick={() => setViewMode('list')}
                                                className={`p-2 rounded transition-colors ${
                                                    viewMode === 'list' 
                                                        ? 'bg-blue-600 text-white' 
                                                        : 'text-slate-400 hover:text-slate-200'
                                                }`}
                                                title="List view"
                                            >
                                                <Icons.List />
                                            </button>
                                        </div>
                                    </div>
                                </div>

                                {/* Repository List */}
                                <div className="bg-slate-800 rounded-lg p-4">
                                    <div className="space-y-1">
                                        {viewMode === 'tree' ? (
                                            // Tree View
                                            Object.entries(processedRepositories.tree)
                                                .sort(([a], [b]) => a.localeCompare(b))
                                                .map(([rootName, rootData]) => (
                                                    <TreeNode
                                                        key={rootName}
                                                        name={rootName}
                                                        data={rootData}
                                                        level={0}
                                                        path=""
                                                        projectsWithStatus={repositoryAccess?.projects_with_status}
                                                    />
                                                ))
                                        ) : (
                                            // List View
                                            (repositoryAccess?.projects_with_status 
                                                ? repositoryAccess.projects_with_status.map(p => p.project_path)
                                                : repositoryAccess?.projects || [])
                                                .filter(project =>
                                                    project.toLowerCase().includes(searchTerm.toLowerCase())
                                                )
                                                .sort()
                                                .map((project) => {
                                                    const webhookStatus = getProjectWebhookStatus(repositoryAccess?.projects_with_status, project);
                                                    return (
                                                        <div 
                                                            key={project} 
                                                            className="flex items-center justify-between py-2 px-3 bg-slate-700 rounded border border-slate-600 hover:border-slate-500 transition-colors mb-1"
                                                        >
                                                            <div className="flex items-center space-x-3">
                                                                <Icons.Git />
                                                                <span className="text-slate-200 font-mono text-sm">
                                                                    {project}
                                                                </span>
                                                            </div>
                                                            <div className={`flex items-center space-x-1 ${webhookStatus.className}`}>
                                                                {webhookStatus.icon}
                                                                <span className="text-xs">{webhookStatus.status}</span>
                                                            </div>
                                                        </div>
                                                    );
                                                })
                                        )}
                                    </div>
                                    
                                    {processedRepositories.filtered === 0 && searchTerm && (
                                        <div className="text-center py-8">
                                            <Icons.Search />
                                            <p className="text-slate-400 mt-4">
                                                No repositories found matching "{searchTerm}"
                                            </p>
                                            <Button 
                                                variant="ghost" 
                                                size="sm" 
                                                onClick={() => setSearchTerm('')}
                                                className="mt-2"
                                            >
                                                Clear search
                                            </Button>
                                        </div>
                                    )}
                                </div>
                            </div>
                        ) : (
                            <div className="text-center py-8">
                                <Icons.EmptyState />
                                <p className="text-slate-400 mt-4">
                                    No repository access information available.
                                </p>
                                <p className="text-slate-500 text-sm mt-2">
                                    This may be due to missing credentials or unsupported provider.
                                </p>
                            </div>
                        )}
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
