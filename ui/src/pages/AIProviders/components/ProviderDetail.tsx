import React from 'react';
import { AIProvider } from '../types';

interface ProviderDetailProps {
    provider: AIProvider;
}

const ProviderDetail: React.FC<ProviderDetailProps> = ({ provider }) => {
    return (
        <div className="mt-6 p-4 bg-slate-700 rounded-lg">
            <h3 className="text-lg font-medium text-white mb-2">
                {provider.name}
            </h3>
            <p className="text-sm text-slate-300 mb-3">
                {provider.description}
            </p>
            <a 
                href={provider.url} 
                target="_blank" 
                rel="noopener noreferrer" 
                className="text-sm text-blue-400 hover:text-blue-300 flex items-center"
            >
                Visit Documentation
            </a>
        </div>
    );
};

export default ProviderDetail;
