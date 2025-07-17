import React, { useState } from 'react';
import { 
    PageHeader, 
    Card, 
    Button, 
    Icons, 
    Input,
    Alert, 
    Section 
} from '../../components/UIPrimitives';

const popularAIProviders = [
    { 
        id: 'openai',
        name: 'OpenAI', 
        url: 'https://platform.openai.com/', 
        apiKey: 'sk-xxxxxxx',
        icon: <Icons.OpenAI />,
        description: 'Access GPT models for code understanding and generation',
        isConfigured: true
    },
    { 
        id: 'gemini',
        name: 'Google Gemini', 
        url: 'https://ai.google.dev/', 
        apiKey: 'gemini-xxxxxxx',
        icon: <Icons.Google />,
        description: 'Google\'s multimodal AI for code and natural language tasks',
        isConfigured: true
    },
    { 
        id: 'claude',
        name: 'Anthropic Claude', 
        url: 'https://www.anthropic.com/', 
        apiKey: 'claude-xxxxxxx',
        icon: <Icons.AI />,
        description: 'Constitutional AI focused on helpful, harmless responses',
        isConfigured: false
    },
    { 
        id: 'cohere',
        name: 'Cohere', 
        url: 'https://cohere.com/', 
        apiKey: 'cohere-xxxxxxx',
        icon: <Icons.AI />,
        description: 'Specialized in understanding and generating human language',
        isConfigured: false
    },
];

const AIProviders: React.FC = () => {
    const [activeProvider, setActiveProvider] = useState('openai');
    const [apiKeys, setApiKeys] = useState({
        openai: 'sk-xxxxxxx',
        gemini: 'gemini-xxxxxxx',
        claude: '',
        cohere: ''
    });
    const [isSaved, setIsSaved] = useState(false);

    const handleSaveApiKey = () => {
        setIsSaved(true);
        setTimeout(() => setIsSaved(false), 3000);
    };

    const handleApiKeyChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setApiKeys({
            ...apiKeys,
            [activeProvider]: e.target.value
        });
    };

    return (
        <div className="container mx-auto px-4 py-8">
            <PageHeader 
                title="AI Providers" 
                description="Configure and manage AI services for code review"
            />

            <Section>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
                    {popularAIProviders.map((provider) => (
                        <div 
                            key={provider.id}
                            className={`cursor-pointer transition-all ${activeProvider === provider.id ? 'ring-2 ring-blue-500 ring-offset-2' : 'hover:shadow-md'}`}
                            onClick={() => setActiveProvider(provider.id)}
                        >
                            <Card className={activeProvider === provider.id ? 'card-brand' : ''}>
                                <div className="flex flex-col items-center text-center">
                                    <div className="h-12 w-12 rounded-full bg-blue-100 flex items-center justify-center mb-4">
                                        {provider.icon}
                                    </div>
                                    <h3 className="text-lg font-medium text-gray-900 mb-1">{provider.name}</h3>
                                    <p className="text-sm text-gray-500 mb-3">{provider.description}</p>
                                    <div className="w-full">
                                        {provider.isConfigured ? (
                                            <Button 
                                                variant="ghost" 
                                                size="sm" 
                                                fullWidth
                                                className="border border-green-200 text-green-700 bg-green-50"
                                            >
                                                Configured
                                            </Button>
                                        ) : (
                                            <Button 
                                                variant="ghost" 
                                                size="sm" 
                                                fullWidth
                                                className="border border-gray-200"
                                            >
                                                Not Configured
                                            </Button>
                                        )}
                                    </div>
                                </div>
                            </Card>
                        </div>
                    ))}
                </div>
            </Section>

            <Section title="API Configuration" description="Manage API keys for the selected provider">
                <Card>
                    {isSaved && (
                        <Alert 
                            variant="success" 
                            icon={<Icons.Success />}
                            className="mb-4"
                        >
                            API key saved successfully!
                        </Alert>
                    )}
                    
                    <div className="space-y-5">
                        {popularAIProviders.find(p => p.id === activeProvider) && (
                            <div className="flex items-center">
                                <div className="mr-4">
                                    <div className="h-12 w-12 rounded-full bg-indigo-100 flex items-center justify-center">
                                        {popularAIProviders.find(p => p.id === activeProvider)?.icon}
                                    </div>
                                </div>
                                <div>
                                    <h3 className="text-lg font-medium text-gray-900">
                                        {popularAIProviders.find(p => p.id === activeProvider)?.name}
                                    </h3>
                                    <a 
                                        href={popularAIProviders.find(p => p.id === activeProvider)?.url} 
                                        target="_blank" 
                                        rel="noopener noreferrer" 
                                        className="text-sm text-indigo-600 hover:text-indigo-800"
                                    >
                                        Visit API Documentation
                                    </a>
                                </div>
                            </div>
                        )}
                        
                        <Input
                            label="API Key"
                            type="password"
                            value={apiKeys[activeProvider as keyof typeof apiKeys] || ''}
                            onChange={handleApiKeyChange}
                            placeholder="Enter your API key"
                            helperText="Your API key will be stored securely"
                        />
                        
                        <div className="flex space-x-3">
                            <Button
                                variant="primary"
                                onClick={handleSaveApiKey}
                            >
                                Save API Key
                            </Button>
                            <Button
                                variant="outline"
                                onClick={() => {
                                    setApiKeys({
                                        ...apiKeys,
                                        [activeProvider]: ''
                                    });
                                }}
                            >
                                Clear
                            </Button>
                        </div>
                    </div>
                </Card>
            </Section>
        </div>
    );
};

export default AIProviders;
