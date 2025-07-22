import { AIProvider } from '../types';

/**
 * Generate a friendly name for a specific provider
 */
export const generateFriendlyNameForProvider = (providerId: string, providers: AIProvider[]) => {
    const providerInfo = providers.find(p => p.id === providerId);
    
    // Generate a friendly name using adjectives and random numbers
    const adjectives = ['Smart', 'Clever', 'Quick', 'Bright', 'Intelligent', 'Sharp', 'Brilliant', 'Creative'];
    const randomAdjective = adjectives[Math.floor(Math.random() * adjectives.length)];
    const randomNum = Math.floor(Math.random() * 1000);
    
    return `${providerInfo?.name || 'AI'}-${randomAdjective}${randomNum}`;
};

/**
 * Get provider details by ID
 */
export const getProviderDetails = (providerId: string, providers: AIProvider[]) => {
    return providers.find(p => p.id === providerId) || providers[0];
};
