import React, { createContext, useContext } from 'react';
import { useDashboardQuery, ReviewLayer, ReviewLayers } from '../../../api/dashboard';

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

// Reads review_layers off the shared dashboard query (see useDashboardQuery) instead of
// fetching it independently - the whole widget grid shares one cached request/result.
export const ReviewLayersProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const { data, isLoading, error, refetch } = useDashboardQuery();

    const value: ReviewLayersContextValue = {
        reviewLayers: data?.review_layers ?? null,
        loading: isLoading,
        error: error ? (error instanceof Error ? error.message : 'Failed to load review layers') : null,
        refetch: () => { void refetch(); },
    };

    return (
        <ReviewLayersContext.Provider value={value}>
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
