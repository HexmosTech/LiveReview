// Shared billing/quota query hooks. Several independent components (Navbar, Dashboard,
// NewReview, TeamCheckout, SubscriptionTab) each need billing/quota data and previously fetched
// it themselves on mount - on a single page load that meant the same endpoint firing 2-3x with
// no caching. These hooks route every consumer through react-query's shared cache instead, so
// they coalesce into one request/result. Each hook stays generic over its response type so
// existing call sites can keep their own locally-defined response shape.
// See docs/perf-improvement.md ("Fix 2").
import { useQuery, UseQueryOptions } from '@tanstack/react-query';
import apiClient from './apiClient';

type QueryOpts<T> = Omit<UseQueryOptions<T>, 'queryKey' | 'queryFn'>;

export const BILLING_STATUS_QUERY_KEY = ['billing-status'] as const;
export const QUOTA_STATUS_QUERY_KEY = ['quota-status'] as const;
export const BILLING_UPGRADE_STATUS_QUERY_KEY = ['billing-upgrade-request-status'] as const;
export const BILLING_USAGE_ME_QUERY_KEY = ['billing-usage-me'] as const;

export function useBillingStatusQuery<T = unknown>(options?: QueryOpts<T>) {
    return useQuery<T>({
        queryKey: BILLING_STATUS_QUERY_KEY,
        queryFn: () => apiClient.get<T>('/billing/status'),
        ...options,
    });
}

export function useQuotaStatusQuery<T = unknown>(options?: QueryOpts<T>) {
    return useQuery<T>({
        queryKey: QUOTA_STATUS_QUERY_KEY,
        queryFn: () => apiClient.get<T>('/quota/status'),
        ...options,
    });
}

export function useBillingUpgradeStatusQuery<T = unknown>(options?: QueryOpts<T>) {
    return useQuery<T>({
        queryKey: BILLING_UPGRADE_STATUS_QUERY_KEY,
        queryFn: () => apiClient.get<T>('/billing/upgrade/request-status'),
        ...options,
    });
}

export function useBillingUsageMeQuery<T = unknown>(options?: QueryOpts<T>) {
    return useQuery<T>({
        queryKey: BILLING_USAGE_ME_QUERY_KEY,
        queryFn: () => apiClient.get<T>('/billing/usage/me'),
        ...options,
    });
}

export function useBillingUsageMembersQuery<T = unknown>(limit = 3, offset = 0, options?: QueryOpts<T>) {
    return useQuery<T>({
        queryKey: ['billing-usage-members', limit, offset],
        queryFn: () => apiClient.get<T>(`/billing/usage/members?limit=${limit}&offset=${offset}`),
        ...options,
    });
}
