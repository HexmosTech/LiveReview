import React from 'react';
import { Input } from './UIPrimitives';

// Searchable model picker shared by the AI Providers connector form and Settings → Managed AI: a dropdown of the
// provider's catalog models with a search box, plus "Custom model…" for any other model ID. Give it
// key={provider} so switching provider resets the search and custom mode.

interface AIModelSelectProps {
    models: string[];
    defaultModel?: string;
    loading?: boolean;
    value: string;
    onChange: (model: string) => void;
    helperText?: React.ReactNode;
    customPlaceholder?: string;
    // Lets the parent lift its own stacking context while the list is open.
    onOpenChange?: (open: boolean) => void;
}

const Spin: React.FC<{ className: string }> = ({ className }) => (
    <svg className={`animate-spin ${className}`} xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
    </svg>
);

const AIModelSelect: React.FC<AIModelSelectProps> = ({
    models,
    defaultModel = '',
    loading = false,
    value,
    onChange,
    helperText,
    customPlaceholder = 'Enter model ID (e.g., gpt-4.1, o3-mini)',
    onOpenChange,
}) => {
    const [customModelMode, setCustomModelMode] = React.useState(false);
    const [searchQuery, setSearchQuery] = React.useState('');
    const [isOpen, setIsOpenState] = React.useState(false);
    const dropdownRef = React.useRef<HTMLDivElement>(null);

    const setIsOpen = React.useCallback(
        (open: boolean) => {
            setIsOpenState(open);
            onOpenChange?.(open);
        },
        [onOpenChange]
    );

    // Close dropdown on click outside
    React.useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, [setIsOpen]);

    const currentModelValue = value || defaultModel || '';
    const usesCustomModel = !!currentModelValue && !models.includes(currentModelValue);
    const shouldShowCustomModelInput = customModelMode || usesCustomModel;

    // Models come straight from the provider API catalog (dynamic), filtered only by
    // the search box. No client-side pruning: whatever the backend returns is shown.
    const filteredModels = React.useMemo(() => {
        if (!searchQuery) return models;
        return models.filter((model) => model.toLowerCase().includes(searchQuery.toLowerCase()));
    }, [models, searchQuery]);

    React.useEffect(() => {
        // Auto-exit custom mode only if the model matches a standard one and is non-empty.
        // This prevents the input from disappearing while the user is actively typing.
        if (!usesCustomModel && value !== '') {
            setCustomModelMode(false);
        }
    }, [usesCustomModel, value]);

    return (
        <div className="space-y-1 relative z-[50]">
            <div className="flex justify-between items-center">
                <label className="block text-sm font-medium text-slate-300">Model</label>
                {loading && <span className="text-xs text-blue-400 animate-pulse">Syncing models from server...</span>}
            </div>
            {/* Custom Searchable Select Dropdown */}
            <div ref={dropdownRef} className={`relative w-full ${isOpen ? 'z-[100]' : 'z-0'}`}>
                <button
                    type="button"
                    className="w-full bg-slate-700 border border-slate-600 text-white rounded-md px-3 py-2 text-left flex justify-between items-center focus:outline-none focus:ring-2 focus:ring-blue-500 cursor-pointer disabled:cursor-not-allowed disabled:opacity-80"
                    onClick={() => setIsOpen(!isOpen)}
                    disabled={loading}
                >
                    <span className="truncate flex items-center">
                        {loading && <Spin className="-ml-1 mr-2.5 h-4 w-4 text-blue-400" />}
                        <span className="truncate">
                            {loading
                                ? 'Loading models...'
                                : shouldShowCustomModelInput
                                    ? 'Custom model…'
                                    : currentModelValue
                                        ? `${currentModelValue}${currentModelValue === defaultModel ? ' (Default)' : ''}`
                                        : 'Select a model'}
                        </span>
                    </span>
                    <span className="text-slate-400 ml-2">{isOpen ? '▲' : '▼'}</span>
                </button>

                {isOpen && (
                    <div className="absolute z-[999] mt-1 w-full bg-slate-800 border border-slate-700 rounded-md shadow-2xl max-h-80 overflow-y-auto focus:outline-none">
                        {/* Search Input inside the dropdown menu */}
                        <div className="sticky top-0 p-2 bg-slate-800 border-b border-slate-700 z-10">
                            <input
                                type="text"
                                placeholder="Search model name..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="block w-full bg-slate-900 border border-slate-700 text-white rounded px-2.5 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
                                autoFocus
                                autoComplete="off"
                                onClick={(e) => e.stopPropagation()} // Prevent closing dropdown on clicking input
                            />
                        </div>

                        {/* Options List */}
                        <div className="py-1">
                            {loading ? (
                                <div className="flex items-center justify-center py-6 text-slate-400 space-x-2">
                                    <Spin className="h-5 w-5 text-indigo-500" />
                                    <span>Loading models...</span>
                                </div>
                            ) : (
                                <>
                                    {filteredModels.map((model) => {
                                        const isSelected = !shouldShowCustomModelInput && currentModelValue === model;
                                        return (
                                            <button
                                                key={model}
                                                type="button"
                                                className={`w-full text-left px-3 py-2 text-sm hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${isSelected ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                                    }`}
                                                onClick={() => {
                                                    setCustomModelMode(false);
                                                    onChange(model);
                                                    setIsOpen(false);
                                                    setSearchQuery('');
                                                }}
                                            >
                                                <span className="truncate">{model}</span>
                                                {model === defaultModel && (
                                                    <span className="text-[10px] bg-slate-700 text-slate-300 px-1.5 py-0.5 rounded">Default</span>
                                                )}
                                            </button>
                                        );
                                    })}

                                    {/* Custom Model Option */}
                                    <button
                                        type="button"
                                        className={`w-full text-left px-3 py-2 text-sm border-t border-slate-700 hover:bg-indigo-600 hover:text-white transition-colors flex justify-between items-center ${shouldShowCustomModelInput ? 'bg-slate-700 text-indigo-400 font-medium' : 'text-slate-200'
                                            }`}
                                        onClick={() => {
                                            setCustomModelMode(true);
                                            if (!usesCustomModel) {
                                                onChange('');
                                            }
                                            setIsOpen(false);
                                            setSearchQuery('');
                                        }}
                                    >
                                        <span>Custom model…</span>
                                    </button>

                                    {filteredModels.length === 0 && (
                                        <div className="px-3 py-2 text-sm text-slate-500 italic">No matching models found</div>
                                    )}
                                </>
                            )}
                        </div>
                    </div>
                )}
            </div>
            {shouldShowCustomModelInput && (
                <Input
                    label="Custom Model ID"
                    name="selectedModel"
                    autoComplete="off"
                    value={usesCustomModel ? currentModelValue : ''}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
                    placeholder={customPlaceholder}
                />
            )}
            {helperText && <p className="text-xs text-slate-400">{helperText}</p>}
        </div>
    );
};

export default AIModelSelect;
