import React from 'react';
import { formatDistanceToNow, format } from 'date-fns';
import { AIConnector } from '../types';
import { 
    Button, 
    Badge,
    Avatar
} from '../../../components/UIPrimitives';

interface ConnectorCardProps {
    connector: AIConnector;
    onEdit: () => void;
    isFirst: boolean;
    isLast: boolean;
    isReorderMode?: boolean;
}

const ConnectorCard: React.FC<ConnectorCardProps> = ({ 
    connector, 
    onEdit,
    isFirst,
    isLast,
    isReorderMode = false
}) => {
    // Lightweight mapping; keep in sync with popularAIProviders order/flags
    const supportMap: Record<string, 'recommended' | 'experimental' | undefined> = {
        gemini: 'recommended',
        ollama: 'recommended',
        openrouter: 'recommended',
        openai: 'experimental',
        claude: 'experimental',
        cohere: 'experimental'
    };

    return (
        <li 
            className="border border-slate-600 rounded-lg bg-slate-700 hover:bg-slate-600 transition-colors cursor-pointer"
            onClick={onEdit}
        >
            <div className="p-3 sm:p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:justify-between sm:items-center">
                    <div className="flex items-start sm:items-center flex-grow min-w-0">
                        <div className="flex-shrink-0 mr-3 relative">
                            <Avatar 
                                size="md"
                                initials={(connector.name && connector.name.length > 0) ? 
                                    connector.name.charAt(0).toUpperCase() : 
                                    (connector.providerName ? connector.providerName.charAt(0).toUpperCase() : 'A')}
                            />
                            {!isReorderMode && (
                                <span className="absolute -top-1 -right-1 bg-blue-500 text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
                                    {connector.displayOrder}
                                </span>
                            )}
                        </div>
                        <div className="flex-1 min-w-0">
                            <div className="flex items-center flex-wrap gap-2">
                                <h3 className="font-medium text-white truncate">{connector.name || `${connector.providerName || 'AI'} Connector`}</h3>
                                <Badge variant="primary" size="sm">
                                    {connector.providerName || 'Unknown'}
                                </Badge>
                                {supportMap[connector.providerName] === 'recommended' && (
                                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-yellow-100 text-yellow-800 font-medium border border-yellow-200 tracking-wide">Recommended</span>
                                )}
                                {supportMap[connector.providerName] === 'experimental' && (
                                    <Badge variant="warning" size="sm">Experimental</Badge>
                                )}
                            </div>
                            <p className="text-sm text-slate-300 truncate">
                                API Key: {connector.apiKey && connector.apiKey.length > 4 
                                    ? '••••••••' + connector.apiKey.slice(-4) 
                                    : (connector.apiKey ? connector.apiKey : 'Not set')}
                            </p>
                            {connector.selectedModel && (
                                <p className="text-sm text-slate-300 truncate">
                                    Model: {connector.selectedModel}
                                </p>
                            )}
                        </div>
                    </div>
                    <div className="flex items-center gap-2 justify-between sm:justify-end w-full sm:w-auto" onClick={(e) => e.stopPropagation()}>
                        {connector.createdAt && (
                            <span 
                                className="text-xs text-slate-300 hover:text-slate-200 cursor-help transition-colors truncate"
                                title={connector.createdAt instanceof Date ? 
                                    format(connector.createdAt, 'PPpp') : 
                                    'Date information unavailable'
                                }
                            >
                                {connector.createdAt instanceof Date ? 
                                    formatDistanceToNow(connector.createdAt, { addSuffix: true }) : 
                                    'Recently added'
                                }
                            </span>
                        )}
                        <div className="flex gap-1">
                            <Button
                                variant="secondary"
                                size="sm"
                                onClick={onEdit}
                            >
                                Edit
                            </Button>
                        </div>
                    </div>
                </div>
            </div>
        </li>
    );
};

export default ConnectorCard;
