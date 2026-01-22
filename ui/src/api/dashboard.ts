// API functions for dashboard data
import apiClient from './apiClient';

export interface ActivityItem {
    id: number;
    action: string;
    repository: string;
    timestamp: string;
    time_ago: string;
    type: string;
}

export interface PerformanceMetrics {
    avg_response_time_seconds: number;
    reviews_this_week: number;
    comments_this_week: number;
    success_rate_percentage: number;
}

export interface SystemStatus {
    job_queue_health: string;
    database_health: string;
    api_health: string;
    last_health_check: string;
}

export interface WebhookHealthSummary {
    total_connectors: number;
    total_projects: number;
    connected_projects: number;
    unconnected_projects: number;
    health_percent: number;
    health_status: 'healthy' | 'partial' | 'setup_required';
    setup_required_connectors: number;
    most_recent_connector_needing_setup_id?: number;
    most_recent_connector_needing_setup_name?: string;
}

export interface ConnectorSetupProgress {
    connector_id: number;
    connector_name: string;
    provider: string;
    // Phase: "discovering" (no projects yet), "installing" (projects exist, webhooks in progress), "ready" (all done), "error" (something failed)
    phase: 'discovering' | 'installing' | 'ready' | 'error';
    total_projects: number;
    connected_projects: number;
    message: string;
}

export interface DashboardData {
    total_reviews: number;
    total_comments: number;
    connected_providers: number;
    active_ai_connectors: number;
    webhook_health?: WebhookHealthSummary;
    connector_setup_progress?: ConnectorSetupProgress[];
    onboarding_api_key?: string;
    api_url: string;
    cli_installed: boolean;
    recent_activity: ActivityItem[];
    performance_metrics: PerformanceMetrics;
    system_status: SystemStatus;
    last_updated: string;
}

export const getDashboardData = async (): Promise<DashboardData> => {
    const response = await apiClient.get<DashboardData>('/api/v1/dashboard');
    return response;
};

// Force-refresh the dashboard cache on the server so counts update immediately
export const refreshDashboardData = async (): Promise<void> => {
    await apiClient.post('/api/v1/dashboard/refresh', {});
};
