import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getDashboardData, refreshDashboardData, ReviewLayer, ReviewLayers } from '../../../api/dashboard';

// Backend always returns one row per known layer, even at all-zero — treat that the same as "no layers at all".
export function hasNoReviewLayerData(layers: ReviewLayer[]): boolean {
    return layers.length === 0 || layers.every((layer) => layer.reviews_run === 0);
}

// A layer can have reviews but zero issues in every category — nothing to plot, so drop it rather than show a dangling node.
export function layersWithCategoryData(layers: ReviewLayer[]): ReviewLayer[] {
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
    const [reviewLayers, setReviewLayers] = useState<ReviewLayers | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Passive load: reads whatever dashboard_cache already has — cheap, no live query.
    useEffect(() => {
        let cancelled = false;
        getDashboardData()
            .then((data) => {
                if (cancelled) return;
                setReviewLayers(data.review_layers ?? null);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(err instanceof Error ? err.message : 'Failed to load review layers');
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
