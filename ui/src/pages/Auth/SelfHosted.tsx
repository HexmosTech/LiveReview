import React, { useState } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { useNavigate } from 'react-router-dom';
import { login } from '../../store/Auth/reducer';
import { handleLoginError } from '../../utils/authHelpers';
import toast from 'react-hot-toast';

const SelfHosted: React.FC = () => {
	const dispatch = useAppDispatch();
	const navigate = useNavigate();
	const [email, setEmail] = useState('');
	const [password, setPassword] = useState('');
	const { isLoading, error } = useAppSelector((state) => state.Auth);

	// Debug logging for login component
	console.log('🚨🚨🚨 === LOGIN COMPONENT DEBUG === 🚨🚨🚨');
	console.log('🔍 Login component rendered at:', new Date().toISOString());
	console.log('🔍 Current URL:', window.location.href);
	console.log('🔍 Is localhost?:', window.location.hostname === 'localhost');
	console.log('🔍 LIVEREVIEW_CONFIG at render:', JSON.stringify(window.LIVEREVIEW_CONFIG, null, 2));

	// Debug logging for login component
	console.log('🚨🚨🚨 === LOGIN COMPONENT DEBUG === 🚨🚨🚨');
	console.log('🔍 Login component rendered at:', new Date().toISOString());
	console.log('🔍 Current URL:', window.location.href);
	console.log('🔍 Is localhost?:', window.location.hostname === 'localhost');
	console.log('🔍 LIVEREVIEW_CONFIG at render:', JSON.stringify(window.LIVEREVIEW_CONFIG, null, 2));

	// Alert cloud/self-hosted mode based on root .env flag
	const isCloud = (process.env.LIVEREVIEW_IS_CLOUD || '').toString();
	if (typeof window !== 'undefined') {
		// Only alert once per page load
		const marker = '__LIVEREVIEW_IS_CLOUD_ALERTED__';
		if (!(window as any)[marker]) {
			(window as any)[marker] = true;
			 console.log(`LIVEREVIEW_IS_CLOUD = ${isCloud}`);
		}
	}

	// Check if we're in production mode and accessing via localhost
	const isProductionModeOnLocalhost = () => {
		// Check if we're on localhost but in production mode (reverse proxy enabled)
		const isLocalhost = window.location.hostname === 'localhost';
		// We can detect production mode by checking if LIVEREVIEW_REVERSE_PROXY would be true
		// In production mode, the app expects to be accessed via a reverse proxy, not directly
		return isLocalhost && window.location.port !== '8081'; // 8081 is demo mode port
	};

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		if (!email || !password) {
			toast.error('Please enter both email and password.');
			return;
		}

		try {
			await dispatch(login({ email, password })).unwrap();
			toast.success('Login successful!');
			navigate('/dashboard');
		} catch (err) {
			handleLoginError(err);
		}
	};

	return (
		<div className="min-h-screen bg-gray-900 flex items-center justify-center">
			<div className="max-w-md w-full bg-gray-800 p-8 rounded-lg shadow-lg">
				<div className="text-center mb-8">
					<img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-16 w-auto mx-auto" />
				</div>
				<div className="text-center">
					<h2 className="mt-6 text-3xl font-extrabold text-white">Sign In</h2>
					<p className="mt-2 text-sm text-gray-400">Sign in to your account</p>
				</div>
        
				{isProductionModeOnLocalhost() && (
					<div className="bg-yellow-900/20 border border-yellow-600 text-yellow-300 px-4 py-3 rounded-lg text-sm">
						<div className="flex">
							<div className="flex-shrink-0">
								<svg className="h-5 w-5 text-yellow-400" viewBox="0 0 20 20" fill="currentColor">
									<path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
								</svg>
							</div>
							<div className="ml-3">
								<h3 className="text-sm font-medium text-yellow-300">Production Mode Notice</h3>
								<p className="mt-1 text-sm text-yellow-200">
									You're accessing LiveReview via localhost, but it's configured for production mode with reverse proxy. 
									This may not work correctly. Please access via your configured domain or switch to demo mode.
								</p>
							</div>
						</div>
					</div>
				)}
        
				<form className="mt-8 space-y-6" onSubmit={handleSubmit}>
					<div className="space-y-4">
						<div>
							<label htmlFor="email-address" className="block text-sm font-medium text-gray-300 mb-2">
								Email address
							</label>
							<input
								id="email-address"
								name="email"
								type="email"
								autoComplete="email"
								required
								className="appearance-none relative block w-full px-4 py-3 border border-gray-600 bg-gray-800 text-white placeholder-gray-400 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors sm:text-sm"
								placeholder="Enter your email address"
								value={email}
								onChange={(e) => setEmail(e.target.value)}
							/>
						</div>
						<div>
							<label htmlFor="password" className="block text-sm font-medium text-gray-300 mb-2">
								Password
							</label>
							<input
								id="password"
								name="password"
								type="password"
								autoComplete="current-password"
								required
								className="appearance-none relative block w-full px-4 py-3 border border-gray-600 bg-gray-800 text-white placeholder-gray-400 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors sm:text-sm"
								placeholder="Enter your password"
								value={password}
								onChange={(e) => setPassword(e.target.value)}
							/>
						</div>
					</div>

					{error && <div className="bg-red-900/20 border border-red-600 text-red-300 px-4 py-3 rounded-lg text-sm">{error}</div>}

					<div className="pt-2">
						<button
							type="submit"
							disabled={isLoading}
							className="group relative w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-lg text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:bg-indigo-400 transition-colors"
						>
							{isLoading ? 'Signing in...' : 'Sign in'}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

export default SelfHosted;

