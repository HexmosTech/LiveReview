import React from 'react';
import { Navbar } from '../Navbar/Navbar';
import { ConnectorForm, ConnectorData } from '../Connector/ConnectorForm';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { addConnector } from '../../store/Connector/reducer';

export const Dashboard: React.FC = () => {
    const dispatch = useAppDispatch();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    // Placeholder stats
    const aiComments = 0;
    const codeReviews = 0;
    const aiService = 'Gemini';
    const apiKey = 'sk-xxxxxxx';

    const handleAddConnector = (connectorData: ConnectorData) => {
        dispatch(addConnector(connectorData));
    };

    return (
        <div className="min-h-screen bg-livereview text-cardText">
            <main className="container mx-auto px-4 py-8">
                <h1 className="text-4xl font-extrabold text-accent mb-8">Dashboard</h1>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8 mb-10">
                    <div className="bg-cardPurple text-cardText shadow-xl rounded-xl p-8 flex flex-col items-center">
                        <span className="text-3xl font-bold mb-2">{connectors.length}</span>
                        <span className="text-lg">Connected Services</span>
                    </div>
                    <div className="bg-cardGreen text-cardText shadow-xl rounded-xl p-8 flex flex-col items-center">
                        <span className="text-3xl font-bold mb-2">{aiComments}</span>
                        <span className="text-lg">AI Comments Posted</span>
                    </div>
                    <div className="bg-cardBlue text-cardText shadow-xl rounded-xl p-8 flex flex-col items-center">
                        <span className="text-3xl font-bold mb-2">{codeReviews}</span>
                        <span className="text-lg">Code Reviews by AI</span>
                    </div>
                    <div className="bg-white text-purple shadow-xl rounded-xl p-8 flex flex-col items-center">
                        <span className="text-3xl font-extrabold mb-2">{aiService}</span>
                        <span className="text-lg">AI Service</span>
                        <span className="text-xs text-gray-400 mt-2">API Key: {apiKey}</span>
                    </div>
                </div>
            </main>
            <footer className="bg-livereview border-t border-cardPurple text-cardText py-8">
                <div className="container mx-auto px-4 flex flex-col md:flex-row justify-between items-center">
                    <div>
                        <h3 className="text-2xl font-bold mb-2 text-accent">LiveReview</h3>
                        <p className="text-lg text-cardText">Automated code reviews made simple</p>
                    </div>
                    <div className="text-right">
                        <p className="text-base text-cardText">© {new Date().getFullYear()} <span className="text-accent font-bold">LiveReview</span>. All rights reserved.</p>
                    </div>
                </div>
            </footer>
        </div>
    );
};
