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
    Input,
    Popover
} from '../../components/UIPrimitives';
import { Connector } from '../../store/Connector/reducer';
import { deleteConnector, getRepositoryAccess, enableManualTriggerForAllProjects, disableManualTriggerForAllProjects, ProjectWithStatus, getConnector } from '../../api/connectors';
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
// NOTE: Backend currently labels projects that have manual trigger webhooks as 'automatic'.
// Until true automatic review mode (review every new MR version) ships, we remap 'automatic' -> 'manual'
// for UI display & counting to avoid confusing users (so Manual shows the real count, Automatic stays 0).
const getProjectWebhookStatus = (
    projectsWithStatus: ProjectWithStatus[] | undefined,
    projectName: string
): { status: string; icon: React.ReactNode; className: string } => {
    if (!projectsWithStatus) {
        return { status: 'Not Ready', icon: <Icons.NotReady />, className: 'text-yellow-500' };
    }

    const project = projectsWithStatus.find(p => p.project_path === projectName);
    if (!project) {
        return { status: 'Not Ready', icon: <Icons.NotReady />, className: 'text-yellow-500' };
    }

    switch (project.webhook_status) {
        case 'automatic': // temporary remap to Manual
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
    const [isDisablingManualTrigger, setIsDisablingManualTrigger] = useState(false);
    
    // Simple notification state
    const [notification, setNotification] = useState<{
        type: 'success' | 'error' | 'warning' | 'info';
        message: string;
        show: boolean;
        persistent?: boolean; // Don't auto-hide if true
    } | null>(null);
    
    // Confirmation modal state
    const [confirmModal, setConfirmModal] = useState<{
        show: boolean;
        title: string;
        message: string;
        onConfirm: () => void;
        confirmText: string;
        cancelText: string;
        type: 'warning' | 'danger';
    } | null>(null);
    
    // Repository filtering and grouping state
    const [searchTerm, setSearchTerm] = useState('');
    const [viewMode, setViewMode] = useState<'tree' | 'list'>('tree'); // Default to tree view

    // Compute overall connector status based on projects
    const connectorStatus = useMemo(() => {
        if (!repositoryAccess?.projects_with_status || repositoryAccess.projects_with_status.length === 0) {
            return 'unconnected';
        }

        const statuses = repositoryAccess.projects_with_status.map(p => p.webhook_status);
        const hasManual = statuses.some(s => s === 'manual');
        const hasAutomatic = statuses.some(s => s === 'automatic');

        // Normalization shim: treat 'automatic' (legacy mislabel) as manual if no explicit manual present
        if (!hasManual && hasAutomatic) return 'manual';
        if (hasManual) return 'manual';
        if (hasAutomatic) return 'automatic'; // future real automatic mode
        return 'unconnected';
    }, [repositoryAccess?.projects_with_status]);

    // Compute status counts for projects
    const statusCounts = useMemo(() => {
        if (!repositoryAccess?.projects_with_status || repositoryAccess.projects_with_status.length === 0) {
            return { unconnected: 0, manual: 0, automatic: 0, total: 0 };
        }

        const statuses = repositoryAccess.projects_with_status.map(p => p.webhook_status);
        const raw = {
            unconnected: statuses.filter(s => s === 'unconnected').length,
            manual: statuses.filter(s => s === 'manual').length,
            automatic: statuses.filter(s => s === 'automatic').length,
            total: statuses.length
        };

        // Normalization shim: if backend only returns 'automatic' (legacy) but no 'manual', treat those as manual.
        if (raw.manual === 0 && raw.automatic > 0) {
            return {
                unconnected: raw.unconnected,
                manual: raw.automatic,
                automatic: 0,
                total: raw.total
            };
        }

        return raw;
    }, [repositoryAccess?.projects_with_status]);

    // Helper function to parse simple markdown bold (**text**)
    const parseMarkdownBold = (text: string) => {
        const parts = text.split(/(\*\*[^*]+\*\*)/);
        return parts.map((part, index) => {
            if (part.startsWith('**') && part.endsWith('**')) {
                return <strong key={index}>{part.slice(2, -2)}</strong>;
            }
            return part;
        });
    };

    // Helper function for simple notifications
    const showNotification = (type: 'success' | 'error' | 'warning' | 'info', message: string, persistent = false) => {
        setNotification({ type, message, show: true, persistent });
        // Auto-hide after 5 seconds unless persistent
        if (!persistent) {
            setTimeout(() => {
                setNotification(prev => prev ? { ...prev, show: false } : null);
            }, 5000);
        }
    };

    // Helper function to show confirmation modal
    const showConfirmModal = (
        title: string,
        message: string,
        onConfirm: () => void,
        options?: {
            confirmText?: string;
            cancelText?: string;
            type?: 'warning' | 'danger';
        }
    ) => {
        setConfirmModal({
            show: true,
            title,
            message,
            onConfirm: () => {
                setConfirmModal(null);
                onConfirm();
            },
            confirmText: options?.confirmText || 'Continue',
            cancelText: options?.cancelText || 'Cancel',
            type: options?.type || 'warning'
        });
    };

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
        const initializeConnector = async () => {
            if (!connectorId) {
                setError('No connector ID provided');
                setIsLoading(false);
                return;
            }

            try {
                // First try to find connector in Redux state
                let foundConnector = connectors.find(c => c.id === connectorId);
                
                // If not found in Redux state, fetch it directly from API
                if (!foundConnector && connectors.length === 0) {
                    // Connectors not loaded yet, try to fetch this specific connector
                    try {
                        const connectorResponse = await getConnector(connectorId);
                        // Transform API response to Connector format
                        foundConnector = {
                            id: connectorResponse.id.toString(),
                            name: connectorResponse.connection_name || `${connectorResponse.provider} Connection`,
                            type: connectorResponse.provider as any,
                            url: connectorResponse.provider_url || '',
                            apiKey: connectorResponse.provider_app_id || '',
                            createdAt: connectorResponse.created_at,
                            metadata: connectorResponse.metadata || {}
                        };
                    } catch (apiError) {
                        console.error('Error fetching specific connector:', apiError);
                        setError('Connector not found');
                        setIsLoading(false);
                        return;
                    }
                } else if (!foundConnector && connectors.length > 0) {
                    // Connectors are loaded but this one doesn't exist
                    setError('Connector not found');
                    setIsLoading(false);
                    return;
                }

                if (foundConnector) {
                    setConnector(foundConnector);
                    setError(null);
                    // Fetch repository access information
                    fetchRepositoryAccess(connectorId);
                }
            } catch (err) {
                console.error('Error initializing connector:', err);
                setError('Failed to load connector');
            } finally {
                setIsLoading(false);
            }
        };

        initializeConnector();
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
        
        showConfirmModal(
            "Refresh Repository Data",
            "This operation may take a while as it will fetch fresh data from the Git provider. All cached repository information will be updated. Continue?",
            async () => {
                try {
                    await fetchRepositoryAccess(connectorId, true);
                    showNotification('success', 'Successfully updated repository information from your Git provider.');
                } catch (err) {
                    console.error('Error refreshing repositories:', err);
                    showNotification('error', 'Failed to refresh repository data. Please try again or contact support.');
                }
            },
            {
                confirmText: 'Yes, Refresh',
                cancelText: 'Cancel',
                type: 'warning'
            }
        );
    };

    const handleDisableManualTrigger = async () => {
        if (!connectorId) return;
        
        const modalMessage = "This will remove webhook access for all projects in this connector, disabling the ability to trigger AI reviews using \"@liveapibot\" mentions or via the web UI. **This process may take several minutes to complete.** Continue?";
        
        showConfirmModal(
            "Disable Manual Trigger for All Projects",
            modalMessage,
            async () => {
                setIsDisablingManualTrigger(true);
                try {
                    const result = await disableManualTriggerForAllProjects(connectorId);
                    // Reset loading state immediately after successful API call
                    setIsDisablingManualTrigger(false);
                    
                    showNotification(
                        'success', 
                        `✅ Webhook removal has been queued for ${result.jobs_queued || result.total_projects || 'all'} projects. **It may take several minutes for all webhook statuses to update.** Manual trigger functionality has been disabled for this connector.`,
                        true // Make it persistent
                    );
                    
                    // Refresh repository access to show updated status after a short delay
                    setTimeout(() => {
                        fetchRepositoryAccess(connectorId, true);
                    }, 2000);
                } catch (err) {
                    console.error('Error disabling manual trigger:', err);
                    setIsDisablingManualTrigger(false);
                    showNotification('error', 'An error occurred while disabling manual trigger. Please try again or contact support if the problem persists.');
                }
            },
            {
                confirmText: 'Yes, Disable',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );
    };

    const handleEnableManualTrigger = async () => {
        if (!connectorId) return;
        
        const modalMessage = "This will configure webhook access for all projects in this connector, allowing you to trigger AI reviews using \"@liveapibot\" mentions or via the web UI. **This process may take several minutes to complete.** Continue?";
        
        showConfirmModal(
            "Enable Manual Trigger for All Projects",
            modalMessage,
            async () => {
                setIsEnablingManualTrigger(true);
                try {
                    const result = await enableManualTriggerForAllProjects(connectorId);
                    // Reset loading state immediately after successful API call
                    setIsEnablingManualTrigger(false);
                    
                    showNotification(
                        'success', 
                        `✅ Webhook configuration has been queued for ${result.jobs_queued || result.total_projects || 'all'} projects. **It may take several minutes for all webhook statuses to update.** You can now trigger AI reviews using "@liveapibot" mentions or via the web UI.`,
                        true // Make it persistent
                    );
                    
                    // Refresh repository access to show updated status after a short delay
                    setTimeout(() => {
                        fetchRepositoryAccess(connectorId, true);
                    }, 2000);
                } catch (err) {
                    console.error('Error enabling manual trigger:', err);
                    setIsEnablingManualTrigger(false);
                    showNotification('error', 'An error occurred while enabling manual trigger. Please try again or contact support if the problem persists.');
                }
            },
            {
                confirmText: 'Yes, Enable',
                cancelText: 'Cancel',
                type: 'warning'
            }
        );
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

                                {/* Provider Documentation (Popover Trigger) */}
                                <div className="md:col-span-2 flex items-center space-x-3 bg-slate-800/40 border border-slate-700 rounded-md px-3 py-2">
                                    <div className="flex items-center text-blue-300">
                                        <Icons.Info />
                                    </div>
                                    <div className="text-xs sm:text-sm text-slate-300 flex-1">
                                        Need help with tokens / OAuth setup?
                                    </div>
                                    <Popover
                                        hover
                                        trigger={
                                            <Button variant="primary" size="sm" className="!text-xs">
                                                Guide
                                            </Button>
                                        }
                                    >
                                        <div className="space-y-2">
                                            <p className="text-slate-200 font-medium text-sm mb-1">{formatConnectorType(connector.type)} Setup</p>
                                            <p className="text-xs text-slate-400 leading-relaxed">
                                                Follow the official guide to create the required token / application with correct scopes.
                                            </p>
                                            <ul className="text-xs text-slate-300 list-disc pl-4 space-y-1">
                                                {connector.type.startsWith('gitlab') && (
                                                    <>
                                                        <li>Scope: <code className="text-green-400">api</code></li>
                                                        <li>Dedicated bot user recommended</li>
                                                        <li>Grant access to all target groups/projects</li>
                                                    </>
                                                )}
                                                {connector.type === 'github' && (
                                                    <>
                                                        <li>Classic PAT with: <code className="text-green-400">repo</code>, <code className="text-green-400">read:org</code></li>
                                                        <li>Use a dedicated service user</li>
                                                    </>
                                                )}
                                                {connector.type === 'bitbucket' && (
                                                    <>
                                                        <li>Generate API Token (replaces App Password)</li>
                                                        <li>Grant repo read + pull request read</li>
                                                    </>
                                                )}
                                            </ul>
                                            <div className="pt-1">
                                                <a
                                                    href={(() => {
                                                        switch (connector.type) {
                                                            case 'gitlab-self-hosted':
                                                                return 'https://github.com/HexmosTech/LiveReview/wiki/Self%E2%80%90Hosted-GitLab';
                                                            case 'gitlab-com':
                                                            case 'gitlab':
                                                                return 'https://github.com/HexmosTech/LiveReview/wiki/Gitlab';
                                                            case 'github':
                                                                return 'https://github.com/HexmosTech/LiveReview/wiki/Github';
                                                            case 'bitbucket':
                                                                return 'https://github.com/HexmosTech/LiveReview/wiki/BitBucket';
                                                            default:
                                                                return '#';
                                                        }
                                                    })()}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="text-blue-400 hover:text-blue-300 underline text-xs font-medium"
                                                >
                                                    Open full guide ↗
                                                </a>
                                            </div>
                                        </div>
                                    </Popover>
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
                                <div className="ml-3 text-slate-300">
                                    <div className="font-medium">Syncing repository data...</div>
                                    <div className="text-sm text-slate-400 mt-1">
                                        This may take a while as we fetch all visible projects from your Git provider
                                    </div>
                                </div>
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
                                        <div className="flex-1">
                                            <p className="text-sm text-slate-300 font-medium mb-3">
                                                Project Webhook Status
                                            </p>
                                            <div className="grid grid-cols-3 gap-4">
                                                <div className="text-center">
                                                    <div className="flex items-center justify-center space-x-1 mb-1">
                                                        <Icons.Error />
                                                        <span className="text-lg font-semibold text-red-400">
                                                            {statusCounts.unconnected}
                                                        </span>
                                                    </div>
                                                    <p className="text-xs text-slate-400">Not Connected</p>
                                                </div>
                                                <div className="text-center">
                                                    <div className="flex items-center justify-center space-x-1 mb-1">
                                                        <Icons.Clock />
                                                        <span className="text-lg font-semibold text-blue-400">
                                                            {statusCounts.manual}
                                                        </span>
                                                    </div>
                                                    <p className="text-xs text-slate-400">Manual Trigger</p>
                                                </div>
                                                <div className="text-center">
                                                    <div className="flex items-center justify-center space-x-1 mb-1">
                                                        <Icons.Success />
                                                        <span className="text-lg font-semibold text-green-400">
                                                            {statusCounts.automatic}
                                                        </span>
                                                    </div>
                                                    <p className="text-xs text-slate-400">Automatic</p>
                                                    <p className="text-xs text-slate-500">(Coming Soon)</p>
                                                </div>
                                            </div>
                                        </div>
                                        <div className="flex items-center space-x-2 ml-6">
                                            <Button
                                                variant={connectorStatus === 'unconnected' ? 'primary' : 'outline'}
                                                size="sm"
                                                onClick={handleEnableManualTrigger}
                                                disabled={isEnablingManualTrigger || connectorStatus === 'manual'}
                                                icon={isEnablingManualTrigger ? <Spinner size="sm" /> : <Icons.Settings />}
                                                iconPosition="left"
                                                className={connectorStatus === 'unconnected' ? '' : 'text-xs'}
                                            >
                                                {isEnablingManualTrigger ? 'Enabling...' : 
                                                 connectorStatus === 'unconnected' ? 'Enable Manual Trigger for All Projects' : 'Re-enable Manual Trigger'}
                                            </Button>
                                            
                                            <Button
                                                variant="outline" 
                                                size="sm"
                                                onClick={handleDisableManualTrigger}
                                                disabled={isDisablingManualTrigger || (statusCounts.manual + statusCounts.automatic) === 0}
                                                icon={isDisablingManualTrigger ? <Spinner size="sm" /> : <Icons.Warning />}
                                                iconPosition="left"
                                                className="text-xs !text-red-400 hover:!text-red-300 hover:!border-red-400"
                                            >
                                                {isDisablingManualTrigger ? 'Disabling...' : 'Disable Manual Trigger for All Projects'}
                                            </Button>
                                        </div>
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
            
            {/* Confirmation Modal */}
            {confirmModal && confirmModal.show && (
                <div className="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4">
                    <div className="bg-white rounded-lg shadow-xl max-w-md w-full">
                        <div className="p-6">
                            {/* Modal Header */}
                            <div className="flex items-center mb-4">
                                <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center mr-3 ${
                                    confirmModal.type === 'danger' ? 'bg-red-100 text-red-600' : 'bg-yellow-100 text-yellow-600'
                                }`}>
                                    {confirmModal.type === 'danger' ? <Icons.Error /> : <Icons.Warning />}
                                </div>
                                <h3 className="text-lg font-medium text-gray-900">{confirmModal.title}</h3>
                            </div>
                            
                            {/* Modal Message */}
                            <div className="mb-6">
                                <p className="text-sm text-gray-700">
                                    {parseMarkdownBold(confirmModal.message)}
                                </p>
                            </div>
                            
                            {/* Modal Actions */}
                            <div className="flex justify-end space-x-3">
                                <Button
                                    variant="outline"
                                    onClick={() => setConfirmModal(null)}
                                >
                                    {confirmModal.cancelText}
                                </Button>
                                <Button
                                    variant={confirmModal.type === 'danger' ? 'danger' : 'primary'}
                                    onClick={confirmModal.onConfirm}
                                >
                                    {confirmModal.confirmText}
                                </Button>
                            </div>
                        </div>
                    </div>
                </div>
            )}
            
            {/* Notification Display */}
            {notification && notification.show && (
                <div className={`fixed z-50 transition-all duration-300 ${
                    notification.persistent 
                        ? 'inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4' // Full overlay for persistent
                        : 'top-4 right-4 max-w-md' // Top-right for regular notifications
                }`}>
                    <div className={notification.persistent ? 'bg-white rounded-lg shadow-xl max-w-lg w-full p-6' : ''}>
                        {notification.persistent ? (
                            // Persistent notification as modal
                            <div>
                                <div className="flex items-center mb-4">
                                    <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center mr-3 ${
                                        notification.type === 'success' ? 'bg-green-100 text-green-600' :
                                        notification.type === 'error' ? 'bg-red-100 text-red-600' :
                                        notification.type === 'warning' ? 'bg-yellow-100 text-yellow-600' : 'bg-blue-100 text-blue-600'
                                    }`}>
                                        {notification.type === 'success' ? <Icons.Success /> : 
                                         notification.type === 'error' ? <Icons.Error /> :
                                         notification.type === 'warning' ? <Icons.Warning /> : <Icons.Info />}
                                    </div>
                                    <h3 className="text-lg font-medium text-gray-900">
                                        {notification.type === 'success' ? 'Success!' :
                                         notification.type === 'error' ? 'Error' :
                                         notification.type === 'warning' ? 'Warning' : 'Information'}
                                    </h3>
                                </div>
                                <div className="mb-6">
                                    <p className="text-sm text-gray-700">
                                        {parseMarkdownBold(notification.message)}
                                    </p>
                                </div>
                                <div className="flex justify-end">
                                    <Button
                                        variant="primary"
                                        onClick={() => setNotification(prev => prev ? { ...prev, show: false } : null)}
                                    >
                                        Got it
                                    </Button>
                                </div>
                            </div>
                        ) : (
                            // Regular notification as Alert
                            <Alert
                                variant={notification.type}
                                onClose={() => setNotification(prev => prev ? { ...prev, show: false } : null)}
                            >
                                {notification.message}
                            </Alert>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
};

export default ConnectorDetails;
