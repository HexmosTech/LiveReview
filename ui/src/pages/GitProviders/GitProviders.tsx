import React from 'react';
import { ConnectorForm, ConnectorData } from '../../components/Connector/ConnectorForm';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { addConnector } from '../../store/Connector/reducer';

const GitProviders: React.FC = () => {
    const dispatch = useAppDispatch();
    const connectors = useAppSelector((state) => state.Connector.connectors);

    const handleAddConnector = (connectorData: ConnectorData) => {
        dispatch(addConnector(connectorData));
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <h1 className="text-2xl font-bold text-gray-800 mb-6">Git Providers</h1>
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                <div>
                    <ConnectorForm onSubmit={handleAddConnector} />
                </div>
                <div>
                    <div className="bg-white shadow-xl rounded-xl p-6 border border-gray-100 h-full">
                        <div className="flex items-center justify-between mb-6">
                            <h2 className="text-xl font-bold text-indigo-700">
                                Your Connectors
                            </h2>
                            <span className="bg-indigo-100 text-indigo-800 text-xs font-medium px-2.5 py-0.5 rounded-full">
                                {connectors.length} total
                            </span>
                        </div>
                        {connectors.length === 0 ? (
                            <div className="flex flex-col items-center justify-center py-12 text-center">
                                <svg className="w-16 h-16 text-gray-300 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M17 14v6m-3-3h6M6 10h2a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v2a2 2 0 002 2zm10 0h2a2 2 0 002-2V6a2 2 0 00-2-2h-2a2 2 0 00-2 2v2a2 2 0 002 2zM6 20h2a2 2 0 002-2v-2a2 2 0 00-2-2H6a2 2 0 00-2 2v2a2 2 0 002 2z" />
                                </svg>
                                <p className="text-gray-500 mb-2">
                                    No connectors yet
                                </p>
                                <p className="text-gray-400 text-sm">
                                    Create your first connector to start integrating with your code repositories
                                </p>
                            </div>
                        ) : (
                            <ul className="space-y-3">
                                {connectors.map((connector) => (
                                    <li
                                        key={connector.id}
                                        className="border border-gray-100 p-4 rounded-lg hover:bg-gray-50 transition duration-150 ease-in-out"
                                    >
                                        <div className="flex justify-between items-center">
                                            <div>
                                                <div className="flex items-center">
                                                    {/* ...existing code for connector icons and info... */}
                                                    <div>
                                                        <h3 className="font-medium text-gray-800">
                                                            {connector.name}
                                                        </h3>
                                                        <p className="text-sm text-gray-500">
                                                            {connector.url}
                                                        </p>
                                                    </div>
                                                </div>
                                            </div>
                                            <div className="flex items-center space-x-2">
                                                <span className="text-xs text-gray-500">{new Date(connector.createdAt).toLocaleDateString()}</span>
                                                <button
                                                    className="text-xs bg-indigo-50 hover:bg-indigo-100 text-indigo-600 px-3 py-1.5 rounded-lg transition font-medium"
                                                    onClick={() => {
                                                        alert(`Testing connection to ${connector.name}`);
                                                    }}
                                                >
                                                    Test Connection
                                                </button>
                                            </div>
                                        </div>
                                    </li>
                                ))}
                            </ul>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
};

export default GitProviders;
