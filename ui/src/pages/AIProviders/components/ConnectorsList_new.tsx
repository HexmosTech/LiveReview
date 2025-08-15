import React, { useState } from 'react';
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
    reorderConnectors: (newOrder: AIConnector[]) => Promise<boolean>;
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
    dropdownRef,
    reorderConnectors
}) => {
    const [isReorderMode, setIsReorderMode] = useState(false);
    const [draggedConnector, setDraggedConnector] = useState<AIConnector | null>(null);
    const [tempOrder, setTempOrder] = useState<AIConnector[]>([]);

    const getProviderDetails = (providerId: string) => {
        return providers.find(p => p.id === providerId) || providers[0];
    };
    
    const filteredConnectors = selectedProvider === 'all' 
        ? connectors 
        : connectors.filter(c => c.providerName === selectedProvider);
        
    const sortedConnectors = [...filteredConnectors].sort((a, b) => a.displayOrder - b.displayOrder);

    const handleToggleReorderMode = () => {
        if (isReorderMode) {
            // Cancel reorder mode
            setIsReorderMode(false);
            setTempOrder([]);
        } else {
            // Enter reorder mode
            setIsReorderMode(true);
            setTempOrder([...sortedConnectors]);
        }
    };

    const handleSaveOrder = async () => {
        const success = await reorderConnectors(tempOrder);
        if (success) {
            setIsReorderMode(false);
            setTempOrder([]);
        }
    };

    const handleDragStart = (e: React.DragEvent, connector: AIConnector) => {
        setDraggedConnector(connector);
        e.dataTransfer.effectAllowed = 'move';
    };

    const handleDragOver = (e: React.DragEvent) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
    };

    const handleDrop = (e: React.DragEvent, targetConnector: AIConnector) => {
        e.preventDefault();
        
        if (!draggedConnector || draggedConnector.id === targetConnector.id) {
            return;
        }

        const newOrder = [...tempOrder];
        const draggedIndex = newOrder.findIndex(c => c.id === draggedConnector.id);
        const targetIndex = newOrder.findIndex(c => c.id === targetConnector.id);

        // Remove dragged item and insert at target position
        newOrder.splice(draggedIndex, 1);
        newOrder.splice(targetIndex, 0, draggedConnector);

        setTempOrder(newOrder);
        setDraggedConnector(null);
    };

    const handleDragEnd = () => {
        setDraggedConnector(null);
    };

    const connectorsToDisplay = isReorderMode ? tempOrder : sortedConnectors;

    return (
        <Card 
            title="Your Connectors" 
            badge={`${filteredConnectors.length}`}
        >
            <div className="flex justify-between items-center mb-4">
                {/* Reorder controls - only show for "All Connectors" */}
                {selectedProvider === 'all' && connectorsToDisplay.length > 1 && (
                    <div className="flex items-center space-x-2">
                        {!isReorderMode ? (
                            <Button
                                variant="outline"
                                size="sm"
                                onClick={handleToggleReorderMode}
                                icon={<Icons.List />}
                            >
                                Reorder
                            </Button>
                        ) : (
                            <>
                                <Button
                                    variant="primary"
                                    size="sm"
                                    onClick={handleSaveOrder}
                                    icon={<Icons.Success />}
                                >
                                    Save Order
                                </Button>
                                <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={handleToggleReorderMode}
                                    icon={<Icons.Error />}
                                >
                                    Cancel
                                </Button>
                            </>
                        )}
                    </div>
                )}
                
                {/* Add Connector button */}
                <div className="flex space-x-2">
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
                <div className="text-center py-8">
                    <EmptyState
                        icon={<Icons.EmptyState />}
                        title={selectedProvider === 'all' 
                            ? "No AI connectors configured" 
                            : `No ${getProviderDetails(selectedProvider).name} connectors configured`}
                        description={selectedProvider === 'all'
                            ? "Add your first AI connector to start using AI-powered code reviews"
                            : `Add your first ${getProviderDetails(selectedProvider).name} connector to start using this AI service`}
                    />
                    <div className="mt-4 text-sm text-slate-400">
                        Click "Add Connector" above to get started
                    </div>
                </div>
            ) : (
                <ul className="space-y-4">
                    {connectorsToDisplay.map((connector, index) => (
                        <div
                            key={connector.id}
                            draggable={isReorderMode}
                            onDragStart={(e) => handleDragStart(e, connector)}
                            onDragOver={handleDragOver}
                            onDrop={(e) => handleDrop(e, connector)}
                            onDragEnd={handleDragEnd}
                            className={`${isReorderMode ? 'cursor-move' : ''}`}
                        >
                            <ConnectorCard
                                connector={connector}
                                onEdit={() => onEditConnector(connector)}
                                isFirst={index === 0}
                                isLast={index === connectorsToDisplay.length - 1}
                            />
                        </div>
                    ))}
                </ul>
            )}
        </Card>
    );
};

export default ConnectorsList;
