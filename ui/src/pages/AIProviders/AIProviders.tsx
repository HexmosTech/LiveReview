import React from 'react';

const popularAIProviders = [
    { name: 'OpenAI', url: 'https://platform.openai.com/', apiKey: 'sk-xxxxxxx' },
    { name: 'Google Gemini', url: 'https://ai.google.dev/', apiKey: 'gemini-xxxxxxx' },
    { name: 'Anthropic Claude', url: 'https://www.anthropic.com/', apiKey: 'claude-xxxxxxx' },
    { name: 'Cohere', url: 'https://cohere.com/', apiKey: 'cohere-xxxxxxx' },
];

const AIProviders: React.FC = () => {
    return (
        <div className="container mx-auto px-4 py-8">
            <h1 className="text-2xl font-bold text-gray-800 mb-6">AI Providers</h1>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
                {popularAIProviders.map((provider) => (
                    <div key={provider.name} className="bg-white shadow-xl rounded-xl p-6 border border-gray-100 flex flex-col items-center">
                        <h2 className="text-xl font-bold text-indigo-700 mb-2">{provider.name}</h2>
                        <a href={provider.url} target="_blank" rel="noopener noreferrer" className="text-blue-500 underline mb-2">Visit API Docs</a>
                        <span className="text-xs text-gray-400 mt-2">API Key: {provider.apiKey}</span>
                    </div>
                ))}
            </div>
        </div>
    );
};

export default AIProviders;
