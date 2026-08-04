import React from 'react';

// Generic loading placeholder, deliberately not shaped per chart type.
export const ChartSkeleton: React.FC = () => (
    <div className="w-full h-full flex flex-col items-center justify-center gap-3 animate-pulse p-6">
        <div className="w-3/4 h-4 bg-slate-700/60 rounded" />
        <div className="w-1/2 h-4 bg-slate-700/60 rounded" />
        <div className="w-2/3 h-4 bg-slate-700/60 rounded" />
        <div className="w-1/3 h-4 bg-slate-700/60 rounded" />
    </div>
);
