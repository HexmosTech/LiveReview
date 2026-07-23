import React, { useState, useEffect } from 'react';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import { Badge, Alert } from '../../components/UIPrimitives';

export interface Tool {
	id: number;
	name: string;
	description: string;
	lambda_arn: string;
	multiplier: number;
	use_case: string;
	enabled: boolean;
}

interface ListToolsResponse {
	tools: Tool[];
}

const ThirdPartyToolsTab: React.FC = () => {
	const { currentOrg } = useOrgContext();
	const isOwner = currentOrg?.role === 'owner';

	const [tools, setTools] = useState<Tool[]>([]);
	const [localEnabled, setLocalEnabled] = useState<Record<number, boolean>>({});
	const [creditUsage, setCreditUsage] = useState<any>(null);
	const [loading, setLoading] = useState(true);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [success, setSuccess] = useState<string | null>(null);

	const [sortField, setSortField] = useState<'name' | 'use_case' | 'multiplier' | 'enabled'>('name');
	const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
	const [currentPage, setCurrentPage] = useState(1);
	const pageSize = 10;

	const handleSort = (field: 'name' | 'use_case' | 'multiplier' | 'enabled') => {
		if (sortField === field) {
			setSortDirection(prev => prev === 'asc' ? 'desc' : 'asc');
		} else {
			setSortField(field);
			setSortDirection('asc');
		}
		setCurrentPage(1); // Reset to first page on sort change
	};

	const renderSortIcon = (field: 'name' | 'use_case' | 'multiplier' | 'enabled') => {
		if (sortField !== field) {
			return (
				<svg className="w-3.5 h-3.5 ml-1 opacity-40 group-hover:opacity-80 transition-opacity duration-200 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M8 9l4-4 4 4m0 6l-4 4-4-4" />
				</svg>
			);
		}
		if (sortDirection === 'asc') {
			return (
				<svg className="w-3.5 h-3.5 ml-1 text-violet-400 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 15l7-7 7 7" />
				</svg>
			);
		}
		return (
			<svg className="w-3.5 h-3.5 ml-1 text-violet-400 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 9l-7 7-7-7" />
			</svg>
		);
	};

	const sortedTools = [...tools].sort((a, b) => {
		let valA: any;
		let valB: any;

		if (sortField === 'name') {
			valA = a.name.toLowerCase();
			valB = b.name.toLowerCase();
		} else if (sortField === 'use_case') {
			valA = (a.use_case || '').toLowerCase();
			valB = (b.use_case || '').toLowerCase();
		} else if (sortField === 'multiplier') {
			valA = a.multiplier;
			valB = b.multiplier;
		} else if (sortField === 'enabled') {
			valA = localEnabled[a.id] ? 1 : 0;
			valB = localEnabled[b.id] ? 1 : 0;
		}

		if (valA < valB) return sortDirection === 'asc' ? -1 : 1;
		if (valA > valB) return sortDirection === 'asc' ? 1 : -1;
		return 0;
	});

	useEffect(() => {
		loadTools();
	}, [currentOrg?.id]);

	const loadTools = async () => {
		if (!currentOrg) return;
		setLoading(true);
		setError(null);
		try {
			const response = await apiClient.get<ListToolsResponse>(`/orgs/${currentOrg.id}/tools`);
			const fetchedTools = response.tools || [];
			setTools(fetchedTools);

			const initialMap: Record<number, boolean> = {};
			fetchedTools.forEach(t => {
				initialMap[t.id] = t.enabled;
			});
			setLocalEnabled(initialMap);
			setCurrentPage(1); // Reset page on org change

			const creditResp = await apiClient.get<any>(`/orgs/${currentOrg.id}/tools/credits`);
			setCreditUsage(creditResp);
		} catch (err: any) {
			setError(err.message || 'Failed to load tools');
		} finally {
			setLoading(false);
		}
	};

	const handleToggleTool = (tool: Tool) => {
		setLocalEnabled(prev => ({
			...prev,
			[tool.id]: !prev[tool.id]
		}));
	};

	const handleSaveChanges = async () => {
		if (!currentOrg || !isOwner) return;

		setSaving(true);
		setError(null);
		setSuccess(null);

		const changedTools = tools.filter(t => localEnabled[t.id] !== t.enabled);

		try {
			await Promise.all(
				changedTools.map(t =>
					apiClient.put(`/orgs/${currentOrg.id}/tools/${t.id}`, {
						enabled: localEnabled[t.id]
					})
				)
			);

			setTools(prev => prev.map(t => ({
				...t,
				enabled: localEnabled[t.id]
			})));

			setSuccess('Successfully saved tool configurations');
			setTimeout(() => setSuccess(null), 3000);
		} catch (err: any) {
			setError(err.message || 'Failed to save tool configurations. Please reload.');
		} finally {
			setSaving(false);
		}
	};

	if (!currentOrg) {
		return (
			<div className="p-4">
				<Alert variant="warning">
					Please select an organization to view tools.
				</Alert>
			</div>
		);
	}

	const enabledToolsCount = tools.filter(t => localEnabled[t.id]).length;
	const rawTotalMultiplier = tools.reduce((acc, t) => acc + (localEnabled[t.id] ? Number(t.multiplier) : 0), 0);
	const totalMultiplier = Number(rawTotalMultiplier.toFixed(2));
	
	const totalCreditPool = creditUsage?.credits_limit_month || 50000;
	const usedCredits = creditUsage?.credits_used_month || 0;
	const remainingCredits = Math.max(0, totalCreditPool - usedCredits);
	const estimatedReviews = totalMultiplier > 0 ? Math.floor(remainingCredits / totalMultiplier) : 0;
	const hasChanges = tools.some(t => localEnabled[t.id] !== t.enabled);

	const totalPages = Math.ceil(sortedTools.length / pageSize);
	const activePage = Math.min(currentPage, Math.max(1, totalPages));
	const paginatedTools = sortedTools.slice((activePage - 1) * pageSize, activePage * pageSize);

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-lg font-medium text-white mb-2">Third-Party Static Analysis Tools</h3>
				<p className="text-sm text-slate-300">
					Enable external linters and security scanners to run concurrently as parallel Lambda functions alongside your AI reviews.
				</p>
			</div>

			{/* Cost Explanation Card */}
			<div className="bg-slate-800/40 border border-slate-700/60 rounded-xl p-5 backdrop-blur-md shadow-sm">
				<h4 className="text-sm font-semibold text-white mb-2 flex items-center">
					<svg className="w-4 h-4 mr-2 text-violet-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					Credit Pool & Limits
				</h4>
				<p className="text-xs text-slate-300 leading-relaxed">
					Each tool invocation deducts credits from your organization's budget of <strong>{totalCreditPool.toLocaleString()} credits/month</strong>. 
					The credit cost of a review is equal to the sum of the multipliers of all enabled tools (<strong>Total Multiplier</strong>). 
					Based on your current configuration, your pool allows for up to{' '}
					<strong className="text-amber-400">
						{totalMultiplier > 0 ? `${estimatedReviews.toLocaleString()} reviews` : 'unlimited reviews'}
					</strong>{' '}
					before exhausting the budget.
				</p>
			</div>

			{error && (
				<Alert variant="error" onClose={() => setError(null)}>
					{error}
				</Alert>
			)}

			{success && (
				<Alert variant="success" onClose={() => setSuccess(null)}>
					{success}
				</Alert>
			)}

			{/* Cost Summary Bar */}
			<div className="grid grid-cols-1 md:grid-cols-3 gap-4 bg-slate-800/80 border border-slate-700/80 rounded-xl p-5 backdrop-blur-md shadow-lg">
				<div className="flex flex-col justify-between space-y-1">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Enabled Tools</span>
					<span className="text-3xl font-bold text-white transition-all duration-300">
						{enabledToolsCount} <span className="text-sm font-medium text-slate-400">/ {tools.length}</span>
					</span>
				</div>
				<div className="flex flex-col justify-between space-y-1 border-t md:border-t-0 md:border-l border-slate-700/60 pt-3 md:pt-0 md:pl-5">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Credits Used Per Review</span>
					<span className="text-3xl font-bold text-violet-400 transition-all duration-300">
						{totalMultiplier.toFixed(1)}<span className="text-lg font-medium">×</span>
					</span>
				</div>
				<div className="flex flex-col justify-between space-y-1 border-t md:border-t-0 md:border-l border-slate-700/60 pt-3 md:pt-0 md:pl-5">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Reviews Remaining / Month</span>
					<span className="text-3xl font-bold text-amber-400 transition-all duration-300">
						{totalMultiplier > 0 ? estimatedReviews.toLocaleString() : '—'} <span className="text-sm font-medium text-slate-400">reviews ({Math.round(totalCreditPool/1000)}k pool)</span>
					</span>
				</div>
			</div>

			{/* Action Toolbar */}
			{tools.length > 0 && !loading && (
				<div className="flex justify-between items-center bg-slate-800/40 border border-slate-700/50 p-4 rounded-xl">
					<span className="text-sm text-slate-300">
						{hasChanges ? (
							<span className="text-amber-400 font-medium flex items-center">
								<span className="w-2 h-2 rounded-full bg-amber-400 animate-pulse mr-2"></span>
								Unsaved changes
							</span>
						) : (
							<span className="text-emerald-400 font-medium flex items-center">
								<span className="w-2 h-2 rounded-full bg-emerald-400 mr-2"></span>
								All configurations saved
							</span>
						)}
					</span>
					{isOwner && (
						<button
							onClick={handleSaveChanges}
							disabled={loading || !hasChanges || saving}
							className={`px-5 py-2.5 rounded-lg text-sm font-semibold shadow-md transition-all duration-200 flex items-center space-x-2 ${
								hasChanges && !saving && !loading
									? 'bg-violet-600 hover:bg-violet-500 text-white cursor-pointer hover:shadow-violet-600/20 hover:scale-[1.02]'
									: 'bg-slate-800 text-slate-500 cursor-not-allowed border border-slate-700/50'
							}`}
						>
							{saving ? (
								<>
									<div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
									<span>Saving Changes...</span>
								</>
							) : (
								<span>Save Changes</span>
							)}
						</button>
					)}
				</div>
			)}

			{loading ? (
				<div className="flex items-center justify-center py-12">
					<div className="text-center">
						<div className="w-8 h-8 border-2 border-violet-500 border-t-transparent rounded-full animate-spin mx-auto mb-3"></div>
						<p className="text-slate-400 text-sm">Loading available tools...</p>
					</div>
				</div>
			) : tools.length === 0 ? (
				<div className="text-center py-12 bg-slate-800 rounded-xl border border-slate-700">
					<p className="text-slate-300 mb-1 font-medium">No tools available</p>
					<p className="text-sm text-slate-400">Use the admin register-tools CLI helper to populate the catalog.</p>
				</div>
			) : (
				<div className="overflow-hidden border border-slate-700 rounded-xl bg-slate-900/40">
					<table className="w-full text-left border-collapse">
						<thead>
							<tr className="border-b border-slate-700 bg-slate-800/50 select-none">
								<th 
									onClick={() => handleSort('name')}
									className="p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group"
								>
									<div className="flex items-center">
										Tool {renderSortIcon('name')}
									</div>
								</th>
								<th 
									onClick={() => handleSort('use_case')}
									className="p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group"
								>
									<div className="flex items-center">
										Use Case {renderSortIcon('use_case')}
									</div>
								</th>
								<th 
									onClick={() => handleSort('multiplier')}
									className="p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group"
								>
									<div className="flex items-center">
										Cost Multiplier {renderSortIcon('multiplier')}
									</div>
								</th>
								<th 
									onClick={() => handleSort('enabled')}
									className="p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group text-right"
								>
									<div className="flex items-center justify-end">
										Status {renderSortIcon('enabled')}
									</div>
								</th>
							</tr>
						</thead>
						<tbody className="divide-y divide-slate-800">
							{paginatedTools.map((tool) => {
								return (
									<tr key={tool.id} className="hover:bg-slate-800/30 transition-colors">
										<td className="p-4">
											<div className="font-medium text-white text-sm">{tool.name}</div>
											<div className="text-xs text-slate-400 mt-1 max-w-md">{tool.description}</div>
										</td>
										<td className="p-4 text-sm text-slate-300">
											{tool.use_case || 'General'}
										</td>
										<td className="p-4 text-sm font-semibold text-slate-300">
											{tool.multiplier.toFixed(1)}×
										</td>
										<td className="p-4 text-right">
											{isOwner ? (
												<div className="inline-flex items-center space-x-2">
													<button
														onClick={() => handleToggleTool(tool)}
														disabled={saving}
														className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-300 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:ring-offset-2 focus:ring-offset-slate-900 ${
															localEnabled[tool.id] ? 'bg-violet-600' : 'bg-slate-700'
														} ${saving ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
													>
														<span
															className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform duration-300 ${
																localEnabled[tool.id] ? 'translate-x-6' : 'translate-x-1'
															}`}
														/>
													</button>
												</div>
											) : (
												<Badge variant={localEnabled[tool.id] ? 'success' : 'default'}>
													{localEnabled[tool.id] ? 'Enabled' : 'Disabled'}
												</Badge>
											)}
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>

					{/* Pagination Footer */}
					{totalPages > 1 && (
						<div className="flex items-center justify-between border-t border-slate-700 bg-slate-900/60 px-4 py-3 sm:px-6">
							<div className="flex flex-1 justify-between sm:hidden">
								<button
									onClick={() => setCurrentPage(prev => Math.max(prev - 1, 1))}
									disabled={activePage === 1}
									className="relative inline-flex items-center rounded-md border border-slate-700 bg-slate-800 px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Previous
								</button>
								<button
									onClick={() => setCurrentPage(prev => Math.min(prev + 1, totalPages))}
									disabled={activePage === totalPages}
									className="relative ml-3 inline-flex items-center rounded-md border border-slate-700 bg-slate-800 px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-700 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Next
								</button>
							</div>
							<div className="hidden sm:flex sm:flex-1 sm:items-center sm:justify-between">
								<div>
									<p className="text-sm text-slate-400">
										Showing <span className="font-medium text-white">{((activePage - 1) * pageSize) + 1}</span> to{' '}
										<span className="font-medium text-white">
											{Math.min(activePage * pageSize, sortedTools.length)}
										</span>{' '}
										of <span className="font-medium text-white">{sortedTools.length}</span> tools
									</p>
								</div>
								<div>
									<nav className="isolate inline-flex -space-x-px rounded-md shadow-sm" aria-label="Pagination">
										<button
											onClick={() => setCurrentPage(prev => Math.max(prev - 1, 1))}
											disabled={activePage === 1}
											className="relative inline-flex items-center rounded-l-md px-2 py-2 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 focus:z-20 focus:outline-offset-0 disabled:opacity-30 disabled:cursor-not-allowed"
										>
											<span className="sr-only">Previous</span>
											<svg className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
												<path fillRule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clipRule="evenodd" />
											</svg>
										</button>
										
										{Array.from({ length: totalPages }, (_, i) => i + 1).map((page) => (
											<button
												key={page}
												onClick={() => setCurrentPage(page)}
												className={`relative inline-flex items-center px-4 py-2 text-sm font-semibold focus:z-20 ${
													page === activePage
														? 'z-10 bg-violet-600 text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-violet-600'
														: 'text-slate-300 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 focus:outline-offset-0'
												}`}
											>
												{page}
											</button>
										))}

										<button
											onClick={() => setCurrentPage(prev => Math.min(prev + 1, totalPages))}
											disabled={activePage === totalPages}
											className="relative inline-flex items-center rounded-r-md px-2 py-2 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 focus:z-20 focus:outline-offset-0 disabled:opacity-30 disabled:cursor-not-allowed"
										>
											<span className="sr-only">Next</span>
											<svg className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
												<path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
											</svg>
										</button>
									</nav>
								</div>
							</div>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

export default ThirdPartyToolsTab;
