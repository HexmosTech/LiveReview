import React from 'react';
import { AIProvider } from '../types';
import { 
    Badge,
    Icons 
} from '../../../components/UIPrimitives';

interface ProvidersListProps {
    providers: AIProvider[];
    selectedProvider: string;
    connectorCounts: { [key: string]: number };
    onSelectProvider: (providerId: string) => void;
    totalConnectors: number;
}

const ProvidersList: React.FC<ProvidersListProps> = ({
    providers,
    selectedProvider,
    connectorCounts,
    onSelectProvider,
    totalConnectors
}) => {
    return (
        <ul className="space-y-2">
            {/* All Connectors option */}
            <li 
                key="all-connectors"
                className={`p-3 rounded-lg cursor-pointer transition-all ${
                    selectedProvider === 'all' 
                        ? 'bg-slate-700 border-l-4 border-blue-500' 
                        : 'hover:bg-slate-700'
                }`}
                onClick={() => onSelectProvider('all')}
            >
                <div className="flex items-center">
                    <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center mr-3">
                        <Icons.Dashboard />
                    </div>
                    <div>
                        <h3 className="font-medium text-white">All Connectors</h3>
                        <div className="flex items-center mt-1">
                            <Badge 
                                variant="primary" 
                                size="sm"
                            >
                                {totalConnectors} 
                                {' '}total
                            </Badge>
                        </div>
                    </div>
                </div>
            </li>
            
            {/* Individual provider options */}
            {providers.map((provider) => (
                <li 
                    key={provider.id}
                    className={`p-3 rounded-lg cursor-pointer transition-all ${
                        selectedProvider === provider.id 
                            ? 'bg-slate-700 border-l-4 border-blue-500' 
                            : 'hover:bg-slate-700'
                    }`}
                    onClick={() => onSelectProvider(provider.id)}
                >
                    <div className="flex items-center">
                        <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center mr-3">
                            {provider.icon}
                        </div>
                        <div>
                            <h3 className="font-medium text-white">{provider.name}</h3>
                            <div className="flex items-center mt-1">
                                <Badge 
                                    variant="primary" 
                                    size="sm"
                                >
                                    {connectorCounts[provider.id] || 0} 
                                    {' '}key{(connectorCounts[provider.id] || 0) !== 1 ? 's' : ''}
                                </Badge>
                            </div>
                        </div>
                    </div>
                </li>
            ))}
        </ul>
    );
};

export default ProvidersList;
