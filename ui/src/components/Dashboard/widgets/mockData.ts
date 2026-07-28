// Mock data for the customizable dashboard widget grid.
// This is a pure front-end design-iteration pass — nothing here calls a real API.
// All numbers are generated from a seeded PRNG so the dashboard looks "alive"
// (non-round numbers) but is stable across reloads within the same build.

function mulberry32(seed: number) {
    let a = seed;
    return function random() {
        a |= 0;
        a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

const rand = mulberry32(0xC0DE1234);
const randInt = (min: number, max: number): number => Math.floor(rand() * (max - min + 1)) + min;
const randFloat = (min: number, max: number, decimals = 1): number => {
    const value = rand() * (max - min) + min;
    const factor = 10 ** decimals;
    return Math.round(value * factor) / factor;
};
// The real LiveReview taxonomy — the 10 top-level categories the AI reviewer
// classifies every finding into (source of truth: TaxonomyClassificationRules
// in internal/prompts/templates.go, injected into the live review prompt).
// "Best Practice" is not a category here — it's a value of the separate
// `type` field (Bug | Risk | Optimization | Code Smell | Best Practice |
// Technical Debt), and "Style" isn't part of the taxonomy at all.
export const ISSUE_CATEGORIES = [
    'Security',
    'Reliability',
    'Correctness',
    'Performance',
    'Cost',
    'Scalability',
    'Maintainability',
    'Architecture',
    'Developer Experience',
    'Compliance & Governance',
] as const;
export type IssueCategory = typeof ISSUE_CATEGORIES[number];

export const CATEGORY_COLORS: Record<IssueCategory, string> = {
    'Security': '#F43F5E',
    'Reliability': '#06B6D4',
    'Correctness': '#3B82F6',
    'Performance': '#F59E0B',
    'Cost': '#22C55E',
    'Scalability': '#A855F7',
    'Maintainability': '#7C3AED',
    'Architecture': '#EC4899',
    'Developer Experience': '#14B8A6',
    'Compliance & Governance': '#F97316',
};

// ---------------------------------------------------------------------------
// Contributors — reused across the leaderboard, calendar heatmap, and usage donut
// so the same people show up consistently everywhere.
// ---------------------------------------------------------------------------

export interface ContributorMock {
    id: string;
    name: string;
    initials: string;
    color: string;
    reviewsAuthored: number;
    reviewsGiven: number;
    locReviewed: number;
}

const CONTRIBUTOR_NAMES = [
    'Priya Sharma', 'Alex Chen', 'Marcus Webb', 'Fatima Al-Sayed', 'Diego Ramirez',
    'Yuki Tanaka', 'Sarah O’Connor', 'Ravi Patel', 'Elena Popescu', 'Jordan Lee',
    'Amara Okafor', 'Tomas Novak',
];

const CONTRIBUTOR_COLORS = ['#3B82F6', '#7C3AED', '#22C55E', '#F59E0B', '#F43F5E', '#06B6D4', '#A855F7'];

const initialsOf = (name: string): string =>
    name.split(/\s+/).filter(Boolean).map((part) => part[0]).slice(0, 2).join('').toUpperCase();

export const MOCK_CONTRIBUTORS: ContributorMock[] = CONTRIBUTOR_NAMES.map((name, index) => ({
    id: `contributor-${index}`,
    name,
    initials: initialsOf(name),
    color: CONTRIBUTOR_COLORS[index % CONTRIBUTOR_COLORS.length],
    reviewsAuthored: randInt(12, 165),
    reviewsGiven: randInt(8, 140),
    locReviewed: randInt(4200, 68000),
})).sort((a, b) => b.reviewsGiven - a.reviewsGiven);

// ---------------------------------------------------------------------------
// Git host -> repo -> PR volume hierarchy — reused by the sunburst/treemap and
// to derive system KPIs + connector health so the numbers agree everywhere.
// ---------------------------------------------------------------------------

export interface RepoMock {
    name: string;
    prCount: number;
    coveragePct: number;
}

export interface GitHostMock {
    name: string;
    repos: RepoMock[];
}

const REPO_NAMES_BY_HOST: Record<string, string[]> = {
    GitHub: ['api-gateway', 'web-frontend', 'billing-service', 'mobile-app', 'infra-terraform'],
    GitLab: ['payments-core', 'notification-svc', 'data-pipeline', 'design-system'],
    Bitbucket: ['legacy-monolith', 'reporting-engine', 'internal-tools'],
};

export const MOCK_REPO_TREE: GitHostMock[] = Object.entries(REPO_NAMES_BY_HOST).map(([hostName, repoNames]) => ({
    name: hostName,
    repos: repoNames.map((repoName) => ({
        name: repoName,
        prCount: randInt(18, 340),
        coveragePct: randInt(58, 99),
    })),
}));

// ---------------------------------------------------------------------------
// Review pipeline layers — Pre-commit / API-MCP / MR-PR / Scheduled.
// Volumes are hand-tuned to funnel down realistically.
// ---------------------------------------------------------------------------

export interface ReviewLayerMock {
    id: 'precommit' | 'api-mcp' | 'mr-pr' | 'scheduled';
    label: string;
    reviewsRun: number;
    issuesFound: number;
    trendPct: number;
    categories: { category: IssueCategory; count: number }[];
}

const buildCategorySplit = (total: number): { category: IssueCategory; count: number }[] => {
    const weights = ISSUE_CATEGORIES.map(() => randFloat(0.6, 1.6, 2));
    const weightSum = weights.reduce((sum, w) => sum + w, 0);
    let remaining = total;
    return ISSUE_CATEGORIES.map((category, index) => {
        const isLast = index === ISSUE_CATEGORIES.length - 1;
        const count = isLast ? remaining : Math.round((weights[index] / weightSum) * total);
        remaining -= count;
        return { category, count: Math.max(0, count) };
    });
};

const LAYER_SPECS: { id: ReviewLayerMock['id']; label: string; reviewsRun: number; issueRate: number }[] = [
    { id: 'precommit', label: 'Pre-commit', reviewsRun: 2438, issueRate: 0.22 },
    { id: 'mr-pr', label: 'MR / PR', reviewsRun: 947, issueRate: 0.41 },
    { id: 'api-mcp', label: 'API / MCP', reviewsRun: 416, issueRate: 0.33 },
    { id: 'scheduled', label: 'Scheduled', reviewsRun: 183, issueRate: 0.28 },
];

export const MOCK_REVIEW_LAYERS: ReviewLayerMock[] = LAYER_SPECS.map((spec) => {
    const issuesFound = Math.round(spec.reviewsRun * spec.issueRate);
    return {
        id: spec.id,
        label: spec.label,
        reviewsRun: spec.reviewsRun,
        issuesFound,
        trendPct: randFloat(-8, 18, 1),
        categories: buildCategorySplit(issuesFound),
    };
});

// ---------------------------------------------------------------------------
// System-level KPIs, derived from the repo tree so numbers agree across widgets.
// ---------------------------------------------------------------------------

const totalRepos = MOCK_REPO_TREE.reduce((sum, host) => sum + host.repos.length, 0);
const totalPRs = MOCK_REPO_TREE.reduce((sum, host) => sum + host.repos.reduce((s, r) => s + r.prCount, 0), 0);

export const MOCK_SYSTEM_KPIS = {
    gitHosts: MOCK_REPO_TREE.length,
    aiConnectors: 4,
    totalRepos,
    totalPRs,
    avgReviewsPerPR: randFloat(1.8, 3.6, 1),
    avgReviewsPerCommit: randFloat(0.9, 1.6, 2),
    coverageScorePct: Math.round(MOCK_REPO_TREE.flatMap((h) => h.repos).reduce((sum, r) => sum + r.coveragePct, 0) / totalRepos),
};

// ---------------------------------------------------------------------------
// Connected providers — git hosts + AI providers. Most orgs connect a
// small handful of these (2-3 git hosts, a couple of AI backends), so this
// is intentionally a short list, not a big "fleet health" board — and there's
// no real "health score" concept in the product, just "is it connected".
// ---------------------------------------------------------------------------

export interface ConnectedProviderMock {
    id: string;
    name: string;
    kind: 'git' | 'ai';
    iconKey: string;
    lastSyncMinutesAgo: number;
}

export const MOCK_CONNECTED_PROVIDERS: ConnectedProviderMock[] = [
    ...MOCK_REPO_TREE.map((host, index) => ({
        id: `git-${index}`,
        name: host.name,
        kind: 'git' as const,
        iconKey: host.name,
        lastSyncMinutesAgo: randInt(1, 240),
    })),
    { id: 'ai-openai', name: 'OpenAI', kind: 'ai' as const, iconKey: 'OpenAI', lastSyncMinutesAgo: randInt(1, 240) },
    { id: 'ai-claude', name: 'Anthropic Claude', kind: 'ai' as const, iconKey: 'Claude', lastSyncMinutesAgo: randInt(1, 240) },
    { id: 'ai-ollama', name: 'Ollama', kind: 'ai' as const, iconKey: 'Ollama', lastSyncMinutesAgo: randInt(1, 240) },
];

// ---------------------------------------------------------------------------
// Contribution activity calendar — ~7 months of daily review activity.
// ---------------------------------------------------------------------------

export interface CalendarDayMock {
    date: string;
    count: number;
}

const toISODate = (date: Date): string => date.toISOString().slice(0, 10);

const buildCalendarActivity = (): CalendarDayMock[] => {
    const end = new Date('2026-07-28T00:00:00Z');
    const start = new Date('2026-01-01T00:00:00Z');
    const days: CalendarDayMock[] = [];
    for (let time = start.getTime(); time <= end.getTime(); time += 24 * 60 * 60 * 1000) {
        const date = new Date(time);
        const dayOfWeek = date.getUTCDay();
        const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;
        const base = isWeekend ? randInt(0, 6) : randInt(2, 24);
        // gentle upward trend over the period so the heatmap doesn't look flat
        const progress = (time - start.getTime()) / (end.getTime() - start.getTime());
        const boosted = Math.round(base * (0.7 + progress * 0.6));
        days.push({ date: toISODate(date), count: boosted });
    }
    return days;
};

export const MOCK_CALENDAR_ACTIVITY: CalendarDayMock[] = buildCalendarActivity();
