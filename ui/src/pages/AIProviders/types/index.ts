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
}

// AI Connector structure (mapped from API)
export interface AIConnector {
    id: string;
    name: string;
    providerName: string;
    role?: 'leader' | 'helper';
    apiKey: string; // Masked API key for display
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
    gcpProjectID?: string;
    gcpLocation?: string;
    gcp_project_id?: string;
    gcp_location?: string;
    awsAccessKeyID?: string;
    awsRegion?: string;
}

export interface ConnectorFormData {
    name: string;
    apiKey: string;
    providerType: string;
    role?: 'leader' | 'helper';
    baseURL?: string; // Base URL for providers like Ollama
    selectedModel?: string; // Selected model for the connector
    gcpProjectID?: string;
    gcpLocation?: string;
    awsAccessKeyID?: string;
    awsRegion?: string;
}

export interface ReviewAISettings {
    helper_enabled: boolean;
    helper_mode: 'concise_then_expand' | 'polish_only';
    // Optional soft preferences - null means "use the system default".
    // Always clamped server-side to the instance-wide ceiling: an org can
    // turn these down, never up past what the instance allows.
    preferred_max_concurrent_batches?: number | null;
    preferred_review_timeout_seconds?: number | null;
    // What will actually be used right now (preference resolved against
    // the current instance-wide ceiling), returned by GET/PUT so the UI
    // can show "you asked for X, the instance allows up to Y".
    effective_max_concurrent_batches?: number;
    effective_review_timeout_seconds?: number;
}

// Instance-wide review concurrency/timeout ceiling (super-admin / self-hosted
// owner only). Bounds every org's own preference above.
export interface AIReviewPerformanceSettings {
    max_concurrent_batches_ceiling: number;
    min_review_timeout_floor_seconds: number;
    max_review_timeout_ceiling_seconds: number;
}

export interface ValidationResult {
    valid: boolean;
    message?: string;
}
