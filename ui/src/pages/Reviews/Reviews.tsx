import React, {
    useCallback,
    useEffect,
    useMemo,
    useRef,
    useState,
} from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
    ColumnDef,
    SortingState,
    ColumnFiltersState,
    PaginationState,
    OnChangeFn,
    useReactTable,
    getCoreRowModel,
} from '@tanstack/react-table';
import { LuSearch } from 'react-icons/lu';
import {
    Button,
    Icons,
    Input,
    MultiSelectPanel,
} from '../../components/UIPrimitives';
import {
    normalizeSource,
    sourceLabel,
    SourceIcon,
    extractAuthorFromUrl,
    extractMRInfo,
    getExecutionBadge,
    reviewStatusBadge,
    getCleanRepository,
    getRepoShortName,
    getPrimaryTitle,
} from '../../utils/reviewDisplay';
import { ClientTable } from '../../components/DataTable/ClientTable';
import {
    SortIcon,
    SortableHeaderLabel,
    HeaderFilterPopover,
    multiSelectFilterFn,
    TruncatedWithTooltip,
} from '../../components/DataTable/HeaderControls';
import {
    getReviews,
    formatRelativeTime,
    getStatusText,
} from '../../api/reviews';
import { Review, ReviewsFilters, ReviewsSort } from '../../types/reviews';

const pageSizeOptions = [20, 50, 100];

// How long to wait after the last keystroke in the Review search box before
// actually firing the (server-side) request - typing shouldn't send one
// request per character.
const SEARCH_DEBOUNCE_MS = 300;

const TITLE_MAX = 80;
const AUTHOR_MAX = 20;
const truncate = (text: string, max: number): string =>
    text.length > max ? `${text.slice(0, max)}…` : text;

const REVIEWS_COLUMN_WIDTHS = [
    '18%',
    '12%',
    '14%',
    '12%',
    '12%',
    '10%',
    '12%',
    '10%',
];

// Reads initial Source/search filters from the URL (e.g. a "View Reviews" link) so a direct link lands pre-filtered.
const initialColumnFiltersFromURL = (params: URLSearchParams): ColumnFiltersState => {
    const filters: ColumnFiltersState = [];
    const search = params.get('search');
    if (search) filters.push({ id: 'review', value: search });
    const source = params.get('source');
    if (source) filters.push({ id: 'source', value: source });
    const status = params.get('status');
    if (status) filters.push({ id: 'status', value: status });
    return filters;
};

