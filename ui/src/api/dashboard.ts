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

export interface ReviewLayerCategoryCount {
    category: string;
    count: number;
}

export interface ReviewLayer {
    id: string;
    label: string;
    reviews_run: number;
    issues_found: number;
    categories: ReviewLayerCategoryCount[];
}

export interface ReviewLayers {
    day: ReviewLayer[];
    week: ReviewLayer[];
    month: ReviewLayer[];
    all: ReviewLayer[];
}

// Trailing-window count: day=added in the last 1 day, week=last 7, month=last 30, all=ever.
export interface CountsByWindow {
    day: number;
    week: number;
    month: number;
    all: number;
}

// Trailing-window: each period is activity in the last N days (day=1, week=7, month=30, all=unbounded).
export interface RatesByWindow {
    day: number;
    week: number;
    month: number;
    all: number;
}

export interface RepoOverview {
    name: string;
    pr_count: CountsByWindow;
    coverage_pct: RatesByWindow;
}

export interface HostRepoGroup {
    host: string;
    repos: RepoOverview[];
}

export interface ConnectedProvider {
    id: string;
    name: string;
    kind: 'git' | 'ai';
    provider: string;
    last_synced_at: string;
}

export interface SystemOverview {
    git_hosts: CountsByWindow;
    ai_connectors: CountsByWindow;
    total_repos: CountsByWindow;
    total_prs: CountsByWindow;
    coverage_pct: RatesByWindow;
    avg_reviews_per_pr: RatesByWindow;
    avg_reviews_per_commit: RatesByWindow;
    repo_hierarchy: HostRepoGroup[];
    connected_providers: ConnectedProvider[];
}

export interface PeopleContributor {
    email: string;
    name: string;
    reviews_given: number;
    loc_reviewed: number;
}

// Trailing-window, same reasoning as SystemOverview's rate metrics — a "top reviewer" ranking should reflect recent activity.
export interface PeopleContributorsByWindow {
    day: PeopleContributor[];
    week: PeopleContributor[];
    month: PeopleContributor[];
    all: PeopleContributor[];
}

export interface CalendarDay {
    date: string;
    count: number;
}

export interface People {
    contributors: PeopleContributorsByWindow;
    // Not period-scoped — always the full lookback range, same treatment as ConnectedProviders.
    contribution_calendar: CalendarDay[];
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
    review_layers?: ReviewLayers;
    system_overview?: SystemOverview;
    people?: People;
    last_updated: string;
}

// Coalesces concurrent callers into a single network request. Dashboard.tsx and each of the
// three widget-layer providers (ReviewLayers/SystemOverview/People) independently call this on
// mount, all within the same render — without this they'd fire as separate simultaneous GETs
// for the exact same response.
let inFlightDashboardDataRequest: Promise<DashboardData> | null = null;

export const getDashboardData = async (): Promise<DashboardData> => {
    if (inFlightDashboardDataRequest) {
        return inFlightDashboardDataRequest;
    }
    inFlightDashboardDataRequest = apiClient.get<DashboardData>('/api/v1/dashboard').finally(() => {
        inFlightDashboardDataRequest = null;
    });
    return inFlightDashboardDataRequest;
};

export interface RefreshDashboardResponse {
    message: string;
    last_updated: string;
    review_layers?: ReviewLayers;
    system_overview?: SystemOverview;
    people?: People;
}

// Force-refresh the dashboard cache on the server so counts update immediately
export const refreshDashboardData = async (): Promise<RefreshDashboardResponse> => {
    return apiClient.post<RefreshDashboardResponse>('/api/v1/dashboard/refresh', {});
};
