import React, { useMemo, useState } from 'react';
import { LuCheck, LuLayoutGrid, LuList, LuSearch } from 'react-icons/lu';
import { GitHubAppRepository } from '../../api/githubApp';

const MAX_CHIPS_SHOWN = 10;
// Rows visible in the list view before it scrolls (each row is h-9).
const LIST_ROWS_VISIBLE = 5;

type ViewMode = 'grid' | 'list';

const repoName = (fullName: string) => fullName.slice(fullName.indexOf('/') + 1);

const ViewButton: React.FC<{
    active: boolean;
    label: string;
    onClick: () => void;
    children: React.ReactNode;
}> = ({ active, label, onClick, children }) => (
    <button
        type="button"
        aria-label={label}
        aria-pressed={active}
        title={label}
        onClick={onClick}
        className={`inline-flex items-center gap-1 px-3 py-1.5 transition-colors ${
            active ? 'bg-blue-700 text-white' : 'text-slate-300 hover:text-white hover:bg-slate-700'
        }`}
    >
        {active && <LuCheck className="h-3.5 w-3.5" />}
        {children}
    </button>
);

// One GitHub account's repositories: chips by default (the first few, expandable), or a searchable list that
// shows five rows and scrolls. The segmented toggle at the top right switches between the two.
const GitHubAppRepositories: React.FC<{ repos: GitHubAppRepository[] }> = ({ repos }) => {
    const [view, setView] = useState<ViewMode>('grid');
    const [expanded, setExpanded] = useState(false);
    const [query, setQuery] = useState('');

    const filtered = useMemo(() => {
        const q = query.trim().toLowerCase();
        return q ? repos.filter((r) => r.full_name.toLowerCase().includes(q)) : repos;
    }, [repos, query]);

    if (repos.length === 0) {
        return <p className="mt-3 text-sm text-slate-300">No repositories yet.</p>;
    }

    const shownChips = expanded ? repos : repos.slice(0, MAX_CHIPS_SHOWN);

    return (
        <div className="mt-3">
            <div className="flex items-center justify-between gap-3">
                <span className="text-xs font-medium uppercase tracking-wide text-slate-300">Repositories</span>
                <div className="inline-flex overflow-hidden rounded-full border border-slate-600 bg-slate-800 text-xs">
                    <ViewButton active={view === 'list'} label="List view" onClick={() => setView('list')}>
                        <LuList className="h-4 w-4" />
                    </ViewButton>
                    <ViewButton active={view === 'grid'} label="Grid view" onClick={() => setView('grid')}>
                        <LuLayoutGrid className="h-4 w-4" />
                    </ViewButton>
                </div>
            </div>

            {view === 'grid' ? (
                <div className={`mt-2 flex flex-wrap gap-2 ${expanded ? 'max-h-72 overflow-y-auto' : ''}`}>
                    {shownChips.map((r) => (
                        <span
                            key={r.full_name}
                            className="inline-flex items-center rounded-md border border-slate-600 bg-slate-800 px-2 py-1 text-xs text-slate-200 break-all"
                        >
                            {repoName(r.full_name)}
                        </span>
                    ))}
                    {repos.length > MAX_CHIPS_SHOWN && (
                        <button
                            type="button"
                            onClick={() => setExpanded((v) => !v)}
                            className="inline-flex items-center px-2 py-1 text-xs text-blue-400 hover:text-blue-300"
                        >
                            {expanded ? 'Show less' : `and ${repos.length - MAX_CHIPS_SHOWN} more`}
                        </button>
                    )}
                </div>
            ) : (
                <div className="mt-2">
                    <div className="relative">
                        <LuSearch className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                        <input
                            type="text"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            placeholder={`Search ${repos.length} repositories`}
                            aria-label="Search repositories"
                            className="w-full rounded-md border border-slate-600 bg-slate-800 py-2 pl-9 pr-3 text-sm text-white placeholder-slate-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                        />
                    </div>
                    <ul
                        className="mt-2 overflow-y-auto rounded-md border border-slate-600 bg-slate-800 divide-y divide-slate-700"
                        style={{ maxHeight: `${LIST_ROWS_VISIBLE * 2.25}rem` }}
                    >
                        {filtered.map((r) => (
                            <li key={r.full_name} className="flex h-9 items-center justify-between gap-3 px-3 text-sm text-slate-200">
                                <span className="truncate">{repoName(r.full_name)}</span>
                                {r.private && <span className="flex-shrink-0 text-xs text-slate-400">Private</span>}
                            </li>
                        ))}
                        {filtered.length === 0 && (
                            <li className="flex h-9 items-center px-3 text-sm text-slate-300">No repositories match &ldquo;{query.trim()}&rdquo;</li>
                        )}
                    </ul>
                    {query.trim() && filtered.length > 0 && (
                        <p className="mt-1 text-xs text-slate-300">
                            {filtered.length} of {repos.length}
                        </p>
                    )}
                </div>
            )}
        </div>
    );
};

export default GitHubAppRepositories;
