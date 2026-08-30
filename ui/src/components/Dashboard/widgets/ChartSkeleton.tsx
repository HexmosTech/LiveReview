import React from 'react';

// Generic loading placeholder, deliberately not shaped per chart type. Uses a shimmer sweep
// (skeleton-shimmer, styles/custom.css) rather than a uniform opacity pulse - the pulse read as
// "stuck" rather than "loading" (user feedback, persisted across rounds even with animate-pulse
// already present - a moving highlight is a much stronger "in progress" signal).
export const ChartSkeleton: React.FC = () => (
    <div className="w-full h-full flex flex-col items-center justify-center gap-3 p-6">
        <div className="w-3/4 h-4 skeleton-shimmer rounded" />
        <div className="w-1/2 h-4 skeleton-shimmer rounded" />
        <div className="w-2/3 h-4 skeleton-shimmer rounded" />
        <div className="w-1/3 h-4 skeleton-shimmer rounded" />
    </div>
);
