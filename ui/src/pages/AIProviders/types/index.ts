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
}

// AI Connector structure (mapped from API)
export interface AIConnector {
    id: string;
    name: string;
    providerName: string;
    apiKey: string;
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
}

export interface ConnectorFormData {
    name: string;
    apiKey: string;
    providerType: string;
}

export interface ValidationResult {
    valid: boolean;
    message?: string;
}
