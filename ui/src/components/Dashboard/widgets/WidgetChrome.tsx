import React, { useState } from 'react';
import { motion } from 'framer-motion';
import classNames from 'classnames';
import { CATEGORY_BADGE_CLASSES, CATEGORY_LABELS, WidgetCategory } from './registry';

const DragHandleIcon: React.FC = () => (
    <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
        <circle cx="6" cy="5" r="1.4" />
        <circle cx="14" cy="5" r="1.4" />
        <circle cx="6" cy="10" r="1.4" />
        <circle cx="14" cy="10" r="1.4" />
        <circle cx="6" cy="15" r="1.4" />
        <circle cx="14" cy="15" r="1.4" />
    </svg>
);

const ExpandIcon: React.FC = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4" />
    </svg>
);

const CloseIcon: React.FC = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
    </svg>
);

const RemoveIcon: React.FC = () => (
    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M6 18L18 6M6 6l12 12" />
    </svg>
);

interface WidgetChromeProps {
    title: string;
    category: WidgetCategory;
    editMode: boolean;
    animationIndex?: number;
    onRemove?: () => void;
    children: React.ReactNode;
}

export const WidgetChrome: React.FC<WidgetChromeProps> = ({
    title,
    category,
    editMode,
    animationIndex = 0,
    onRemove,
    children,
}) => {
    const [expanded, setExpanded] = useState(false);

    const header = (
        <div className="flex items-center justify-between gap-2 px-4 py-3 border-b border-slate-700/60 shrink-0">
            <div className="flex items-center gap-2 min-w-0">
                {editMode && (
                    <span
                        className="widget-drag-handle cursor-grab active:cursor-grabbing text-slate-500 hover:text-slate-300 transition-colors"
                        title="Drag to move"
                    >
                        <DragHandleIcon />
                    </span>
                )}
                <h3 className="text-sm font-semibold text-slate-100 truncate">{title}</h3>
                <span className={classNames('shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded-full uppercase tracking-wide', CATEGORY_BADGE_CLASSES[category])}>
                    {CATEGORY_LABELS[category]}
                </span>
            </div>
            <div className="flex items-center gap-1 shrink-0">
                {!editMode && (
                    <button
                        type="button"
                        onClick={() => setExpanded(true)}
                        className="p-1 rounded text-slate-500 hover:text-slate-200 hover:bg-slate-700/60 transition-colors"
                        title="Expand"
                        aria-label={`Expand ${title}`}
                    >
                        <ExpandIcon />
                    </button>
                )}
                {editMode && onRemove && (
                    <button
                        type="button"
                        onClick={onRemove}
                        className="p-1 rounded text-slate-500 hover:text-red-300 hover:bg-red-900/30 transition-colors"
                        title="Remove widget"
                        aria-label={`Remove ${title}`}
                    >
                        <RemoveIcon />
                    </button>
                )}
            </div>
        </div>
    );

    return (
        <>
            <motion.div
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: Math.min(animationIndex * 0.05, 0.6), duration: 0.4 }}
                className={classNames(
                    'h-full flex flex-col bg-slate-800/90 rounded-lg shadow-sm border border-slate-700/80 backdrop-blur-sm',
                    'hover:shadow-md hover:border-slate-600/80 transition-all duration-200 overflow-hidden'
                )}
            >
                {header}
                <div className="flex-1 min-h-0 p-4 overflow-hidden">
                    {children}
                </div>
            </motion.div>

            {expanded && (
                <div className="fixed inset-0 z-[100] flex items-center justify-center bg-slate-950/70 backdrop-blur-sm p-6">
                    <div className="w-full max-w-5xl h-[80vh] flex flex-col bg-slate-800/95 rounded-xl shadow-2xl border border-slate-700/80">
                        <div className="flex items-center justify-between gap-2 px-5 py-4 border-b border-slate-700/60 shrink-0">
                            <h3 className="text-base font-semibold text-slate-100">{title}</h3>
                            <button
                                type="button"
                                onClick={() => setExpanded(false)}
                                className="p-1.5 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-700/60 transition-colors"
                                aria-label="Close expanded view"
                            >
                                <CloseIcon />
                            </button>
                        </div>
                        <div className="flex-1 min-h-0 p-5 overflow-hidden">
                            {children}
                        </div>
                    </div>
                </div>
            )}
        </>
    );
};
