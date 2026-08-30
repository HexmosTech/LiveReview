import { useState, useEffect } from 'react';
import apiClient from '../api/apiClient';

/**
 * Checks whether the production URL has been configured in Settings → Instance.
 * Returns { isConfigured, isLoading } so callers can show a prerequisite warning
 * when the URL is missing (same pattern as UserForm.tsx's checkPrerequisites).
 */
export function useProductionUrlCheck() {
    const [isConfigured, setIsConfigured] = useState<boolean | null>(null);
    const [isLoading, setIsLoading] = useState(true);

    useEffect(() => {
        const check = async () => {
            try {
                const response = await apiClient.get<{ url: string }>('/api/v1/production-url');
                setIsConfigured(!!response?.url);
            } catch {
                setIsConfigured(false);
            } finally {
                setIsLoading(false);
            }
        };
        check();
    }, []);

    return { isConfigured, isLoading };
}
