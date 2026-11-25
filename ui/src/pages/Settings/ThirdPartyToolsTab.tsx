import React, { useState, useEffect, useRef } from 'react';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import { Badge, Alert } from '../../components/UIPrimitives';
import { isSafeExternalUrl } from '../Reviews/reviewDetailHelpers';

export interface Tool {
	id: number;
	name: string;
	description: string;
	lambda_arn: string;
	multiplier: number;
	use_case: string;
	category?: string;
	docs_url?: string;
	enabled: boolean;
}

interface ListToolsResponse {
	tools: Tool[];
}

const DEFAULT_TOOL_CREDITS_LIMIT = 50000;

const formatCompactNumber = (num: number): string => {
	return num.toLocaleString(undefined, { notation: 'compact', maximumFractionDigits: 1 });
};

interface ToolInfoPopoverProps {
	tool: Tool;
	openUpward: boolean;
	isHovered: boolean;
	isPinned: boolean;
	onHoverStart: () => void;
	onHoverEnd: () => void;
	onTogglePin: () => void;
}

const ToolInfoPopover: React.FC<ToolInfoPopoverProps> = ({
	tool,
	openUpward,
	isHovered,
	isPinned,
	onHoverStart,
	onHoverEnd,
	onTogglePin,
}) => {
	const isOpen = isHovered || isPinned;
	const hasSafeDocsUrl = isSafeExternalUrl(tool.docs_url);
	const popoverId = `tool-info-popover-${tool.id}`;

	return (
		<div
			className="relative inline-flex items-center"
			data-tool-info-root="true"
			onMouseEnter={onHoverStart}
			onMouseLeave={onHoverEnd}
		>
			<button
				type="button"
				aria-label={`Inspect ${tool.name} details`}
				aria-expanded={isOpen}
				aria-controls={popoverId}
				onClick={(e) => {
					e.stopPropagation();
					onTogglePin();
				}}
				onFocus={onHoverStart}
				onBlur={(e) => {
					const related = e.relatedTarget as Node | null;
					if (!e.currentTarget.parentElement?.contains(related)) {
						onHoverEnd();
					}
				}}
				className={`inline-flex h-5 w-5 items-center justify-center rounded-full border text-[11px] font-semibold transition-colors cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 ${
					isOpen
						? 'bg-violet-500/20 text-violet-200 border-violet-400/60'
						: 'bg-slate-800/70 text-slate-400 border-slate-700/80 hover:text-white hover:border-slate-500 group-hover/row:border-slate-500 group-hover/row:text-slate-200'
				}`}
				title={`About ${tool.name}`}
			>
				i
			</button>

			{isOpen && (
				<div
					id={popoverId}
					role="dialog"
					aria-label={`${tool.name} information`}
					className={`absolute left-0 z-30 w-80 sm:w-96 rounded-xl border border-slate-700 bg-slate-900/95 p-4 shadow-2xl backdrop-blur-md text-left ${
						openUpward ? 'bottom-full mb-2' : 'top-full mt-2'
					}`}
				>
					<div className="flex flex-wrap items-center gap-2">
						<span className="font-mono text-sm font-semibold text-white">
							{tool.name}
						</span>
						{tool.category && (
							<span className="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 text-[11px] font-medium text-slate-300">
								{tool.category}
							</span>
						)}
					</div>
					<p className="mt-0.5 text-[11px] text-slate-400">
						{tool.use_case || 'General Analysis'}
					</p>

					<p className="mt-2.5 text-xs leading-relaxed text-slate-300">
						{tool.description || 'Static analysis tool configured in catalog.'}
					</p>

					{hasSafeDocsUrl && (
						<div className="mt-3 flex items-center justify-end border-t border-slate-800 pt-2.5 text-[11px]">
							<a
								href={tool.docs_url}
								target="_blank"
								rel="noopener noreferrer"
								className="inline-flex items-center gap-1.5 font-medium text-violet-400 hover:text-violet-300 hover:underline"
							>
								<svg className="w-3.5 h-3.5 shrink-0" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
								</svg>
								<span>Official GitHub repository</span>
								<svg className="w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
									<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
								</svg>
							</a>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

const ThirdPartyToolsTab: React.FC = () => {
	const { currentOrg, canManageCurrentOrg } = useOrgContext();
	const isOwner = Boolean(canManageCurrentOrg);

	const [tools, setTools] = useState<Tool[]>([]);
	const [loading, setLoading] = useState(true);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [success, setSuccess] = useState<string | null>(null);
	const [localEnabled, setLocalEnabled] = useState<Record<number, boolean>>({});
	const [creditUsage, setCreditUsage] = useState<{ used: number; limit: number } | null>(null);

	const [sortField, setSortField] = useState<'name' | 'use_case' | 'category' | 'multiplier' | 'enabled'>('name');
	const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
	const [currentPage, setCurrentPage] = useState(1);
	const [hoveredInfoId, setHoveredInfoId] = useState<number | null>(null);
	const [pinnedInfoId, setPinnedInfoId] = useState<number | null>(null);
	const closeHoverTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const pageSize = 10;

	const clearHoverTimer = () => {
		if (closeHoverTimerRef.current) {
			clearTimeout(closeHoverTimerRef.current);
			closeHoverTimerRef.current = null;
		}
	};

	const openInfoOnHover = (toolId: number) => {
		clearHoverTimer();
		setHoveredInfoId(toolId);
	};

	const closeInfoOnLeave = (toolId: number) => {
		clearHoverTimer();
		closeHoverTimerRef.current = setTimeout(() => {
			setHoveredInfoId(prev => (prev === toolId ? null : prev));
		}, 150);
	};

	useEffect(() => {
		const handleDocMouseDown = (event: MouseEvent) => {
			const target = event.target as HTMLElement | null;
			if (target && target.closest('[data-tool-info-root="true"]')) {
				return;
			}
			setPinnedInfoId(null);
			setHoveredInfoId(null);
		};
		const handleKeyDown = (event: KeyboardEvent) => {
			if (event.key === 'Escape') {
				setPinnedInfoId(null);
				setHoveredInfoId(null);
			}
		};
		document.addEventListener('mousedown', handleDocMouseDown);
		document.addEventListener('keydown', handleKeyDown);
		return () => {
			clearHoverTimer();
			document.removeEventListener('mousedown', handleDocMouseDown);
			document.removeEventListener('keydown', handleKeyDown);
		};
	}, []);

	const handleSort = (field: 'name' | 'use_case' | 'category' | 'multiplier' | 'enabled') => {
		if (sortField === field) {
			setSortDirection(prev => prev === 'asc' ? 'desc' : 'asc');
		} else {
			setSortField(field);
			setSortDirection('asc');
		}
		setCurrentPage(1); // Reset to first page on sort change
	};

	const renderSortIcon = (field: 'name' | 'use_case' | 'category' | 'multiplier' | 'enabled') => {
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
		} else if (sortField === 'category') {
			valA = (a.category || '').toLowerCase();
			valB = (b.category || '').toLowerCase();
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
			const [toolsRes, usageRes] = await Promise.all([
				apiClient.get<ListToolsResponse>(`/orgs/${currentOrg.id}/tools`),
				apiClient.get<any>('/billing/usage/summary', { headers: { 'X-Org-Context': String(currentOrg.id) } }).catch((): null => null),
			]);
			const fetchedTools = toolsRes.tools || [];
			setTools(fetchedTools);

			const initialMap: Record<number, boolean> = {};
			fetchedTools.forEach(t => {
				initialMap[t.id] = t.enabled;
			});
			setLocalEnabled(initialMap);
			if (usageRes && typeof usageRes.tool_credits_used_month === 'number') {
				setCreditUsage({
					used: usageRes.tool_credits_used_month,
					limit: typeof usageRes.tool_credits_limit_month === 'number' ? usageRes.tool_credits_limit_month : DEFAULT_TOOL_CREDITS_LIMIT,
				});
			} else {
				setCreditUsage({
					used: 0,
					limit: DEFAULT_TOOL_CREDITS_LIMIT,
				});
			}
			setCurrentPage(1); // Reset page on org change
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
	const totalCreditPool = creditUsage?.limit ?? DEFAULT_TOOL_CREDITS_LIMIT;
	const creditsUsed = creditUsage?.used ?? 0;
	const remainingCredits = Math.max(0, totalCreditPool - creditsUsed);
	const estimatedReviews = totalMultiplier > 0 ? Math.floor(remainingCredits / totalMultiplier) : 0;
	const hasChanges = tools.some(t => localEnabled[t.id] !== t.enabled);
	const remainingLabel = formatCompactNumber(remainingCredits);
	const exampleTool = tools.find(t => Math.abs(t.multiplier - 1.0) < 0.01)
		|| (tools.length > 0 ? tools.reduce((min, t) => t.multiplier < min.multiplier ? t : min, tools[0]) : null);
	const exampleToolLabel = exampleTool ? `${exampleTool.name} at ` : 'baseline ';
	const exampleToolMultiplier = exampleTool ? Number(exampleTool.multiplier) : 1.0;
	const exampleToolCapacity = exampleToolMultiplier > 0 ? Math.floor(remainingCredits / exampleToolMultiplier) : Math.floor(remainingCredits);

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
				<div className="flex flex-wrap items-center justify-between gap-2 mb-2">
					<h4 className="text-sm font-semibold text-white flex items-center">
						<svg className="w-4 h-4 mr-2 text-violet-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
						Credit Pool & Limits
					</h4>
					<span className="text-xs text-slate-400">
						<strong className="text-white font-medium">{remainingCredits.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}</strong> / {totalCreditPool.toLocaleString()} credits remaining
					</span>
				</div>
				<p className="text-xs text-slate-300 leading-relaxed mb-3">
					Each tool invocation deducts credits from your organization's monthly <strong>{totalCreditPool.toLocaleString()} credit budget</strong>. 
					The credit cost of a review is equal to the sum of the multipliers of all enabled tools (<strong>Total Multiplier: {totalMultiplier.toFixed(1)}×</strong>). 
					Based on your current configuration and credit balance ({creditsUsed.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} credits used this month), your remaining pool allows for up to{' '}
					<strong className="text-amber-400">
						{totalMultiplier > 0 
							? `~${estimatedReviews.toLocaleString()} overall reviews with this setting` 
							: `~${exampleToolCapacity.toLocaleString()} tool runs (e.g. ${exampleToolLabel}${exampleToolMultiplier.toFixed(1)}×)`}
					</strong>{' '}
					before exhausting the budget.
				</p>
				<div className="w-full bg-slate-900 rounded-full h-1.5 overflow-hidden">
					<div
						className="bg-violet-500 h-1.5 rounded-full transition-all duration-300"
						style={{ width: `${Math.min(100, Math.max(0, (creditsUsed / totalCreditPool) * 100))}%` }}
					/>
				</div>
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
				<div className="flex flex-col justify-between space-y-1.5">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Enabled Tools</span>
					<div className="flex items-baseline space-x-1.5">
						<span className="text-3xl font-bold text-white transition-all duration-300">
							{enabledToolsCount}
						</span>
						<span className="text-sm font-medium text-slate-400">/ {tools.length}</span>
					</div>
					<span className="text-xs text-slate-400 block">
						{enabledToolsCount === 0 ? 'No tools selected' : `${enabledToolsCount} of ${tools.length} active`}
					</span>
				</div>
				<div className="flex flex-col justify-between space-y-1.5 border-t md:border-t-0 md:border-l border-slate-700/60 pt-3 md:pt-0 md:pl-5">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Total Multiplier</span>
					<div className="flex items-baseline space-x-1">
						<span className="text-3xl font-bold text-violet-400 transition-all duration-300">
							{totalMultiplier.toFixed(1)}
						</span>
						<span className="text-lg font-medium text-violet-400">×</span>
					</div>
					<span className="text-xs text-slate-400 block">
						{totalMultiplier > 0 ? `${totalMultiplier.toFixed(1)} credits / review` : '0 credits deducted / review'}
					</span>
				</div>
				<div className="flex flex-col justify-between space-y-1.5 border-t md:border-t-0 md:border-l border-slate-700/60 pt-3 md:pt-0 md:pl-5">
					<span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Estimated Reviews</span>
					{totalMultiplier > 0 ? (
						<div>
							<div className="flex items-baseline space-x-2">
								<span className="text-3xl font-bold text-amber-400 transition-all duration-300">
									~{estimatedReviews.toLocaleString()}
								</span>
								<span className="text-sm font-medium text-slate-400">reviews</span>
							</div>
							<span className="text-xs text-slate-400 block mt-0.5">
								Overall reviews with this setting ({totalMultiplier.toFixed(1)}× cost · {remainingLabel} remaining)
							</span>
						</div>
					) : (
						<div>
							<div className="flex items-baseline space-x-2">
								<span className="text-3xl font-bold text-amber-400 transition-all duration-300">
									~{exampleToolCapacity.toLocaleString()}
								</span>
								<span className="text-sm font-medium text-slate-400">tool runs</span>
							</div>
							<span className="text-xs text-slate-400 block mt-0.5">
								{exampleTool
									? `e.g. ~${exampleToolCapacity.toLocaleString()} ${exampleTool.name} tool runs (${exampleToolMultiplier.toFixed(1)}× cost · ${remainingLabel} remaining)`
									: `~${exampleToolCapacity.toLocaleString()} 1.0× tool runs (${remainingLabel} remaining)`}
							</span>
						</div>
					)}
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
					<p className="text-slate-300 mb-1 font-medium">No tools registered</p>
					<p className="text-sm text-slate-400">Tools are not available at the moment.</p>
				</div>
			) : (
				<div className="overflow-visible border border-slate-700 rounded-xl bg-slate-900/40">
					<table className="w-full text-left border-collapse">
						<thead>
							<tr className="border-b border-slate-700 bg-slate-800/50 select-none">
								<th 
									onClick={() => handleSort('name')}
									className="first:rounded-tl-xl p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group"
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
									onClick={() => handleSort('category')}
									className="p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group"
								>
									<div className="flex items-center">
										Category {renderSortIcon('category')}
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
									className="last:rounded-tr-xl p-4 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors duration-200 group text-right"
								>
									<div className="flex items-center justify-end">
										Status {renderSortIcon('enabled')}
									</div>
								</th>
							</tr>
						</thead>
						<tbody className="divide-y divide-slate-800">
							{paginatedTools.map((tool, index) => {
								const isInfoOpen = hoveredInfoId === tool.id || pinnedInfoId === tool.id;
								const openUpward = paginatedTools.length > 3 && index >= paginatedTools.length - 3;
								return (
									<tr
										key={tool.id}
										className={`group/row transition-colors duration-150 ${
											isInfoOpen
												? 'bg-slate-800/55'
												: 'hover:bg-slate-800/50'
										}`}
									>
										<td className="p-4">
											<div className="flex items-center gap-2">
												<span className="font-medium text-white text-sm group-hover/row:text-violet-100 transition-colors">
													{tool.name}
												</span>
												<ToolInfoPopover
													tool={tool}
													openUpward={openUpward}
													isHovered={hoveredInfoId === tool.id}
													isPinned={pinnedInfoId === tool.id}
													onHoverStart={() => openInfoOnHover(tool.id)}
													onHoverEnd={() => closeInfoOnLeave(tool.id)}
													onTogglePin={() => setPinnedInfoId(prev => (prev === tool.id ? null : tool.id))}
												/>
											</div>
										</td>
										<td className="p-4 text-sm text-slate-300">
											{tool.use_case || 'General Analysis'}
										</td>
										<td className="p-4 text-sm text-slate-300">
											{tool.category ? (
												<span className="inline-flex items-center rounded-full border border-slate-700 bg-slate-800/80 px-2.5 py-0.5 text-xs font-medium text-slate-300">
													{tool.category}
												</span>
											) : (
												<span className="text-slate-500">—</span>
											)}
										</td>
										<td className="p-4 text-sm font-semibold text-slate-300 tabular-nums">
											{tool.multiplier.toFixed(1)}×
										</td>
										<td className="p-4 text-right">
											{isOwner ? (
												<div className="inline-flex items-center space-x-2">
													<button
														type="button"
														aria-label={`Toggle ${tool.name}`}
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
						<div className="flex items-center justify-between rounded-b-xl border-t border-slate-700 bg-slate-900/60 px-4 py-3 sm:px-6">
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
