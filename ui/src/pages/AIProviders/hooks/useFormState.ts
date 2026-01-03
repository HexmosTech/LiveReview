import { useState } from 'react';
import { AIConnector, ConnectorFormData } from '../types';
import { generateFriendlyNameForProvider } from '../utils/nameUtils';

interface UseFormStateResult {
    formData: ConnectorFormData;
    selectedConnector: AIConnector | null;
    setSelectedConnector: (connector: AIConnector | null) => void;
    handleInputChange: (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => void;
    handleProviderTypeChange: (providerType: string, providers: any[]) => void;
    resetForm: () => void;
    setFormData: (data: ConnectorFormData) => void;
    generateFriendlyName: (providerId: string, providers: any[]) => void;
}

export const useFormState = (
    initialFormData: ConnectorFormData = { name: '', apiKey: '', providerType: '' }
): UseFormStateResult => {
    const [formData, setFormData] = useState<ConnectorFormData>(initialFormData);
    const [selectedConnector, setSelectedConnector] = useState<AIConnector | null>(null);

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
        const { name, value } = e.target;
        setFormData({
            ...formData,
            [name]: value
        });
    };

    const handleProviderTypeChange = (providerType: string, providers: any[]) => {
        const providerMeta = providers.find((p: any) => p.id === providerType);
        const defaultModel = providerMeta?.defaultModel || '';
        setFormData({
            ...formData,
            providerType,
            name: formData.name || generateFriendlyNameForProvider(providerType, providers),
            // Reset model selection when provider changes
            selectedModel: defaultModel,
            // Reset base URL when provider changes to non-Ollama
            baseURL: providerType === 'ollama' ? formData.baseURL : ''
        });
    };

    const resetForm = () => {
        setFormData({
            name: '',
            apiKey: '',
            providerType: '',
            baseURL: '',
            selectedModel: ''
        });
        setSelectedConnector(null);
    };

    const generateFriendlyName = (providerId: string, providers: any[]) => {
        setFormData({
            ...formData,
            name: generateFriendlyNameForProvider(providerId, providers)
        });
    };

    return {
        formData,
        selectedConnector,
        setSelectedConnector,
        handleInputChange,
        handleProviderTypeChange,
        resetForm,
        setFormData,
        generateFriendlyName
    };
};