const Reviews: React.FC = () => {
    const navigate = useNavigate();
    const [searchParams, setSearchParams] = useSearchParams();

    const [reviews, setReviews] = useState<Review[]>([]);
    const [total, setTotal] = useState(0);
    const [totalPages, setTotalPages] = useState(1);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Sorting/filtering/pagination are server-driven; columnFilters also mirrors to/from the URL so filtered views are linkable.
    const [sorting, setSorting] = useState<SortingState>([]);
    const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() =>
        initialColumnFiltersFromURL(searchParams)
    );
    const [pagination, setPagination] = useState<PaginationState>({
        pageIndex: 0,
        pageSize: pageSizeOptions[0],
    });
    const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(
        null
    );

    // Mirrors of the three state values above, read by refetchCurrent (used by
    // the 15s auto-refresh poll) so that callback can stay stable rather than
    // resetting its interval on every keystroke/sort/page change.
    const sortingRef = useRef(sorting);
    const columnFiltersRef = useRef(columnFilters);
    const paginationRef = useRef(pagination);
    useEffect(() => {
        sortingRef.current = sorting;
    }, [sorting]);
    useEffect(() => {
        columnFiltersRef.current = columnFilters;
    }, [columnFilters]);
    useEffect(() => {
        paginationRef.current = pagination;
    }, [pagination]);

    const buildFetchParams = (
        sortingState: SortingState,
        filtersState: ColumnFiltersState,
        paginationState: PaginationState
    ): ReviewsFilters => {
        const sortEntry = sortingState[0];
        const getFilter = (id: string) =>
            filtersState.find((f) => f.id === id)?.value as string | undefined;
        return {
            page: paginationState.pageIndex + 1,
            perPage: paginationState.pageSize,
            search: getFilter('review') || undefined,
            status: getFilter('status') || undefined,
            provider: getFilter('source') || undefined,
            sort: sortEntry ? (sortEntry.id as ReviewsSort) : undefined,
            order: sortEntry ? (sortEntry.desc ? 'desc' : 'asc') : undefined,
        };
    };

    const fetchReviews = useCallback(async (params: ReviewsFilters) => {
        try {
            setLoading(true);
            setError(null);
            const response = await getReviews(params);
            setReviews(response.reviews || []);
            setTotal(response.total || 0);
            setTotalPages(response.totalPages || 1);
        } catch (err) {
            console.error('Error fetching reviews:', err);
            setError(
                err instanceof Error ? err.message : 'Failed to fetch reviews'
            );
        } finally {
            setLoading(false);
        }
    }, []);

    // Initial load.
    useEffect(() => {
        fetchReviews(buildFetchParams(sorting, columnFilters, pagination));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const refetchCurrent = useCallback(() => {
        fetchReviews(
            buildFetchParams(
                sortingRef.current,
                columnFiltersRef.current,
                paginationRef.current
            )
        );
    }, [fetchReviews]);

    // Light auto-refresh while any review on the current page is non-terminal
    // (created/in_progress).
    useEffect(() => {
        const hasActive = reviews.some(
            (r) => r.status === 'created' || r.status === 'in_progress'
        );
        if (!hasActive) return;
        const id = setInterval(() => refetchCurrent(), 15000); // 15s cadence to avoid hammering
        return () => clearInterval(id);
    }, [reviews, refetchCurrent]);

    const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
        const next = typeof updater === 'function' ? updater(sorting) : updater;
        const nextPagination = { ...pagination, pageIndex: 0 };
        setSorting(next);
        setPagination(nextPagination);
        fetchReviews(buildFetchParams(next, columnFilters, nextPagination));
    };

    // Mirrors filters into the URL (replace, not push, so filter changes don't spam history).
    const syncFiltersToURL = (filters: ColumnFiltersState) => {
        const next = new URLSearchParams();
        const search = filters.find((f) => f.id === 'review')?.value as string | undefined;
        const source = filters.find((f) => f.id === 'source')?.value as string | undefined;
        const status = filters.find((f) => f.id === 'status')?.value as string | undefined;
        if (search) next.set('search', search);
        if (source) next.set('source', source);
        if (status) next.set('status', status);
        setSearchParams(next, { replace: true });
    };

    const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
        updater
    ) => {
        const next =
            typeof updater === 'function' ? updater(columnFilters) : updater;
        setColumnFilters(next);
        syncFiltersToURL(next);
        const nextPagination = { ...pagination, pageIndex: 0 };

        const prevSearch = columnFilters.find((f) => f.id === 'review')?.value;
        const nextSearch = next.find((f) => f.id === 'review')?.value;
        if (prevSearch !== nextSearch) {
            // Free-text search: debounce the actual request, but let the input's
            // own value (bound to column.getFilterValue()) update immediately via
            // the setColumnFilters call above, so typing itself stays responsive.
            if (searchDebounceRef.current)
                clearTimeout(searchDebounceRef.current);
            searchDebounceRef.current = setTimeout(() => {
                setPagination(nextPagination);
                fetchReviews(buildFetchParams(sorting, next, nextPagination));
            }, SEARCH_DEBOUNCE_MS);
            return;
        }

        // Checkbox filters (Source/Status): discrete clicks, fetch right away.
        setPagination(nextPagination);
        fetchReviews(buildFetchParams(sorting, next, nextPagination));
    };

    const handlePaginationChange: OnChangeFn<PaginationState> = (updater) => {
        const next =
            typeof updater === 'function' ? updater(pagination) : updater;
        setPagination(next);
        fetchReviews(buildFetchParams(sorting, columnFilters, next));
    };

    const handleViewReview = useCallback(
        (review: Review) => {
            navigate(`/reviews/${review.id}`);
        },
        [navigate]
    );

    const columns = useMemo<ColumnDef<Review>[]>(
        () => [
            {
                id: 'review',
                accessorFn: (review) => getPrimaryTitle(review),
                // Searches title/repo/branch/URL/author together, same combined
                // free-text search the old top search bar offered (it searched
                // "repositories or URLs"; author is included too since it's visible
                // in this same column).
                filterFn: (row, _columnId, filterValue: string) => {
                    const q = (filterValue || '').trim().toLowerCase();
                    if (!q) return true;
                    const r = row.original;
                    return [
                        r.repository,
                        r.branch,
                        r.prMrUrl,
                        r.mrTitle,
                        r.friendlyName,
                        r.aiSummaryTitle,
                        r.authorName,
                        r.authorUsername,
                    ]
                        .filter((v): v is string => Boolean(v))
                        .some((v) => v.toLowerCase().includes(q));
                },
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Review"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <div className="flex items-center gap-2">
                            <SortIcon
                                sorted={column.getIsSorted()}
                                onToggle={column.getToggleSortingHandler()}
                            />
                            <HeaderFilterPopover
                                icon={LuSearch}
                                label="Search reviews"
                            >
                                <Input
                                    placeholder="Search repositories, URLs, or author..."
                                    value={
                                        (column.getFilterValue() as string) ??
                                        ''
                                    }
                                    onChange={(e) =>
                                        column.setFilterValue(
                                            e.target.value || undefined
                                        )
                                    }
                                    icon={<Icons.Search />}
                                    aria-label="Search reviews"
                                    className="text-sm"
                                />
                            </HeaderFilterPopover>
                        </div>
                    </div>
                ),
                cell: ({ row }) => {
                    const review = row.original;
                    const rawMrDescriptor = review.prMrUrl
                        ? extractMRInfo(review.prMrUrl)
                        : '';
                    const mrDescriptor =
                        rawMrDescriptor && rawMrDescriptor !== 'MR/PR'
                            ? rawMrDescriptor
                            : '';
                    const metaChips = [mrDescriptor].filter((v): v is string =>
                        Boolean(v)
                    );
                    const primaryTitle = getPrimaryTitle(review);
                    const displayTitle = truncate(primaryTitle, TITLE_MAX);

                    return (
                        <div className="flex flex-col justify-center gap-1 min-w-0 min-h-[44px]">
                            <div className="flex items-center gap-2 min-w-0">
                                <span className="min-w-0 truncate text-white font-semibold">
                                    <TruncatedWithTooltip
                                        text={primaryTitle}
                                        max={TITLE_MAX}
                                    >
                                        {displayTitle}
                                    </TruncatedWithTooltip>
                                </span>
                                {review.prMrUrl && (
                                    <a
                                        href={review.prMrUrl}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        onClick={(e) => e.stopPropagation()}
                                        className="flex-shrink-0 text-blue-400 hover:text-blue-300 text-xs font-medium underline underline-offset-2"
                                    >
                                        Open PR/MR
                                    </a>
                                )}
                            </div>
                            {metaChips.length > 0 && (
                                <div className="min-w-0 text-sm text-slate-400 truncate">
                                    {metaChips.join(' · ')}
                                </div>
                            )}
                        </div>
                    );
                },
            },
            {
                id: 'branch',
                accessorFn: (review) => review.branch?.trim() || '',
                enableColumnFilter: false,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Branch"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <SortIcon
                            sorted={column.getIsSorted()}
                            onToggle={column.getToggleSortingHandler()}
                        />
                    </div>
                ),
                cell: ({ getValue }) => {
                    const branch = getValue() as string;
                    return (
                        <TruncatedWithTooltip text={branch} max={TITLE_MAX}>
                            <div className="min-w-0 truncate text-white text-sm">
                                {truncate(branch, TITLE_MAX) || '—'}
                            </div>
                        </TruncatedWithTooltip>
                    );
                },
            },
            {
                id: 'repository',
                accessorFn: (review) => getCleanRepository(review),
                // Free-text search lives on the "Review" column only (one combined
                // `search` query param on the backend) - this column is sort-only.
                enableColumnFilter: false,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Repository"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <SortIcon
                            sorted={column.getIsSorted()}
                            onToggle={column.getToggleSortingHandler()}
                        />
                    </div>
                ),
                cell: ({ getValue }) => {
                    const cleanedRepository = getValue() as string;
                    return (
                        <TruncatedWithTooltip
                            text={cleanedRepository}
                            max={TITLE_MAX}
                        >
                            <div className="min-w-0 truncate text-white text-sm">
                                {truncate(cleanedRepository, TITLE_MAX) || '—'}
                            </div>
                        </TruncatedWithTooltip>
                    );
                },
            },
            {
                id: 'source',
                accessorFn: (review) => normalizeSource(review.provider, review.triggerType),
                filterFn: multiSelectFilterFn,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Source"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <div className="flex items-center gap-2">
                            <SortIcon
                                sorted={column.getIsSorted()}
                                onToggle={column.getToggleSortingHandler()}
                            />
                            <HeaderFilterPopover>
                                <MultiSelectPanel
                                    label="Sources"
                                    value={
                                        (column.getFilterValue() as string) ??
                                        ''
                                    }
                                    onChange={(v) =>
                                        column.setFilterValue(v || undefined)
                                    }
                                    options={[
                                        { value: 'github', label: 'GitHub' },
                                        { value: 'gitlab', label: 'GitLab' },
                                        {
                                            value: 'bitbucket',
                                            label: 'Bitbucket',
                                        },
                                        { value: 'gitea', label: 'Gitea' },
                                        {
                                            value: 'azuredevops',
                                            label: 'Azure DevOps',
                                        },
                                        { value: 'cli', label: 'CLI' },
                                        { value: 'scheduled', label: 'Scheduled' },
                                    ]}
                                />
                            </HeaderFilterPopover>
                        </div>
                    </div>
                ),
                cell: ({ row }) => (
                    <div className="flex items-center gap-1.5 text-white">
                        <SourceIcon provider={row.original.provider} triggerType={row.original.triggerType} />
                        {sourceLabel(row.original.provider, row.original.triggerType)}
                    </div>
                ),
            },
            {
                id: 'status',
                accessorKey: 'status',
                filterFn: multiSelectFilterFn,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Status"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <div className="flex items-center gap-2">
                            <SortIcon
                                sorted={column.getIsSorted()}
                                onToggle={column.getToggleSortingHandler()}
                            />
                            <HeaderFilterPopover>
                                <MultiSelectPanel
                                    label="Statuses"
                                    value={
                                        (column.getFilterValue() as string) ??
                                        ''
                                    }
                                    onChange={(v) =>
                                        column.setFilterValue(v || undefined)
                                    }
                                    options={[
                                        { value: 'created', label: 'Created' },
                                        {
                                            value: 'in_progress',
                                            label: 'In Progress',
                                        },
                                        {
                                            value: 'completed',
                                            label: 'Completed',
                                        },
                                        { value: 'failed', label: 'Failed' },
                                    ]}
                                />
                            </HeaderFilterPopover>
                        </div>
                    </div>
                ),
                cell: ({ row }) => {
                    const review = row.original;
                    const badge = reviewStatusBadge(review.status);
                    const executionBadge = getExecutionBadge(review);
                    return (
                        <div className="flex flex-wrap items-center gap-1.5">
                            <span
                                className="inline-flex items-center rounded-md px-2 py-0.5 text-sm font-medium w-fit"
                                style={{
                                    backgroundColor: 'transparent',
                                    color: badge.color,
                                    border: `1px solid ${badge.borderColor}`,
                                }}
                            >
                                {getStatusText(review.status)}
                            </span>
                            {executionBadge && (
                                <span
                                    className="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium w-fit"
                                    style={{
                                        backgroundColor: 'transparent',
                                        color: executionBadge.color,
                                        border: `1px solid ${executionBadge.borderColor}`,
                                    }}
                                >
                                    {executionBadge.label}
                                </span>
                            )}
                        </div>
                    );
                },
            },
            {
                id: 'author',
                accessorFn: (review) => {
                    const fallbackAuthorFromUrl = review.prMrUrl
                        ? extractAuthorFromUrl(review.prMrUrl)
                        : null;
                    return (
                        review.authorName?.trim() ||
                        review.authorUsername?.trim() ||
                        review.userEmail?.trim() ||
                        fallbackAuthorFromUrl?.trim() ||
                        'System'
                    );
                },
                enableColumnFilter: false,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Author"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <SortIcon
                            sorted={column.getIsSorted()}
                            onToggle={column.getToggleSortingHandler()}
                        />
                    </div>
                ),
                cell: ({ getValue }) => {
                    const author = getValue() as string;
                    return (
                        <TruncatedWithTooltip text={author} max={AUTHOR_MAX}>
                            <div className="min-w-0 truncate text-white text-sm">
                                {truncate(author, AUTHOR_MAX)}
                            </div>
                        </TruncatedWithTooltip>
                    );
                },
            },
            {
                id: 'last_activity',
                accessorFn: (review) =>
                    review.completedAt || review.startedAt || review.createdAt,
                enableColumnFilter: false,
                header: ({ column }) => (
                    <div className="flex items-center justify-between gap-2">
                        <SortableHeaderLabel
                            label="Last Activity"
                            onToggle={column.getToggleSortingHandler()}
                        />
                        <SortIcon
                            sorted={column.getIsSorted()}
                            onToggle={column.getToggleSortingHandler()}
                        />
                    </div>
                ),
                cell: ({ getValue }) => (
                    <div className="text-white text-sm">
                        {formatRelativeTime(getValue() as string)}
                    </div>
                ),
            },
            {
                id: 'details',
                enableSorting: false,
                enableColumnFilter: false,
                header: () => (
                    <span className="font-semibold text-slate-300 uppercase tracking-wide text-xs">
                        Details
                    </span>
                ),
                cell: ({ row }) => (
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={(e) => {
                            e.stopPropagation();
                            handleViewReview(row.original);
                        }}
                        className="border-slate-400 text-white hover:bg-white/10 hover:border-white text-sm cursor-pointer"
                    >
                        View Details
                    </Button>
                ),
            },
            // eslint-disable-next-line react-hooks/exhaustive-deps
        ],
        [handleViewReview]
    );

    // Server-side: TanStack just renders whatever page of already-
    // filtered/sorted rows the API returned, instead of computing
    // filtered/sorted/paginated row models itself from a client-side array.
    const table = useReactTable({
        data: reviews,
        columns,
        getCoreRowModel: getCoreRowModel(),
        manualFiltering: true,
        manualSorting: true,
        manualPagination: true,
        pageCount: totalPages,
        state: { sorting, columnFilters, pagination },
        onSortingChange: handleSortingChange,
        onColumnFiltersChange: handleColumnFiltersChange,
        onPaginationChange: handlePaginationChange,
    });

    return (
        <div className="container mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-8">
                <div>
                    <h1 className="text-3xl font-bold text-white mb-2">
                        {columnFilters.find((f) => f.id === 'source')?.value === 'scheduled' ? 'List Scheduled Reviews' : 'List Reviews'}
                    </h1>
                    <p className="text-slate-300">
                        Manage and monitor your AI-powered code review sessions
                    </p>
                </div>
                <Button
                    as={Link}
                    to="/reviews/new"
                    variant="primary"
                    icon={<Icons.Add />}
                    title="Safe preview - no comments posted"
                >
                    New Review
                </Button>
            </div>

            <ClientTable
                table={table}
                columnWidths={REVIEWS_COLUMN_WIDTHS}
                loading={loading}
                loadingLabel="Loading reviews..."
                error={error}
                onRetry={refetchCurrent}
                // Don't hide the search/filter controls when a filter causes zero results.
                isEmpty={total === 0 && columnFilters.length === 0}
                empty={{
                    title: 'No review previews found',
                    description:
                        'Get started by creating your first preview session (safe - no comments posted)',
                    action: (
                        <Button
                            as={Link}
                            to="/reviews/new"
                            variant="primary"
                            icon={<Icons.Add />}
                        >
                            New Review
                        </Button>
                    ),
                }}
                pageSizeOptions={pageSizeOptions}
                onRowClick={handleViewReview}
                manualTotal={total}
            />
        </div>
    );
};

export default Reviews;
