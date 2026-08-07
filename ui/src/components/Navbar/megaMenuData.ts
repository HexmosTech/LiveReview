import React from 'react';
import { Icons } from '../UIPrimitives';
import { isCloudMode } from '../../utils/deploymentMode';

export type MegaMenuContext = {
    isSuperAdmin: boolean;
    hasOrg: boolean;
    orgRole?: string;
    isDevMode: boolean;
};

export type MegaMenuLinkNode = {
    kind: 'link';
    name: string;
    icon: React.ReactNode;
    path: string;
    isVisible?: (ctx: MegaMenuContext) => boolean;
    isEnterprise?: boolean;
};

export type MegaMenuGroupNode = {
    kind: 'group';
    name: string;
    icon?: React.ReactNode;
    children: MegaMenuNode[];
    isVisible?: (ctx: MegaMenuContext) => boolean;
    // When set, the group's header is itself a navigable link (e.g. "AI Providers" opening /ai)
    // in addition to being a hover-to-expand trigger for its children.
    path?: string;
};

export type MegaMenuNode = MegaMenuLinkNode | MegaMenuGroupNode;

export type MegaMenuSection = {
    key: string;
    name: string;
    icon: React.ReactNode;
    path: string;
    requiresOwnerOrAdmin?: boolean;
    requiresSuperAdmin?: boolean;
    devOnly?: boolean;
    items?: MegaMenuNode[];
};

const link = (
    name: string,
    icon: React.ReactNode,
    path: string,
    isVisible?: (ctx: MegaMenuContext) => boolean,
    isEnterprise?: boolean
): MegaMenuLinkNode => ({ kind: 'link', name, icon, path, isVisible, isEnterprise });

const group = (
    name: string,
    children: MegaMenuNode[],
    icon?: React.ReactNode,
    isVisible?: (ctx: MegaMenuContext) => boolean,
    path?: string
): MegaMenuGroupNode => ({ kind: 'group', name, icon, children, isVisible, path });

// Filters a node (and its descendants) against the current user's permissions.
// A group with no visible children left disappears entirely.
export const filterMegaMenuNode = (node: MegaMenuNode, ctx: MegaMenuContext): MegaMenuNode | null => {
    if (node.isVisible && !node.isVisible(ctx)) return null;
    if (node.kind === 'link') return node;
    const children = node.children
        .map((child) => filterMegaMenuNode(child, ctx))
        .filter((child): child is MegaMenuNode => child !== null);
    if (children.length === 0) return null;
    return { ...node, children };
};

export const filterMegaMenuSection = (section: MegaMenuSection, ctx: MegaMenuContext): MegaMenuSection => ({
    ...section,
    items: section.items
        ?.map((item) => filterMegaMenuNode(item, ctx))
        .filter((item): item is MegaMenuNode => item !== null),
});

// A single flattened, navigable entry for the mega-menu search — one per link, wherever it sits
// in the tree (top-level section, a flat item, or nested inside a group).
export type MegaMenuSearchEntry = {
    name: string;
    path: string;
    icon: React.ReactNode;
    breadcrumb: string;
    isEnterprise?: boolean;
};

const flattenMegaMenuNode = (node: MegaMenuNode, trail: string[]): MegaMenuSearchEntry[] => {
    if (node.kind === 'link') {
        return [{ name: node.name, path: node.path, icon: node.icon, breadcrumb: trail.join(' > '), isEnterprise: node.isEnterprise }];
    }
    const ownEntry: MegaMenuSearchEntry[] = node.path
        ? [{ name: node.name, path: node.path, icon: node.icon, breadcrumb: trail.join(' > ') }]
        : [];
    return [...ownEntry, ...node.children.flatMap((child) => flattenMegaMenuNode(child, [...trail, node.name]))];
};

export const flattenMegaMenuSections = (sections: MegaMenuSection[]): MegaMenuSearchEntry[] =>
    sections.flatMap((section) => [
        { name: section.name, path: section.path, icon: section.icon, breadcrumb: section.name },
        ...(section.items || []).flatMap((item) => flattenMegaMenuNode(item, [section.name])),
    ]);

