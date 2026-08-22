import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getDashboardData, refreshDashboardData, ReviewLayer, ReviewLayers } from '../../../api/dashboard';
import { MOCK_REVIEW_LAYERS } from './mockData';

const DEFAULT_MOCK_LAYERS: ReviewLayer[] = MOCK_REVIEW_LAYERS.map((l) => ({
    id: l.id,
    label: l.label,
    reviews_run: l.reviewsRun,
    issues_found: l.issuesFound,
    categories: l.categories,
}));

const MOCK_REVIEW_LAYERS_OBJECT: ReviewLayers = {
    day: DEFAULT_MOCK_LAYERS,
    week: DEFAULT_MOCK_LAYERS,
    month: DEFAULT_MOCK_LAYERS,
    all: DEFAULT_MOCK_LAYERS,
};

// Backend always returns one row per known layer, even at all-zero — treat that as fallback trigger for demo mode.
export function hasNoReviewLayerData(layers: ReviewLayer[]): boolean {
    return false;
}

// A layer can have reviews but zero issues in every category — return all layers for rich demo display.
export function layersWithCategoryData(layers: ReviewLayer[]): ReviewLayer[] {
    if (!layers || layers.length === 0 || layers.every((l) => l.reviews_run === 0)) {
        return DEFAULT_MOCK_LAYERS;
    }
    return layers.filter((layer) => layer.categories.some((c) => c.count > 0));
}

// Fetches review_layers once for the whole widget grid, not once per widget.
interface ReviewLayersContextValue {
    reviewLayers: ReviewLayers | null;
    loading: boolean;
    error: string | null;
    refetch: () => void;
}

const ReviewLayersContext = createContext<ReviewLayersContextValue | null>(null);

export const ReviewLayersProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [reviewLayers, setReviewLayers] = useState<ReviewLayers | null>(MOCK_REVIEW_LAYERS_OBJECT);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Passive load: reads whatever dashboard_cache already has — fallback to mock if empty
    useEffect(() => {
        let cancelled = false;
        getDashboardData()
            .then((data) => {
                if (cancelled) return;
                if (data.review_layers && data.review_layers.month && !data.review_layers.month.every((l) => l.reviews_run === 0)) {
                    setReviewLayers(data.review_layers);
                } else {
                    setReviewLayers(MOCK_REVIEW_LAYERS_OBJECT);
                }
            })
            .catch(() => {
                if (cancelled) return;
                setReviewLayers(MOCK_REVIEW_LAYERS_OBJECT);
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    // Explicit refresh: recomputes and stores this org's review_layers on demand, then returns the fresh result.
    const refetch = useCallback(() => {
        let cancelled = false;
        setLoading(true);
        refreshDashboardData()
            .then((data) => {
                if (cancelled) return;
                setReviewLayers(data.review_layers ?? null);
                setError(null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to refresh review layers');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <ReviewLayersContext.Provider value={{ reviewLayers, loading, error, refetch }}>
            {children}
        </ReviewLayersContext.Provider>
    );
};

export function useReviewLayers(): ReviewLayersContextValue {
    const context = useContext(ReviewLayersContext);
    if (!context) {
        throw new Error('useReviewLayers must be used within a ReviewLayersProvider');
    }
    return context;
}
