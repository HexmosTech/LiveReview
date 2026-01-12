import { Dispatch, AnyAction } from '@reduxjs/toolkit';
import toast from 'react-hot-toast';
import { LoginResponse } from '../api/auth';

/**
 * Get the stored redirect URL from sessionStorage after successful login
 * @returns The redirect URL or null if not set
 */
export const getRedirectAfterLogin = (): string | null => {
	const redirectUrl = sessionStorage.getItem('redirectAfterLogin');
	return redirectUrl;
};

/**
 * Clear the stored redirect URL from sessionStorage
 */
export const clearRedirectAfterLogin = (): void => {
	sessionStorage.removeItem('redirectAfterLogin');
};

/**
 * Common post-login handler that processes a successful login response
 * by dispatching it to the Redux store. The store will handle setting
 * tokens in localStorage and updating auth state.
 * 
 * @param loginResponse - The response from the login API
 * @param dispatch - Redux dispatch function
 */
export const handleLoginSuccess = (
	loginResponse: LoginResponse,
	dispatch: Dispatch<AnyAction>
) => {
	// Import the login action fulfillment logic
	// The Redux reducer will handle setting tokens in localStorage and state
	dispatch({
		type: 'auth/login/fulfilled',
		payload: loginResponse,
	});
	
	toast.success('Login successful!');
};

/**
 * Handle login errors consistently
 * @param error - The error object
 */
export const handleLoginError = (error: unknown) => {
	const errorMessage = (error as Error).message || 'An unknown error occurred.';
	toast.error(`Login failed: ${errorMessage}`);
	console.error('Login error:', error);
};
