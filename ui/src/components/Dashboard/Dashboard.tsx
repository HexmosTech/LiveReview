import React from 'react';
import { Navbar } from '../Navbar/Navbar';
import { ConnectorForm, ConnectorData } from '../Connector/ConnectorForm';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { addConnector } from '../../store/Connector/reducer';

export const Dashboard: React.FC = () => {
    const dispatch = useAppDispatch();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    
    const handleAddConnector = (connectorData: ConnectorData) => {
        dispatch(addConnector(connectorData));
    };
    
    return (
        <div className="flex flex-col min-h-screen bg-gray-100">
            <Navbar title="LiveReview" />
            
            <main className="flex-grow container mx-auto px-4 py-6">
                <h1 className="text-2xl font-bold mb-6">Dashboard</h1>
                
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <div>
                        <ConnectorForm onSubmit={handleAddConnector} />
                    </div>
                    
                    <div>
                        <div className="bg-white shadow-md rounded-lg p-6">
                            <h2 className="text-xl font-semibold mb-4">
                                Your Connectors
                            </h2>
                            
                            {connectors.length === 0 ? (
                                <p className="text-gray-500">
                                    No connectors yet. Create one to get
                                    started.
                                </p>
                            ) : (
                                <ul className="space-y-2">
                                    {connectors.map((connector) => (
                                        <li
                                            key={connector.id}
                                            className="border p-3 rounded-md hover:bg-gray-50"
                                        >
                                            <div className="flex justify-between items-center">
                                                <div>
                                                    <h3 className="font-medium">
                                                        {connector.name}
                                                    </h3>
                                                    <p className="text-sm text-gray-600">
                                                        {connector.type} • 
                                                        {connector.url}
                                                    </p>
                                                </div>
                                                <button
                                                    className="text-xs bg-slate-200 hover:bg-slate-300 px-2 py-1 rounded transition"
                                                    onClick={() => {
                                                        // Future implementation: test connection
                                                        alert(
                                                            `Testing connection to ${connector.name}`
                                                        );
                                                    }}
                                                >
                                                    Test Connection
                                                </button>
                                            </div>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>
                    </div>
                </div>
            </main>
            
            <footer className="bg-slate-800 text-white py-4">
                <div className="container mx-auto px-4 text-center">
                    <p>LiveReview © {new Date().getFullYear()}</p>
                </div>
            </footer>
        </div>
    );
};
