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
        <div className="flex flex-col min-h-screen bg-gray-50">
            <Navbar title="LiveReview" />
            
            <main className="flex-grow container mx-auto px-4 py-8">
                <div className="flex items-center justify-between mb-8">
                    <h1 className="text-3xl font-bold text-gray-800">Dashboard</h1>
                    <div className="flex items-center space-x-2">
                        <button className="px-4 py-2 bg-white border border-gray-200 rounded-lg shadow-sm hover:bg-gray-50 flex items-center text-gray-700 text-sm font-medium">
                            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
                            </svg>
                            Filter
                        </button>
                        <button className="px-4 py-2 bg-white border border-gray-200 rounded-lg shadow-sm hover:bg-gray-50 flex items-center text-gray-700 text-sm font-medium">
                            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                            </svg>
                            Export
                        </button>
                    </div>
                </div>
                
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
                                                        {connector.type === 'gitlab' && (
                                                            <span className="flex-shrink-0 w-8 h-8 bg-orange-100 text-orange-500 rounded-md flex items-center justify-center mr-3">
                                                                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                                                    <path d="M22.65 14.39L12 22.13 1.35 14.39a.84.84 0 0 1-.3-.94l1.22-3.78 2.44-7.51A.42.42 0 0 1 4.82 2a.43.43 0 0 1 .58 0 .42.42 0 0 1 .11.18l2.44 7.49h8.1l2.44-7.51A.42.42 0 0 1 18.6 2a.43.43 0 0 1 .58 0 .42.42 0 0 1 .11.18l2.44 7.51L23 13.45a.84.84 0 0 1-.35.94z"></path>
                                                                </svg>
                                                            </span>
                                                        )}
                                                        {connector.type === 'github' && (
                                                            <span className="flex-shrink-0 w-8 h-8 bg-gray-100 text-gray-700 rounded-md flex items-center justify-center mr-3">
                                                                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                                                    <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"></path>
                                                                </svg>
                                                            </span>
                                                        )}
                                                        {connector.type === 'custom' && (
                                                            <span className="flex-shrink-0 w-8 h-8 bg-blue-100 text-blue-500 rounded-md flex items-center justify-center mr-3">
                                                                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                                                                </svg>
                                                            </span>
                                                        )}
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
                                                            // Future implementation: test connection
                                                            alert(
                                                                `Testing connection to ${connector.name}`
                                                            );
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
            </main>
            
            <footer className="bg-gradient-to-r from-indigo-600 to-purple-600 text-white py-6">
                <div className="container mx-auto px-4">
                    <div className="flex flex-col md:flex-row justify-between items-center">
                        <div className="mb-4 md:mb-0">
                            <h3 className="text-lg font-bold mb-2">LiveReview</h3>
                            <p className="text-indigo-200 text-sm">Automated code reviews made simple</p>
                        </div>
                        <div className="text-center md:text-right">
                            <p className="text-indigo-200 text-sm">© {new Date().getFullYear()} LiveReview. All rights reserved.</p>
                        </div>
                    </div>
                </div>
            </footer>
        </div>
    );
};
