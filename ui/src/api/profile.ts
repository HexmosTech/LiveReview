import apiClient from './apiClient';

export interface UserProfile {
    id: number;
    email: string;
    first_name: string | null;
    last_name: string | null;
    is_active: boolean;
    created_at: string;
    updated_at: string;
    last_login_at: string | null;
    plan_type: string | null;
    license_expires_at: string | null;
}

export interface UpdateProfileRequest {
    first_name?: string;
    last_name?: string;
    email?: string;
}

export interface ChangePasswordRequest {
    current_password: string;
    new_password: string;
}

/**
 * Update the current user's own name/email. Session-only on the backend.
 */
export const updateProfile = async (req: UpdateProfileRequest): Promise<UserProfile> => {
    return apiClient.put<UserProfile>('/users/profile', req);
};

/**
 * Change the current user's own password. Session-only on the backend.
 */
export const changePassword = async (req: ChangePasswordRequest): Promise<{ message: string }> => {
    return apiClient.put<{ message: string }>('/users/password', req);
};

/**
 * Save the browser's time zone (IANA name) so PR tables show this user's local time.
 */
export const saveTimeZone = async (timeZone: string): Promise<void> => {
    await apiClient.put('/users/time-zone', { time_zone: timeZone });
};
