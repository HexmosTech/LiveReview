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
            {providers.map((provider) => {
                const keyCount = connectorCounts[provider.id] || 0;
                return (
                    <li 
                        key={provider.id}
                        className={`p-3 rounded-lg cursor-pointer transition-all ${
                            selectedProvider === provider.id 
                                ? 'bg-slate-700 border-l-4 border-blue-500' 
                                : 'hover:bg-slate-700'
                        }`}
                        onClick={() => onSelectProvider(provider.id)}
                    >
                        <div className="flex items-center justify-between">
                            <div className="flex items-center">
                                <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center mr-3">
                                    {provider.icon}
                                </div>
                                <div>
                                    <h3 className="font-medium text-white leading-tight">
                                        {provider.name}
                                    </h3>
                                    <div className="mt-1">
                                        <Badge variant="primary" size="sm">
                                            {keyCount} key{keyCount !== 1 ? 's' : ''}
                                        </Badge>
                                    </div>
                                </div>
                            </div>
                            <div className="pl-3 flex-shrink-0 flex items-center">
                                {provider.supportLevel === 'recommended' && (
                                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-yellow-100 text-yellow-800 font-medium shadow-sm border border-yellow-200 tracking-wide">Recommended</span>
                                )}
                                {provider.supportLevel === 'experimental' && (
                                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-slate-600 text-slate-300 border border-slate-500 tracking-wide">
                                        Experimental
                                    </span>
                                )}
                            </div>
                        </div>
                    </li>
                );
            })}
        </ul>
    );
};

export default ProvidersList;
