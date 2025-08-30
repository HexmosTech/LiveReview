import apiClient from './apiClient';
import { Organization, OrganizationMember } from '../store/Organizations/types';

export interface CreateOrganizationRequest {
    name: string;
    description?: string;
}

export interface UpdateOrganizationRequest {
    name?: string;
    description?: string;
    is_active?: boolean;
}

export interface ChangeRoleRequest {
    role: string;
}

export interface OrganizationsResponse {
    organizations: any[]; // Using any to handle the raw API response before mapping
    total: number;
}

/**
 * Organizations API client
 */
export const organizationsApi = {
    /**
     * Get all organizations for the current user
     */
    async getUserOrganizations(): Promise<Organization[]> {
        const response = await apiClient.get<{ organizations: any[] }>('/organizations');
        return response.organizations.map(org => ({
            ...org,
            role: org.role_name, // Map role_name to role
        }));
    },

    /**
     * Get all organizations (super admin only)
     */
    async getAllOrganizations(): Promise<Organization[]> {
        const response = await apiClient.get<OrganizationsResponse>('/api/v1/admin/organizations');
        return response.organizations.map(org => ({
            ...org,
            role: org.role_name, // Map role_name to role
        }));
    },

    /**
     * Get organization details by ID
     */
    async getOrganization(orgId: number): Promise<Organization> {
        return apiClient.get<Organization>(`/api/v1/organizations/${orgId}`);
    },

    /**
     * Create a new organization (super admin only)
     */
    async createOrganization(data: CreateOrganizationRequest): Promise<Organization> {
        return apiClient.post<Organization>('/api/v1/admin/organizations', data);
    },

    /**
     * Update an organization
     */
    async updateOrganization(orgId: number, data: UpdateOrganizationRequest): Promise<Organization> {
        return apiClient.put<Organization>(`/api/v1/orgs/${orgId}`, data);
    },

    /**
     * Deactivate an organization (super admin only)
     */
    async deleteOrganization(orgId: number): Promise<void> {
        return apiClient.delete(`/api/v1/admin/organizations/${orgId}`);
    },

    /**
     * Get organization members
     */
    async getOrganizationMembers(orgId: number): Promise<OrganizationMember[]> {
        return apiClient.get<OrganizationMember[]>(`/api/v1/orgs/${orgId}/members`);
    },

    /**
     * Change user role in organization
     */
    async changeUserRole(orgId: number, userId: number, role: string): Promise<void> {
        return apiClient.put(`/api/v1/orgs/${orgId}/members/${userId}/role`, { role });
    },
};