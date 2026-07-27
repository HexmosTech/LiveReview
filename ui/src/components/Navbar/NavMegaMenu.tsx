import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import classNames from 'classnames';
import { Icons } from '../UIPrimitives';
import { flattenMegaMenuSections, MegaMenuGroupNode, MegaMenuLinkNode, MegaMenuNode, MegaMenuSearchEntry, MegaMenuSection } from './megaMenuData';
import { shortcutKeyLabel } from '../../utils/platform';


// Ranking tiers (lower score wins). Each tier's floor is set well above the previous tier's
// realistic ceiling (UI labels/breadcrumbs here never approach hundreds of characters), so a
// worse-tier match can never outrank a better-tier one:
//   0..~100         substring match on the entry's own name (ranked by how early it appears)
//   SUBSEQUENCE_MATCH_SCORE (1000)   in-order subsequence match on the name (typo/partial tolerance)
//   BREADCRUMB_MATCH_OFFSET (2000) + the above  same two tiers, but matched on the breadcrumb instead
const SUBSEQUENCE_MATCH_SCORE = 1000;
const BREADCRUMB_MATCH_OFFSET = 2000;

const matchScore = (query: string, target: string): number | null => {
    const q = query.toLowerCase();
    const t = target.toLowerCase();
    if (!q) return null;
    const substringIndex = t.indexOf(q);
    if (substringIndex !== -1) return substringIndex;

    let qi = 0;
    for (let ti = 0; ti < t.length && qi < q.length; ti++) {
        if (t[ti] === q[qi]) qi += 1;
    }
    return qi === q.length ? SUBSEQUENCE_MATCH_SCORE : null;
};

const scoreSearchEntry = (entry: MegaMenuSearchEntry, query: string): number | null => {
    const nameScore = matchScore(query, entry.name);
    if (nameScore !== null) return nameScore;
    const breadcrumbScore = matchScore(query, entry.breadcrumb);
    return breadcrumbScore === null ? null : breadcrumbScore + BREADCRUMB_MATCH_OFFSET;
};

type NavMegaMenuProps = {
    isOpen: boolean;
    onClose: () => void;
    sections: MegaMenuSection[];
    highlightKey?: string | null;
    searchFocusToken?: number;
};

const LinkRow: React.FC<{ node: MegaMenuLinkNode; goTo: (path: string) => void }> = ({ node, goTo }) => (
    <li>
        <button
            type="button"
            onClick={() => goTo(node.path)}
            className="flex w-full items-center gap-2 whitespace-nowrap rounded-lg px-2 py-1.5 text-left text-xs text-slate-300 transition-colors hover:bg-slate-700/60 hover:text-white"
        >
            <span className="shrink-0 text-slate-400">{node.icon}</span>
            <span className="truncate">{node.name}</span>
        </button>
    </li>
);

// A group nested inside another group (not used by any current data, but rendered flat/always-open
// as a fallback so a future 3-level entry degrades gracefully instead of needing a 3rd hover layer).
const NestedGroupFallback: React.FC<{ node: MegaMenuGroupNode; goTo: (path: string) => void }> = ({ node, goTo }) => (
    <li className="pt-1">
        <div className="px-2 pb-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-500">{node.name}</div>
        <ul className="space-y-0.5">
            {node.children.map((child) => (
                child.kind === 'link'
                    ? <LinkRow key={`${child.path}:${child.name}`} node={child} goTo={goTo} />
                    : <NestedGroupFallback key={child.name} node={child} goTo={goTo} />
            ))}
        </ul>
    </li>
);