const isoDate = (date: Date): string => date.toISOString().slice(0, 10);

// Builds a /reports link scoped to the last N days, computed fresh each time the menu opens.
const reportsRangePath = (days: number): string => {
    const until = new Date();
    const since = new Date();
    since.setDate(since.getDate() - days);
    return `/reports?since=${isoDate(since)}&until=${isoDate(until)}`;
};

// Same 9 provider brands, reused under both "Leader Model" and "Helper Model" — only the ?role=
// query param differs, so a click lands on /ai/{provider} with that role pre-selected.
const buildAiProviderLinks = (role: 'leader' | 'helper'): MegaMenuLinkNode[] => [
    link('Google Gemini', React.createElement(Icons.Google), `/ai/gemini?role=${role}`),
    link('Gemini Enterprise', React.createElement(Icons.Google), `/ai/gemini-enterprise?role=${role}`),
    link('OpenAI', React.createElement(Icons.OpenAI), `/ai/openai?role=${role}`),
    link('Anthropic Claude', React.createElement(Icons.Claude), `/ai/claude?role=${role}`),
    link('AWS Bedrock', React.createElement(Icons.Aws), `/ai/bedrock?role=${role}`),
    link('DeepSeek', React.createElement(Icons.DeepSeek), `/ai/deepseek?role=${role}`),
    link('OpenRouter', React.createElement(Icons.OpenRouter), `/ai/openrouter?role=${role}`),
    link('Ollama', React.createElement(Icons.Ollama), `/ai/ollama?role=${role}`),
    link('Atlas Cloud', React.createElement(Icons.AtlasCloud), `/ai/atlas?role=${role}`),
];

