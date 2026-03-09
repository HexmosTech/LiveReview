import React from 'react';
import { AIProvider } from '../types';

interface ProviderDetailProps {
    provider: AIProvider;
}

const ProviderDetail: React.FC<ProviderDetailProps> = ({ provider }) => {
    return (
        <div className="mt-6 p-4 bg-slate-700 rounded-lg">
            <div className="flex items-center flex-wrap gap-2 mb-2">
                <h3 className="text-lg font-medium text-white">
                    {provider.name}
                </h3>
            </div>
            <p className="text-sm text-slate-300 mb-3 leading-relaxed">
                {provider.description}
            </p>
            <div className="flex flex-col space-y-3">
                <a 
                    href={provider.url} 
                    target="_blank" 
                    rel="noopener noreferrer" 
                    className="text-sm text-blue-400 hover:text-blue-300 flex items-center"
                >
                    Visit Documentation
                </a>
            </div>
        </div>
    );
};

export default ProviderDetail;
