import React from 'react';
import { AIConnector, AIProvider } from '../types';
import { 
    Card, 
    Button, 
    Icons, 
    EmptyState,
    Spinner
} from '../../../components/UIPrimitives';
import ConnectorCard from './ConnectorCard';

interface ConnectorsListProps {
    connectors: AIConnector[];
    providers: AIProvider[];
    selectedProvider: string;
    isLoading: boolean;
    error: string | null;
    onEditConnector: (connector: AIConnector) => void;
    onAddConnector: () => void;
    onRetry: () => void;
    showAddDropdown: boolean;
    onToggleDropdown: () => void;
    onSelectProviderToAdd: (providerId: string) => void;
    dropdownRef: React.RefObject<HTMLDivElement>;
}

const ConnectorsList: React.FC<ConnectorsListProps> = ({
    connectors,
    providers,
    selectedProvider,
    isLoading,
    error,
    onEditConnector,
    onAddConnector,
    onRetry,
    showAddDropdown,
    onToggleDropdown,
    onSelectProviderToAdd,
    dropdownRef
}) => {
    const getProviderDetails = (providerId: string) => {
        return providers.find(p => p.id === providerId) || providers[0];
    };
    
    const filteredConnectors = selectedProvider === 'all' 
        ? connectors 
        : connectors.filter(c => c.providerName === selectedProvider);
        
    const sortedConnectors = [...filteredConnectors].sort((a, b) => a.displayOrder - b.displayOrder);

    return (
        <Card 
            title="Your Connectors" 
            badge={`${filteredConnectors.length}`}
        >
            <div className="flex justify-end mb-4">
                {selectedProvider === 'all' ? (
                    <div className="relative" ref={dropdownRef}>
                        <Button
                            variant="primary"
                            size="sm"
                            onClick={onToggleDropdown}
                            className="flex items-center"
                        >
                            Add Connector
                            <span className="ml-1">
                                <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                                </svg>
                            </span>
                        </Button>
                        
                        {showAddDropdown && (
                            <div className="absolute right-0 mt-2 w-56 rounded-md shadow-lg bg-slate-800 ring-1 ring-black ring-opacity-5 z-10">
                                <div className="py-1" role="menu" aria-orientation="vertical">
                                    <div className="px-3 py-2 text-xs text-slate-400 uppercase">
                                        Select Provider
                                    </div>
                                    {providers.map(provider => (
                                        <button
                                            key={provider.id}
                                            className="w-full text-left px-4 py-2 text-sm text-white hover:bg-slate-700 flex items-center"
                                            onClick={() => onSelectProviderToAdd(provider.id)}
                                        >
                                            <span className="w-8 h-8 flex-shrink-0 rounded-full bg-indigo-600 flex items-center justify-center mr-3">
                                                {provider.icon}
                                            </span>
                                            {provider.name}
                                        </button>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                ) : (
                    <Button
                        variant="primary"
                        size="sm"
                        onClick={onAddConnector}
                    >
                        Add {getProviderDetails(selectedProvider).name} Connector
                    </Button>
                )}
            </div>
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
                        onClick={onRetry}
                    >
                        Retry
                    </Button>
                </div>
            ) : sortedConnectors.length === 0 ? (
                <EmptyState
                    icon={<Icons.EmptyState />}
                    title={selectedProvider === 'all' 
                        ? "No AI connectors yet" 
                        : `No ${getProviderDetails(selectedProvider).name} connectors yet`}
                    description={selectedProvider === 'all'
                        ? "Add your first AI connector to start using AI services"
                        : `Add your first ${getProviderDetails(selectedProvider).name} connector to start using this AI service`}
                />
            ) : (
                <ul className="space-y-4">
                    {sortedConnectors.map((connector, index) => (
                        <ConnectorCard
                            key={connector.id}
                            connector={connector}
                            onEdit={() => onEditConnector(connector)}
                            isFirst={index === 0}
                            isLast={index === sortedConnectors.length - 1}
                        />
                    ))}
                </ul>
            )}
        </Card>
    );
};

export default ConnectorsList;