export const buildMegaMenuSections = (): MegaMenuSection[] => [
    {
        key: 'dashboard',
        name: 'Dashboard',
        icon: React.createElement(Icons.Dashboard),
        path: '/dashboard',
    },
    {
        key: 'reviews',
        name: 'Reviews',
        icon: React.createElement(Icons.Reviews),
        path: '/reviews',
        items: [
            link('List Reviews', React.createElement(Icons.List), '/reviews'),
            link('Create Review', React.createElement(Icons.Add), '/reviews/new'),
        ],
    },
    {
        key: 'explore',
        name: 'Explore',
        icon: React.createElement(Icons.FolderOpen),
        path: '/explore/repositories',
        items: [
            link('Repositories', React.createElement(Icons.List), '/explore/repositories'),
            link('Merge Requests', React.createElement(Icons.Reviews), '/explore/merge-requests'),
        ],
    },
    {
        key: 'providers',
        name: 'Providers',
        icon: React.createElement(Icons.Layers),
        path: '/git',
        requiresOwnerOrAdmin: true,
        items: [
            group('Git Providers', [
                group('Connect Git', [
                    link('GitHub', React.createElement(Icons.GitHub), '/git/github/manual'),
                    link('GitLab.com', React.createElement(Icons.GitLab), '/git/gitlab-com/manual'),
                    link('Self-Hosted GitLab', React.createElement(Icons.GitLab), '/git/gitlab-self-hosted/manual'),
                    link('Bitbucket', React.createElement(Icons.Bitbucket), '/git/bitbucket/manual'),
                    link('Gitea', React.createElement(Icons.Gitea), '/git/gitea/manual'),
                    link('Azure DevOps', React.createElement(Icons.AzureDevOps), '/git/azuredevops/manual'),
                ], React.createElement(Icons.Add)),
                link('List Git Hosts', React.createElement(Icons.List), '/git'),
            ], React.createElement(Icons.Git), undefined, '/git'),
            group('AI Providers', [
                group('Leader Model', [
                    group('Connect AI', buildAiProviderLinks('leader'), React.createElement(Icons.Add)),
                    link('List Providers', React.createElement(Icons.List), '/ai?role=leader'),
                ], React.createElement(Icons.AI)),
                group('Helper Model', [
                    group('Connect AI', buildAiProviderLinks('helper'), React.createElement(Icons.Add)),
                    link('List Providers', React.createElement(Icons.List), '/ai?role=helper'),
                ], React.createElement(Icons.AI)),
            ], React.createElement(Icons.AI), undefined, '/ai'),
        ],
    },
    {
        key: 'reports',
        name: 'Reports',
        icon: React.createElement(Icons.Reports),
        path: '/reports',
        requiresOwnerOrAdmin: true,
        items: [
            group('View Summary', [
                link('Last 7 Days', React.createElement(Icons.Clock), reportsRangePath(7)),
                link('Last 30 Days', React.createElement(Icons.Clock), reportsRangePath(30)),
            ], React.createElement(Icons.Dashboard)),
            group('View Findings', [
                link('Critical Issues', React.createElement(Icons.Error), '/reports?mode=explore&severity=Critical'),
                link('Security Findings', React.createElement(Icons.Info), '/reports?mode=explore&category=Security'),
                link('All Findings', React.createElement(Icons.Search), '/reports?mode=explore'),
            ], React.createElement(Icons.List)),
            link('Download Report', React.createElement(Icons.Download), '/reports?export=pdf'),
        ],
    },
    {
        key: 'settings',
        name: 'Settings',
        icon: React.createElement(Icons.Settings),
        path: '/settings',
        items: [
            group('Manage Team', [
                link('View Users', React.createElement(Icons.List), '/settings#users', (ctx) => ctx.isSuperAdmin || ctx.hasOrg),
                link('Invite User', React.createElement(Icons.Add), '/settings/users/add', (ctx) => ctx.isSuperAdmin || ctx.hasOrg),
            ], React.createElement(Icons.User)),
            group('Customize AI', [
                link('Edit Prompts', React.createElement(Icons.Settings), '/settings#prompts', (ctx) => ctx.isSuperAdmin || (ctx.hasOrg && ['owner', 'member'].includes(ctx.orgRole || ''))),
                link('View Learnings', React.createElement(Icons.List), '/settings#learnings', (ctx) => ctx.isSuperAdmin || ctx.hasOrg),
            ], React.createElement(Icons.AI)),
            group('Manage API Access', [
                link('Manage API Keys', React.createElement(Icons.Settings), '/settings#api-keys', (ctx) => ctx.isSuperAdmin || ctx.hasOrg),
                link('Connect MCP', React.createElement(Icons.AI), '/settings#mcp', (ctx) => ctx.isSuperAdmin || ctx.hasOrg),
            ], React.createElement(Icons.Add)),
            group('Connect Integrations', [
                link('Slack', React.createElement(Icons.Slack), '/settings#integrations', (ctx) => ctx.isSuperAdmin || ctx.hasOrg, true),
                link('Microsoft Teams', React.createElement(Icons.Teams), '/settings#integrations', (ctx) => ctx.isSuperAdmin || ctx.hasOrg, true),
                link('Discord', React.createElement(Icons.Discord), '/settings#integrations', (ctx) => ctx.isSuperAdmin || ctx.hasOrg, true),
                link('SMTP', React.createElement(Icons.Email), '/settings#smtp', (ctx) => ctx.isSuperAdmin && !isCloudMode()),
            ], React.createElement(Icons.Grid)),
            group('Manage Billing', [
                link('View License', React.createElement(Icons.Info), '/settings#license', (ctx) => (isCloudMode() ? ctx.isSuperAdmin : ctx.isSuperAdmin || ctx.orgRole === 'owner')),
                link('View Usage', React.createElement(Icons.List), '/settings-subscriptions-overview', (ctx) => isCloudMode() && (ctx.isSuperAdmin || ctx.hasOrg)),
            ], React.createElement(Icons.Reports)),
        ],
    },
    {
        key: 'test-middleware',
        name: 'Test Middleware',
        icon: React.createElement(Icons.Success),
        path: '/test-middleware',
        devOnly: true,
    },
];
