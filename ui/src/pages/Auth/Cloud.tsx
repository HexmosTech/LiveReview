import React, { useEffect, useState } from 'react';
import { useAppDispatch } from '../../store/configureStore';
import { handleLoginSuccess, handleLoginError } from '../../utils/authHelpers';
import { LoginResponse } from '../../api/auth';

interface AuthedUser {
	email: string;
	name: string;
	jwt: string;
	avatarUrl?: string;
}

interface CloudProvisionResponse {
	status?: string;
	user_id?: number;
	org_id?: number;
	created_user?: boolean;
	created_org?: boolean;
	super_admin_assigned?: boolean;
	email?: string;
	error?: string; // capture error message from backend
	raw?: any; // full raw response for debugging
}

const Cloud: React.FC = () => {
	const dispatch = useAppDispatch();
	const [user, setUser] = useState<AuthedUser | null>(null);
	const [isClient, setIsClient] = useState(false);
	const [provisionResult, setProvisionResult] = useState<CloudProvisionResponse | null>(null);
	const [provisioning, setProvisioning] = useState(false);
	const [loggingIn, setLoggingIn] = useState(false);
	const [hasUrlParams, setHasUrlParams] = useState(false);

	useEffect(() => {
		setIsClient(true);
		// Check if we have URL params immediately to show loader
		const sp = new URLSearchParams(window.location.search);
		if (sp.get('data')) {
			setHasUrlParams(true);
		}
	}, []);

	// Parse ?data= payload on first mount (similar to access-livereview.tsx)
	useEffect(() => {
		if (!isClient) return;

		const sp = new URLSearchParams(window.location.search);
		const raw = sp.get('data');
		if (!raw) return;

		let parsed: any;
		try {
			const decoded = decodeURIComponent(raw);
			parsed = JSON.parse(decoded);
		} catch (e) {
			console.error('[CloudLogin] Failed to parse data param', e);
			return;
		}

		const jwt: string | undefined = parsed?.result?.jwt;
		const email: string | undefined =
			parsed?.result?.data?.email || parsed?.result?.data?.username;
		if (!jwt || !email || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) return;

		const first = parsed?.result?.data?.first_name?.trim() || '';
		const last = parsed?.result?.data?.last_name?.trim() || '';
		let name = (first + (last && last !== first ? ' ' + last : '')).trim();
		if (!name) {
			const username = parsed?.result?.data?.username;
			if (username && !username.includes('@')) name = username;
			else name = email.split('@')[0];
		}
		const avatarUrl: string | undefined = parsed?.result?.data?.profilePicUrl;

		const userObj: AuthedUser = { email, name, jwt, avatarUrl };
		setUser(userObj);

		try {
			window.history.replaceState(null, '', window.location.pathname);
		} catch {}
	}, [isClient]);

	const handleLoginClick = () => {
		try {
			const currentUrl = window.location.href;
			const hostEnv = (window as any).__ENV__?.VITE_HOST_ENV;
			let signinUrl = `/signin/auth/index.html?preselectProvider=authentik&app=livereview&appRedirectURI=${encodeURIComponent(
				currentUrl,
			)}`;
			if (!hostEnv || hostEnv !== 'onprem') {
				signinUrl = `https://hexmos.com/signin?app=livereview&appRedirectURI=${encodeURIComponent(
					currentUrl,
				)}`;
			}
			window.location.href = signinUrl;
		} catch (e) {
			console.error('[CloudLogin] Failed to initiate login', e);
		}
	};

	// After we have the external user + JWT, call ensure-cloud-user (idempotent)
	useEffect(() => {
		if (!user || provisioning || provisionResult || loggingIn) return;
		setProvisioning(true);
		(async () => {
			try {
				const url = `${window.location.origin}/api/v1/auth/ensure-cloud-user`;
				const payload = {
					email: user.email,
					first_name: user.name.split(' ')[0] || user.name,
					last_name: user.name.split(' ').slice(1).join(' ') || '',
				};
				const resp = await fetch(url, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						Authorization: `Bearer ${user.jwt}`,
					},
					body: JSON.stringify(payload),
				});
				let data: any = null;
				try { data = await resp.json(); } catch { data = { error: 'non-json response' }; }
				if (!resp.ok) {
					setProvisionResult({ error: data?.error || `HTTP ${resp.status}`, raw: data });
				} else {
					setProvisionResult({ ...data, raw: data });
					
					// If we got tokens and user info back, process as a login
					if (data.tokens && data.user && data.organizations) {
						setLoggingIn(true);
						const loginResponse: LoginResponse = {
							user: data.user,
							tokens: data.tokens,
							organizations: data.organizations,
						};
						handleLoginSuccess(loginResponse, dispatch);
						// Navigation will happen automatically via Redux state change
					}
				}
			} catch (err: any) {
				setProvisionResult({ error: err?.message || 'unknown error', raw: null });
				handleLoginError(err);
			} finally {
				setProvisioning(false);
			}
		})();
	}, [user, provisioning, provisionResult, loggingIn, dispatch]);

	return (
		<div className="min-h-screen bg-gray-900 flex items-center justify-center">
			<div className="max-w-md w-full bg-gray-800 p-8 rounded-lg shadow-lg">
				<div className="text-center mb-8">
					<img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-16 w-auto mx-auto" />
				</div>

				{hasUrlParams || loggingIn || provisioning ? (
					<div className="flex flex-col items-center justify-center space-y-4 py-8">
						<div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-500"></div>
						<p className="text-gray-400 text-sm">Logging you in...</p>
					</div>
				) : (
					<>
						<div className="text-center mb-8">
							<h2 className="text-lg font-medium text-gray-300">Sign in to <strong>LiveReview</strong> in 30 seconds:</h2>
						</div>
						<div className="flex justify-center">
							<button
								onClick={handleLoginClick}
								className="inline-flex items-center justify-center gap-3 py-4 px-8 border border-transparent text-base font-semibold rounded-lg text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-colors shadow-lg"
							>
								<img 
									src="assets/hexmos-logo.svg" 
									alt="Hexmos" 
									className="h-6 w-6"
								/>
								Sign in with Hexmos
							</button>
						</div>
					</>
				)}
				
				<div className="overflow-x-auto hidden">
					<table className="min-w-full text-left text-sm border border-gray-700 rounded-lg overflow-hidden">
						<thead className="bg-gray-700 text-gray-100">
							<tr>
								<th className="px-4 py-2 border-b border-gray-600">Field</th>
								<th className="px-4 py-2 border-b border-gray-600">Value</th>
							</tr>
						</thead>
						<tbody>
							<tr className="bg-gray-900/40">
								<td className="px-4 py-2 border-b border-gray-700">Status</td>
								<td className="px-4 py-2 border-b border-gray-700">
									{user ? 'Authenticated' : 'Not authenticated'}
								</td>
							</tr>
							<tr>
								<td className="px-4 py-2 border-b border-gray-700">Name</td>
								<td className="px-4 py-2 border-b border-gray-700">
									{user?.name || '-'}
								</td>
							</tr>
							<tr className="bg-gray-900/40">
								<td className="px-4 py-2 border-b border-gray-700">Email</td>
								<td className="px-4 py-2 border-b border-gray-700">
									{user?.email || '-'}
								</td>
							</tr>
							<tr>
								<td className="px-4 py-2 border-b border-gray-700">JWT</td>
								<td className="px-4 py-2 border-b border-gray-700 break-all max-w-xs">
									{user?.jwt || '-'}
								</td>
							</tr>
							<tr className="bg-gray-900/40">
								<td className="px-4 py-2">Avatar URL</td>
								<td className="px-4 py-2 break-all max-w-xs">
									{user?.avatarUrl || '-'}
								</td>
							</tr>
							<tr>
								<td className="px-4 py-2 border-t border-gray-700">Cloud Provision</td>
								<td className="px-4 py-2 border-t border-gray-700">
									{!user && 'Awaiting auth'}
									{user && provisioning && 'Provisioning...'}
									{user && !provisioning && provisionResult && (
										<div className="space-y-1">
											<div className="text-xs">
												Status: {provisionResult.status || (provisionResult.error ? 'error' : 'ok')}
											</div>
											{provisionResult.error && (
												<div className="text-xs text-red-400 break-all">Error: {provisionResult.error}</div>
											)}
											{provisionResult.user_id !== undefined && (
												<div className="text-xs">User ID: {provisionResult.user_id}</div>
											)}
											{provisionResult.org_id !== undefined && (
												<div className="text-xs">Org ID: {provisionResult.org_id}</div>
											)}
											{provisionResult.super_admin_assigned !== undefined && (
												<div className="text-xs">Super Admin Assigned: {String(provisionResult.super_admin_assigned)}</div>
											)}
											{provisionResult.created_user !== undefined && (
												<div className="text-xs">Created User: {String(provisionResult.created_user)}</div>
											)}
											{provisionResult.created_org !== undefined && (
												<div className="text-xs">Created Org: {String(provisionResult.created_org)}</div>
											)}
											{provisionResult.raw && (
												<div className="text-xs text-gray-400 break-all">
													Raw: {(() => { try { return JSON.stringify(provisionResult.raw); } catch { return 'unstringifiable'; } })()}
												</div>
											)}
										</div>
									)}
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
};

export default Cloud;

