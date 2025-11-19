import React, { useEffect, useState } from 'react';

interface AuthedUser {
	email: string;
	name: string;
	jwt: string;
	avatarUrl?: string;
}

const Cloud: React.FC = () => {
	const [user, setUser] = useState<AuthedUser | null>(null);
	const [isClient, setIsClient] = useState(false);

	useEffect(() => {
		setIsClient(true);
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

	return (
		<div className="min-h-screen bg-gray-900 flex items-center justify-center">
			<div className="max-w-2xl w-full bg-gray-800 p-8 rounded-lg shadow-lg text-white">
				<h1 className="text-2xl font-bold mb-4 text-center">Cloud Login</h1>
				<p className="text-sm text-gray-300 mb-6 text-center">
					Sign in via Hexmos SSO and see the returned credentials
					below.
				</p>
				<div className="flex justify-center mb-8">
					<button
						onClick={handleLoginClick}
						className="inline-block bg-blue-600 hover:bg-blue-700 transition text-white font-semibold py-2 px-6 rounded"
					>
						Sign in with Hexmos
					</button>
				</div>
				<div className="overflow-x-auto">
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
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
};

export default Cloud;