// Renders one top-level nav category. Which of its groups is expanded is tracked here, at the
// column level, and only collapses when the mouse leaves the WHOLE column (not a single row) —
// otherwise moving from an open group down to a sibling link collapses the group mid-transit and
// the target jumps out from under the cursor.
const SectionColumn: React.FC<{ section: MegaMenuSection; goTo: (path: string) => void; isHighlighted?: boolean; forceExpandAll?: boolean }> = ({ section, goTo, isHighlighted, forceExpandAll }) => {
    // Groups a user has hovered stay expanded — switching to a sibling group must never collapse
    // another one, since collapsing shifts content under the cursor and closes everything.
    const [openGroups, setOpenGroups] = useState<Set<string>>(new Set());
    const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    const cancelClose = () => {
        if (closeTimerRef.current) {
            clearTimeout(closeTimerRef.current);
            closeTimerRef.current = null;
        }
    };

    const scheduleClose = () => {
        cancelClose();
        closeTimerRef.current = setTimeout(() => setOpenGroups(new Set()), 300);
    };

    const openGroup = (name: string) => {
        cancelClose();
        setOpenGroups((prev) => (prev.has(name) ? prev : new Set(prev).add(name)));
    };

    return (
        <div
            className={classNames(
                'min-w-[150px] flex-1 rounded-lg p-2 -m-2 transition-colors',
                isHighlighted && 'bg-slate-800/60'
            )}
            onMouseEnter={cancelClose}
            onMouseLeave={scheduleClose}
        >
            {section.items && section.items.length > 0 && (
                <>
                    <div className="flex items-center gap-2 px-2 pb-1 text-sm font-semibold text-white">
                        <span className="text-blue-400">{section.icon}</span>
                        {section.name}
                    </div>
                    <ul className="space-y-0.5">
                        {section.items.map((node: MegaMenuNode) => {
                            if (node.kind === 'link') {
                                return <LinkRow key={`${node.path}:${node.name}`} node={node} goTo={goTo} />;
                            }
                            const isOpen = forceExpandAll || openGroups.has(node.name);
                            return (
                                <li key={node.name}>
                                    <div
                                        onMouseEnter={() => openGroup(node.name)}
                                        className={classNames(
                                            'flex cursor-default items-center gap-1 whitespace-nowrap rounded-lg px-2 py-1.5 text-left text-xs font-medium transition-colors',
                                            isOpen ? 'bg-slate-700/60 text-white' : 'text-slate-300 hover:bg-slate-700/60 hover:text-white'
                                        )}
                                    >
                                        <span className="flex min-w-0 items-center gap-2">
                                            {node.icon && <span className="shrink-0 text-slate-400">{node.icon}</span>}
                                            <span className="truncate">{node.name}</span>
                                        </span>
                                        <span className={classNames('shrink-0 transition-transform', isOpen && 'rotate-90')}>
                                            <Icons.ChevronRight />
                                        </span>
                                    </div>
                                    {isOpen && (
                                        <ul className="ml-2 mt-0.5 space-y-0.5 border-l border-slate-700/60 pl-2">
                                            {node.children.map((child) => (
                                                child.kind === 'link'
                                                    ? <LinkRow key={`${child.path}:${child.name}`} node={child} goTo={goTo} />
                                                    : <NestedGroupFallback key={child.name} node={child} goTo={goTo} />
                                            ))}
                                        </ul>
                                    )}
                                </li>
                            );
                        })}
                    </ul>
                </>
            )}
        </div>
    );
};

const SearchResultRow: React.FC<{ entry: MegaMenuSearchEntry; isFirst: boolean; goTo: (path: string) => void }> = ({ entry, isFirst, goTo }) => (
    <li>
        <button
            type="button"
            onClick={() => goTo(entry.path)}
            className={classNames(
                'flex w-full items-center gap-3 whitespace-nowrap rounded-lg px-3 py-2 text-left transition-colors hover:bg-slate-700/60',
                isFirst && 'bg-slate-800/60'
            )}
        >
            <span className="shrink-0 text-slate-400">{entry.icon}</span>
            <span className="min-w-0">
                <span className="block truncate text-sm text-white">{entry.name}</span>
                <span className="block truncate text-[11px] text-slate-400">{entry.breadcrumb}</span>
            </span>
        </button>
    </li>
);

