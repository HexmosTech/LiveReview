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
}

const ConnectorCard: React.FC<ConnectorCardProps> = ({ 
    connector, 
    onEdit,
    isFirst,
    isLast
}) => {
    return (
        <li className="border border-slate-600 rounded-lg bg-slate-700 hover:bg-slate-600 transition-colors">
            <div className="p-4">
                <div className="flex justify-between items-center">
                    <div className="flex items-center">
                        <div className="flex-shrink-0 mr-3 relative">
                            <Avatar 
                                size="md"
                                initials={(connector.name && connector.name.length > 0) ? 
                                    connector.name.charAt(0).toUpperCase() : 
                                    (connector.providerName ? connector.providerName.charAt(0).toUpperCase() : 'A')}
                            />
                            <span className="absolute -top-1 -right-1 bg-blue-500 text-white text-xs rounded-full w-5 h-5 flex items-center justify-center">
                                {connector.displayOrder + 1}
                            </span>
                        </div>
                        <div>
                            <div className="flex items-center">
                                <h3 className="font-medium text-white">{connector.name || `${connector.providerName || 'AI'} Connector`}</h3>
                                <Badge 
                                    variant="primary" 
                                    size="sm"
                                    className="ml-2"
                                >
                                    {connector.providerName || 'Unknown'}
                                </Badge>
                            </div>
                            <p className="text-sm text-slate-300">
                                API Key: {connector.apiKey && connector.apiKey.length > 4 
                                    ? '••••••••' + connector.apiKey.slice(-4) 
                                    : (connector.apiKey ? connector.apiKey : 'Not set')}
                            </p>
                        </div>
                    </div>
                    <div className="flex items-center space-x-2">
                        {connector.createdAt && (
                            <span 
                                className="text-xs text-slate-300 hover:text-slate-200 cursor-help transition-colors"
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
                        <div className="flex space-x-1">
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
