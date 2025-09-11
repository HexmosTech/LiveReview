import React from 'react';

// AI Provider data structure
export interface AIProvider {
    id: string;
    name: string;
    url: string;
    description: string;
    icon: React.ReactNode;
    apiKeyPlaceholder: string;
    models?: string[]; // Available models for this provider
    defaultModel?: string; // Default model to use
    baseURLPlaceholder?: string; // Placeholder for base URL field (for providers like Ollama)
    requiresBaseURL?: boolean; // Whether this provider requires a base URL
    /**
     * Support level shown in UI. 'recommended' are first-class, well-tested connectors.
     * 'experimental' are available but may change or have limited test coverage.
     */
    supportLevel?: 'recommended' | 'experimental';
}

// AI Connector structure (mapped from API)
export interface AIConnector {
    id: string;
    name: string;
    providerName: string;
    apiKey: string; // Masked API key for display
    fullApiKey?: string; // Full API key for editing
    baseURL?: string; // Base URL for providers like Ollama
    displayOrder: number;
    createdAt: Date;
    lastUsed?: Date;
    usageStats?: {
        totalCalls: number;
        successfulCalls: number;
        failedCalls: number;
        averageLatency: number; // in ms
        lastError?: string;
    };
    models?: string[]; // Available models for this connector
    selectedModel?: string; // Currently selected model
    isActive: boolean; // Whether this connector is active or disabled
    // Legacy field names for backward compatibility
    base_url?: string; 
    selected_model?: string;
}

export interface ConnectorFormData {
    name: string;
    apiKey: string;
    providerType: string;
    baseURL?: string; // Base URL for providers like Ollama
    selectedModel?: string; // Selected model for the connector
}

export interface ValidationResult {
    valid: boolean;
    message?: string;
}