export const NavMegaMenu: React.FC<NavMegaMenuProps> = ({ isOpen, onClose, sections, highlightKey, searchFocusToken }) => {
    const navigate = useNavigate();
    const [query, setQuery] = useState('');
    const [isAllExpanded, setIsAllExpanded] = useState(false);
    const inputRef = useRef<HTMLInputElement>(null);

    const searchEntries = useMemo(() => flattenMegaMenuSections(sections), [sections]);

    const results = useMemo(() => {
        const trimmed = query.trim();
        if (!trimmed) return [];
        return searchEntries
            .map((entry) => ({ entry, score: scoreSearchEntry(entry, trimmed) }))
            .filter((s): s is { entry: MegaMenuSearchEntry; score: number } => s.score !== null)
            .sort((a, b) => a.score - b.score)
            .slice(0, 8)
            .map((s) => s.entry);
    }, [searchEntries, query]);

    useEffect(() => {
        if (!isOpen) {
            setQuery('');
            setIsAllExpanded(false);
        }
    }, [isOpen]);

    useEffect(() => {
        if (searchFocusToken) inputRef.current?.focus();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchFocusToken]);

    useEffect(() => {
        if (!isOpen) return;
        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') onClose();
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose]);

    if (!isOpen) return null;

    const goTo = (path: string) => {
        navigate(path);
        onClose();
    };

    const handleSearchKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
        if (event.key === 'Enter' && results.length > 0) {
            goTo(results[0].path);
        }
    };

    return (
        <div
            className="absolute left-0 right-0 top-full z-[100] max-h-[70vh] overflow-auto border-b border-slate-700/60 bg-slate-900 opacity-100 shadow-2xl"
            role="region"
            aria-label="All features"
        >
            <div className="container relative mx-auto flex justify-center px-4 pt-5 pb-8">
                <div className="relative mr-8 w-full max-w-xl">
                    <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400">
                        <Icons.Search />
                    </span>
                    <input
                        ref={inputRef}
                        type="text"
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        onKeyDown={handleSearchKeyDown}
                        onMouseEnter={(event) => event.currentTarget.focus()}
                        placeholder="Search actions..."
                        className="w-full rounded-lg border border-slate-700/60 bg-slate-800/70 py-2 pl-10 pr-16 text-sm text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <span
                        className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 font-mono text-[11px] text-slate-500"
                        style={{ letterSpacing: '0.02em' }}
                    >
                        {shortcutKeyLabel()}
                    </span>
                </div>
                <button
                    type="button"
                    onClick={() => setIsAllExpanded((value) => !value)}
                    className="absolute right-4 top-1/2 flex -translate-y-1/2 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium text-slate-400 transition-colors hover:bg-slate-700/60 hover:text-white"
                >
                    <span className={classNames('shrink-0 transition-transform', isAllExpanded && 'rotate-180')}>
                        <Icons.ChevronDown />
                    </span>
                    {isAllExpanded ? 'Minimize' : 'Expand all'}
                </button>
            </div>

            {query.trim() ? (
                <div key="search-results" className="container mx-auto flex justify-center px-4 pb-4">
                    <div className="mr-8 w-full max-w-xl">
                        {results.length === 0 ? (
                            <p className="px-3 py-6 text-center text-sm text-slate-400">No matches for "{query}"</p>
                        ) : (
                            <ul className="grid grid-cols-2 gap-0.5">
                                {results.map((entry, index) => (
                                    <SearchResultRow key={`${entry.breadcrumb}:${entry.path}:${entry.name}`} entry={entry} isFirst={index === 0} goTo={goTo} />
                                ))}
                            </ul>
                        )}
                    </div>
                </div>
            ) : (
                <div key="section-columns" className="container mx-auto flex flex-nowrap gap-6 px-4 py-6">
                    {sections
                        .filter((section) => section.items && section.items.length > 0)
                        .map((section) => (
                            <SectionColumn
                                key={section.key}
                                section={section}
                                goTo={goTo}
                                isHighlighted={Boolean(highlightKey) && section.key === highlightKey}
                                forceExpandAll={isAllExpanded}
                            />
                        ))}
                </div>
            )}
        </div>
    );
};

export default NavMegaMenu;
