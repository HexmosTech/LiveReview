import { useState, useEffect } from 'react';
import { useNavigate, useLocation, useParams } from 'react-router-dom';
import { AIProvider } from '../types';

interface UseProviderSelectionResult {
    selectedProvider: string;
    setSelectedProvider: (provider: string) => void;
    updateUrlFragment: (provider: string, action?: string | null, connectorId?: string | null) => void;
    isEditing: boolean;
    setIsEditing: (value: boolean) => void;
    initialAction: string | null;
    initialConnectorId: string | null;
}

export const useProviderSelection = (
    providers: AIProvider[]
): UseProviderSelectionResult => {
    const navigate = useNavigate();
    const location = useLocation();
    const params = useParams<{ provider?: string; action?: string; connectorId?: string }>();
    
    // Get initial state from URL params
    const initialProvider = params.provider || 'all';
    const initialAction = params.action || null;
    const initialConnectorId = params.connectorId || null;
    
    // State
    const [selectedProvider, setSelectedProvider] = useState<string>(initialProvider);
    const [isEditing, setIsEditing] = useState(initialAction === 'edit');
    
    // Update URL when selectedProvider changes
    const updateUrlFragment = (provider: string, action: string | null = null, connectorId: string | null = null) => {
        let path = `/ai/${provider}`;
        if (action) {
            path += `/${action}`;
            if (connectorId) {
                path += `/${connectorId}`;
            }
        }
        navigate(path, { replace: true });
    };
    
    // Update selected provider when URL changes
    useEffect(() => {
        const provider = params.provider || 'all';
        if (provider !== selectedProvider) {
            setSelectedProvider(provider);
        }
        
        if (params.action === 'edit') {
            setIsEditing(true);
        } else if (!params.action) {
            setIsEditing(false);
        }
    }, [params, selectedProvider]);
    
    return {
        selectedProvider,
        setSelectedProvider,
        updateUrlFragment,
        isEditing,
        setIsEditing,
        initialAction,
        initialConnectorId
    };
};
