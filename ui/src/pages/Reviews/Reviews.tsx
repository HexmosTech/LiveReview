import React, { useState, useEffect, useCallback } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Icons } from '../../components/UIPrimitives';
import { 
  getReviews, 
  formatRelativeTime, 
  getStatusColor, 
  getStatusText 
} from '../../api/reviews';
import { 
  Review, 
  ReviewsFilters, 
  ReviewStatus 
} from '../../types/reviews';

const Reviews: React.FC = () => {
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();
    
    // Helper function to extract author from PR/MR URL
    const extractAuthorFromUrl = (url: string): string | null => {
        try {
            const urlObj = new URL(url);
            const pathParts = urlObj.pathname.split('/').filter(Boolean);
            
            // GitHub: /owner/repo/pull/123 -> owner
            // GitLab: /owner/repo/-/merge_requests/123 -> owner  
            // Bitbucket: /workspace/repo/pull-requests/123 -> workspace
            if (pathParts.length >= 2) {
                return pathParts[0]; // First part is usually the owner/workspace
            }
        } catch (e) {
            console.warn('Failed to parse URL for author:', url);
        }
        return null;
    };
    
    const toTitleCase = (value: string): string => value.replace(/\b\w/g, char => char.toUpperCase());

    // Helper function to extract MR/PR info from URL
    const extractMRInfo = (url: string): string => {
        try {
            const urlObj = new URL(url);
            const pathParts = urlObj.pathname.split('/').filter(Boolean);
            
            // GitHub: /owner/repo/pull/123
            if (pathParts.includes('pull') && pathParts.length >= 4) {
                const prNumber = pathParts[pathParts.indexOf('pull') + 1];
                return `PR #${prNumber}`;
            }
            
            // GitLab: /owner/repo/-/merge_requests/123
            if (pathParts.includes('merge_requests') && pathParts.length >= 4) {
                const mrNumber = pathParts[pathParts.indexOf('merge_requests') + 1];
                return `MR !${mrNumber}`;
            }
            
            // Bitbucket: /workspace/repo/pull-requests/123
            if (pathParts.includes('pull-requests') && pathParts.length >= 4) {
                const prNumber = pathParts[pathParts.indexOf('pull-requests') + 1];
                return `PR #${prNumber}`;
            }
            
            return 'MR/PR';
        } catch (e) {
            return 'MR/PR';
        }
    };
    
    const [reviews, setReviews] = useState<Review[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [total, setTotal] = useState(0);
    const [currentPage, setCurrentPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const perPageOptions = [20, 50, 100];
    
    // Filter states
    const [filters, setFilters] = useState<ReviewsFilters>({
        page: 1,
        perPage: 20
    });
    const [searchQuery, setSearchQuery] = useState('');
    const [statusFilter, setStatusFilter] = useState<ReviewStatus | ''>('');
    const [providerFilter, setProviderFilter] = useState('');

    // Initialize filters from URL params
    useEffect(() => {
        const initialFilters: ReviewsFilters = {
            page: parseInt(searchParams.get('page') || '1'),
            perPage: parseInt(searchParams.get('per_page') || '20'),
            status: (searchParams.get('status') as ReviewStatus) || undefined,
            provider: searchParams.get('provider') || undefined,
            search: searchParams.get('search') || undefined,
        };
        
        setFilters(initialFilters);
        setCurrentPage(initialFilters.page || 1);
        setSearchQuery(initialFilters.search || '');
        setStatusFilter(initialFilters.status || '');
        setProviderFilter(initialFilters.provider || '');
    }, [searchParams]);

    // Fetch reviews from API
    const fetchReviews = useCallback(async (requestFilters?: ReviewsFilters) => {
        try {
            setLoading(true);
            setError(null);
            
            const filtersToUse = requestFilters || filters;
            const response = await getReviews(filtersToUse);
            
            // Defensive programming: handle null reviews array
            setReviews(response.reviews || []);
            setTotal(response.total || 0);
            setCurrentPage(response.page || 1);
            setTotalPages(response.totalPages || 0);
        } catch (err) {
            console.error('Error fetching reviews:', err);
            setError(err instanceof Error ? err.message : 'Failed to fetch reviews');
        } finally {
            setLoading(false);
        }
    }, [filters]);

    // Update URL params when filters change
    const updateFilters = useCallback((newFilters: Partial<ReviewsFilters>) => {
        const updatedFilters = { ...filters, ...newFilters };
        
        // Reset to page 1 when changing filters (except pagination)
        if (!newFilters.page) {
            updatedFilters.page = 1;
        }
        
        setFilters(updatedFilters);
        
        // Update URL search params
        const params = new URLSearchParams();
        if (updatedFilters.page && updatedFilters.page > 1) params.set('page', updatedFilters.page.toString());
        if (updatedFilters.perPage && updatedFilters.perPage !== 20) params.set('per_page', updatedFilters.perPage.toString());
        if (updatedFilters.status) params.set('status', updatedFilters.status);
        if (updatedFilters.provider) params.set('provider', updatedFilters.provider);
        if (updatedFilters.search) params.set('search', updatedFilters.search);
        
        setSearchParams(params);
        
        // Fetch with new filters
        fetchReviews(updatedFilters);
    }, [filters, setSearchParams, fetchReviews]);

    // Initial load
    useEffect(() => {
        fetchReviews();
    }, []);

    // Light auto-refresh while any review is non-terminal (created/in_progress)
    useEffect(() => {
        const hasActive = reviews.some(r => r.status === 'created' || r.status === 'in_progress');
        if (!hasActive) return;
        const id = setInterval(() => {
            fetchReviews();
        }, 15000); // 15s cadence to avoid hammering
        return () => clearInterval(id);
    }, [reviews, fetchReviews]);

    // Handle search
    const handleSearch = useCallback(() => {
        updateFilters({ search: searchQuery || undefined });
    }, [searchQuery, updateFilters]);

    // Handle filter changes
    const handleStatusFilter = useCallback((status: ReviewStatus | '') => {
        setStatusFilter(status);
        updateFilters({ status: status || undefined });
    }, [updateFilters]);

    const handleProviderFilter = useCallback((provider: string) => {
        setProviderFilter(provider);
        updateFilters({ provider: provider || undefined });
    }, [updateFilters]);

    const handlePerPageChange = useCallback((perPageValue: number) => {
        updateFilters({ perPage: perPageValue });
    }, [updateFilters]);

    // Handle pagination
    const handlePageChange = useCallback((page: number) => {
        updateFilters({ page });
    }, [updateFilters]);

    const handleViewReview = (reviewId: number) => {
        navigate(`/reviews/${reviewId}`);
    };

    // Clear all filters
    const clearFilters = useCallback(() => {
        setSearchQuery('');
        setStatusFilter('');
        setProviderFilter('');
        setFilters({ page: 1, perPage: 20 });
        setSearchParams(new URLSearchParams());
        fetchReviews({ page: 1, perPage: 20 });
    }, [setSearchParams, fetchReviews]);

    if (loading) {
        return (
            <div className="container mx-auto px-4 py-8">
                <div className="flex items-center justify-center min-h-64">
                    <div className="text-center">
                        <svg className="w-8 h-8 mx-auto mb-4 text-blue-500 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        <p className="text-slate-300">Loading reviews...</p>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="container mx-auto px-4 py-8">
            {/* Header */}
            <div className="flex items-center justify-between mb-8">
                <div>
                    <h1 className="text-3xl font-bold text-white mb-2">Code Reviews</h1>
                    <p className="text-slate-300">Manage and monitor your AI-powered code review sessions</p>
                </div>
                <Button
                    as={Link}
                    to="/reviews/new"
                    variant="primary"
                    icon={<Icons.Add />}
                    className="bg-green-600 hover:bg-green-700"
                >
                    Start New Review
                </Button>
            </div>

            {/* Filters */}
            <div className="bg-slate-800 rounded-lg p-6 mb-6 border border-slate-700">
                <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
                    {/* Search */}
                    <div className="md:col-span-2">
                        <div className="flex">
                            <input
                                type="text"
                                placeholder="Search repositories or URLs..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
                                className="flex-1 px-3 py-2 bg-slate-700 border border-slate-600 rounded-l-md text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                            />
                            <Button
                                onClick={handleSearch}
                                variant="primary"
                                className="rounded-l-none px-4"
                            >
                                <Icons.Search />
                            </Button>
                        </div>
                    </div>
                    
                    {/* Status Filter */}
                    <div>
                        <select
                            value={statusFilter}
                            onChange={(e) => handleStatusFilter(e.target.value as ReviewStatus | '')}
                            className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                        >
                            <option value="">All Statuses</option>
                            <option value="created">Created</option>
                            <option value="in_progress">In Progress</option>
                            <option value="completed">Completed</option>
                            <option value="failed">Failed</option>
                        </select>
                    </div>
                    
                    {/* Provider Filter */}
                    <div>
                        <select
                            value={providerFilter}
                            onChange={(e) => handleProviderFilter(e.target.value)}
                            className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                        >
                            <option value="">All Providers</option>
                            <option value="gitlab">GitLab</option>
                            <option value="github">GitHub</option>
                            <option value="bitbucket">Bitbucket</option>
                        </select>
                    </div>

                    {/* Per Page */}
                    <div>
                        <select
                            value={filters.perPage || 20}
                            onChange={(e) => handlePerPageChange(parseInt(e.target.value, 10))}
                            className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                        >
                            {perPageOptions.map(option => (
                                <option key={option} value={option}>
                                    {option} per page
                                </option>
                            ))}
                        </select>
                    </div>
                </div>
                
                {/* Active filters indicator and clear button */}
                {(searchQuery || statusFilter || providerFilter) && (
                    <div className="flex items-center justify-between mt-4 pt-4 border-t border-slate-700">
                        <div className="flex items-center space-x-2 text-sm text-slate-300">
                            <span>Active filters:</span>
                            {searchQuery && (
                                <span className="bg-blue-600 px-2 py-1 rounded text-white">
                                    Search: "{searchQuery}"
                                </span>
                            )}
                            {statusFilter && (
                                <span className="bg-blue-600 px-2 py-1 rounded text-white">
                                    Status: {getStatusText(statusFilter)}
                                </span>
                            )}
                            {providerFilter && (
                                <span className="bg-blue-600 px-2 py-1 rounded text-white">
                                    Provider: {providerFilter}
                                </span>
                            )}
                        </div>
                        <Button
                            onClick={clearFilters}
                            variant="ghost"
                            className="text-slate-400 hover:text-white"
                        >
                            Clear all
                        </Button>
                    </div>
                )}
            </div>

            {error && (
                <div className="mb-6 p-4 bg-red-900/50 border border-red-600 rounded-lg">
                    <div className="flex items-center">
                        <Icons.Error />
                        <span className="ml-2 text-red-200">{error}</span>
                        <Button
                            variant="ghost"
                            onClick={() => fetchReviews()}
                            className="ml-auto text-red-200 hover:text-white"
                        >
                            Retry
                        </Button>
                    </div>
                </div>
            )}

            {/* Reviews Table */}
            {reviews.length === 0 ? (
                <div className="text-center py-16">
                    <Icons.EmptyState />
                    <h3 className="text-xl font-medium text-slate-300 mt-4">No reviews found</h3>
                    <p className="text-slate-400 mt-2 mb-6">Get started by creating your first code review session</p>
                    <Button
                        as={Link}
                        to="/reviews/new"
                        variant="primary"
                        icon={<Icons.Add />}
                        className="bg-green-600 hover:bg-green-700"
                    >
                        Start New Review
                    </Button>
                </div>
            ) : (
                <div className="bg-slate-800 rounded-lg overflow-hidden border border-slate-700">
                    <div className="overflow-x-auto">
                        <table className="w-full">
                            <thead className="bg-slate-700">
                                <tr>
                                    <th className="px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider">
                                        Review
                                    </th>
                                    <th className="px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider">
                                        Status
                                    </th>
                                    <th className="px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider">
                                        Author  
                                    </th>
                                    <th className="px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider">
                                        Last Activity
                                    </th>
                                    <th className="px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider">
                                        Details
                                    </th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-slate-700">
                                {reviews.map((review) => {
                                    const rawMrDescriptor = review.prMrUrl ? extractMRInfo(review.prMrUrl) : '';
                                    const mrDescriptor = rawMrDescriptor && rawMrDescriptor !== 'MR/PR' ? rawMrDescriptor : '';
                                    const cleanedRepository = review.repository ? review.repository.replace(/^https?:\/\//, '').replace(/\/$/, '') : '';
                                    const repoSegments = cleanedRepository.split('/').filter(Boolean);
                                    const repoShort = repoSegments.length ? repoSegments[repoSegments.length - 1] : '';
                                    
                                    // Determine primary title based on review type
                                    let primaryTitle: string;
                                    if (review.triggerType === 'cli_diff') {
                                        // For CLI reviews, prefer AI summary title, then friendly name, then fallback
                                        primaryTitle = review.aiSummaryTitle?.trim() || review.friendlyName?.trim() || repoShort || 'CLI Review';
                                    } else {
                                        // For MR reviews, use MR title or fallback
                                        const mrTitleCandidate = review.mrTitle?.trim();
                                        primaryTitle = mrTitleCandidate && mrTitleCandidate.length > 0 ? mrTitleCandidate : (mrDescriptor || repoShort || 'Code Review');
                                    }
                                    
                                    const displayProviderRaw = review.provider ? review.provider.replace(/[-_]/g, ' ') : '';
                                    const displayProvider = displayProviderRaw ? toTitleCase(displayProviderRaw) : '';
                                    const fallbackAuthorFromUrl = review.prMrUrl ? extractAuthorFromUrl(review.prMrUrl) : null;
                                    const potentialAuthors = [
                                        review.authorName?.trim(),
                                        review.authorUsername?.trim(),
                                        review.userEmail?.trim(),
                                        fallbackAuthorFromUrl ? fallbackAuthorFromUrl.trim() : undefined,
                                        displayProvider,
                                    ].filter((val): val is string => Boolean(val && val.length > 0));
                                    const authorPrimary = potentialAuthors[0] || 'System';
                                    const authorSecondary = potentialAuthors.find(candidate => candidate !== authorPrimary);
                                    const branchLabel = review.branch?.trim();
                                    const metaChips = [
                                        cleanedRepository,
                                        branchLabel ? `Branch: ${branchLabel}` : '',
                                        mrDescriptor,
                                        displayProvider,
                                    ].filter((val): val is string => Boolean(val && val.length > 0));

                                    return (
                                        <tr
                                            key={review.id}
                                            className="group hover:bg-slate-700/50 transition-colors cursor-pointer focus-within:bg-slate-700/50"
                                            onClick={() => handleViewReview(review.id)}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter' || e.key === ' ') {
                                                    e.preventDefault();
                                                    handleViewReview(review.id);
                                                }
                                            }}
                                            tabIndex={0}
                                        >
                                            <td className="px-6 py-4">
                                                <div className="flex flex-col gap-1">
                                                    <div className="flex items-center gap-2">
                                                        <span className="text-white font-semibold">{primaryTitle}</span>
                                                        {review.prMrUrl && (
                                                            <a
                                                                href={review.prMrUrl}
                                                                target="_blank"
                                                                rel="noopener noreferrer"
                                                                onClick={(e) => e.stopPropagation()}
                                                                className="text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
                                                            >
                                                                Open PR/MR
                                                            </a>
                                                        )}
                                                    </div>
                                                    {metaChips.length > 0 && (
                                                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-400">
                                                            {metaChips.map((chip, idx) => (
                                                                <span key={`${chip}-${idx}`} className="flex items-center gap-1">
                                                                    {chip}
                                                                </span>
                                                            ))}
                                                        </div>
                                                    )}
                                                </div>
                                            </td>
                                            <td className="px-6 py-4">
                                                <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium text-white ${getStatusColor(review.status)}`}>
                                                    {getStatusText(review.status)}
                                                </span>
                                            </td>
                                            <td className="px-6 py-4 text-slate-300">
                                                <div className="font-medium text-white">{authorPrimary}</div>
                                                {authorSecondary && (
                                                    <div className="text-xs text-slate-400 mt-1">
                                                        {authorSecondary}
                                                    </div>
                                                )}
                                            </td>
                                            <td className="px-6 py-4 text-slate-300">
                                                {formatRelativeTime(review.completedAt || review.startedAt || review.createdAt)}
                                            </td>
                                            <td className="px-6 py-4">
                                                <Button
                                                    variant="outline"
                                                    size="sm"
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        handleViewReview(review.id);
                                                    }}
                                                    className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500"
                                                >
                                                    View Details
                                                </Button>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                    
                    {/* Pagination */}
                    {totalPages > 1 && (
                        <div className="px-6 py-4 border-t border-slate-700">
                            <div className="flex items-center justify-between">
                                <div className="text-sm text-slate-300">
                                    Showing {((currentPage - 1) * (filters.perPage || 20)) + 1} to {Math.min(currentPage * (filters.perPage || 20), total)} of {total} reviews
                                </div>
                                <div className="flex items-center space-x-2">
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => handlePageChange(currentPage - 1)}
                                        disabled={currentPage <= 1}
                                        className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500 disabled:opacity-50 disabled:cursor-not-allowed"
                                    >
                                        Previous
                                    </Button>
                                    
                                    {/* Page numbers */}
                                    <div className="flex items-center space-x-1">
                                        {Array.from({ length: Math.min(totalPages, 5) }, (_, i) => {
                                            let pageNum;
                                            if (totalPages <= 5) {
                                                pageNum = i + 1;
                                            } else if (currentPage <= 3) {
                                                pageNum = i + 1;
                                            } else if (currentPage >= totalPages - 2) {
                                                pageNum = totalPages - 4 + i;
                                            } else {
                                                pageNum = currentPage - 2 + i;
                                            }
                                            
                                            return (
                                                <Button
                                                    key={pageNum}
                                                    variant={pageNum === currentPage ? "primary" : "outline"}
                                                    size="sm"
                                                    onClick={() => handlePageChange(pageNum)}
                                                    className={
                                                        pageNum === currentPage 
                                                            ? "bg-blue-600 text-white"
                                                            : "border-slate-600 text-slate-300 hover:text-white hover:border-slate-500"
                                                    }
                                                >
                                                    {pageNum}
                                                </Button>
                                            );
                                        })}
                                    </div>
                                    
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        onClick={() => handlePageChange(currentPage + 1)}
                                        disabled={currentPage >= totalPages}
                                        className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500 disabled:opacity-50 disabled:cursor-not-allowed"
                                    >
                                        Next
                                    </Button>
                                </div>
                            </div>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};

export default Reviews;