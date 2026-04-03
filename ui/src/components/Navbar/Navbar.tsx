import React, { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { OrganizationSelector } from '../OrganizationSelector';
import { useSystemInfo } from '../../hooks/useSystemInfo';
import { useOrgContext } from '../../hooks/useOrgContext';
import { isCloudMode } from '../../utils/deploymentMode';
import apiClient from '../../api/apiClient';

type NavbarBillingStatusResponse = {
    billing: {
        current_plan_code: string;
        billing_period_end?: string;
        loc_used_month: number;
    };
    available_plans: Array<{
        plan_code: string;
        monthly_loc_limit: number;
    }>;
};

type NavbarQuotaStatusResponse = {
    envelope?: {
        usage_pct?: number;
        blocked?: boolean;
    };
};

type NavbarUpgradeStatusResponse = {
    request: {
        customer_state?: string;
    } | null;
};

type NavbarMyUsageResponse = {
    member?: {
        total_billable_loc?: number;
        operation_count?: number;
        usage_share_percent?: number;
    };
};

type NavbarUsageMembersResponse = {
    members?: Array<{
        actor_email?: string | null;
        actor_kind?: string;
        total_billable_loc?: number;
        usage_share_percent?: number;
    }>;
};

// Upgrade/Plan Badge Component for Navbar
const UpgradeBadge: React.FC = () => {
    const navigate = useNavigate();
    const { currentOrg } = useOrgContext();
    
    // Only show in cloud mode
    if (!isCloudMode()) {
        return null;
    }
    
    // Get plan from current org
    const planType = currentOrg?.plan_type || 'free';
    const isTeamPlan = planType === 'team';
    
    if (isTeamPlan) {
        // Show Team Plan badge that navigates to subscription settings
        return (
            <button
                onClick={() => navigate('/settings-subscriptions-overview')}
                className="relative ml-2 px-4 py-2 bg-gradient-to-r from-amber-600 to-yellow-600 hover:from-amber-500 hover:to-yellow-500 text-white text-sm font-bold rounded-lg transition-all duration-200 shadow-lg hover:shadow-xl flex items-center gap-2"
            >
                <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
                </svg>
                Team Plan
            </button>
        );
    }
    
    // Show Upgrade button for free plan
    return (
        <button
            onClick={() => navigate('/subscribe')}
            className="relative ml-2 px-4 py-2 bg-gradient-to-r from-yellow-500 to-orange-500 hover:from-yellow-400 hover:to-orange-400 text-slate-900 text-sm font-bold rounded-lg transition-all duration-200 shadow-lg hover:shadow-xl transform hover:scale-105 flex items-center gap-2"
        >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            Upgrade
        </button>
    );
};

const planLabel = (planCode: string): string => {
    const normalized = String(planCode || '').trim().toLowerCase();
    if (normalized === 'free_30k' || normalized === 'free') return 'Free 30k';
    if (normalized === 'team_32usd' || normalized === 'team') return 'Team 100k';
    if (normalized === 'loc_200k') return 'Team 200k';
    if (normalized === 'loc_400k') return 'Team 400k';
    if (normalized === 'loc_800k') return 'Team 800k';
    if (normalized === 'loc_1600k') return 'Team 1.6M';
    if (normalized === 'loc_3200k') return 'Team 3.2M';
    return planCode || 'Plan';
};

const formatResetAt = (value?: string): string => {
    const raw = String(value || '').trim();
    if (!raw) return 'Not available';
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return 'Not available';
    return new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
    }).format(date);
};

const BillingChip: React.FC = () => {
    const navigate = useNavigate();
    const { currentOrg, isSuperAdmin } = useOrgContext();
    const [loading, setLoading] = useState(false);
    const [isOpen, setIsOpen] = useState(false);
    const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [chip, setChip] = useState<{
        planCode: string;
        usagePct: number;
        customerState: string;
        blocked: boolean;
        locUsed: number;
        locLimit: number;
        resetAt: string;
        myUsageLoc: number;
        myOperationCount: number;
        mySharePct: number;
        topMembers: Array<{ label: string; loc: number; share: number; kind: string }>;
        canViewTeamBreakdown: boolean;
    } | null>(null);

    useEffect(() => {
        if (!isCloudMode() || !currentOrg?.id) {
            setChip(null);
            return;
        }

        let cancelled = false;
        const load = async () => {
            setLoading(true);
            try {
                const canViewTeamBreakdown = isSuperAdmin || ['owner', 'admin', 'super_admin'].includes(String(currentOrg?.role || '').toLowerCase());
                const [billing, quota, upgrade, myUsage, teamUsage] = await Promise.all([
                    apiClient.get<NavbarBillingStatusResponse>('/billing/status'),
                    apiClient.get<NavbarQuotaStatusResponse>('/quota/status').catch((): null => null),
                    apiClient.get<NavbarUpgradeStatusResponse>('/billing/upgrade/request-status').catch((): null => null),
                    apiClient.get<NavbarMyUsageResponse>('/billing/usage/me').catch((): null => null),
                    canViewTeamBreakdown
                        ? apiClient.get<NavbarUsageMembersResponse>('/billing/usage/members?limit=3&offset=0').catch((): null => null)
                        : Promise.resolve(null),
                ]);

                if (cancelled || !billing?.billing) return;
                const planCode = billing.billing.current_plan_code || 'free_30k';
                const plan = (billing.available_plans || []).find((item) => item.plan_code === planCode);
                const locUsed = Number(billing.billing.loc_used_month || 0);
                const locLimit = Number(plan?.monthly_loc_limit || 0);
                const fallbackPct = plan && plan.monthly_loc_limit > 0
                    ? Math.min(100, Math.round((locUsed * 100) / Number(plan.monthly_loc_limit || 1)))
                    : 0;

                const topMembers = (teamUsage?.members || [])
                    .slice(0, 3)
                    .map((member: NonNullable<NavbarUsageMembersResponse['members']>[number]) => ({
                        label: String(member.actor_email || (member.actor_kind === 'system' ? 'System' : 'Unknown')).trim(),
                        loc: Number(member.total_billable_loc || 0),
                        share: Number(member.usage_share_percent || 0),
                        kind: String(member.actor_kind || 'unknown').trim(),
                    }));

                setChip({
                    planCode,
                    usagePct: Math.max(0, Math.round(quota?.envelope?.usage_pct ?? fallbackPct)),
                    customerState: String(upgrade?.request?.customer_state || 'none').trim().toLowerCase(),
                    blocked: Boolean(quota?.envelope?.blocked),
                    locUsed,
                    locLimit,
                    resetAt: String(billing.billing.billing_period_end || '').trim(),
                    myUsageLoc: Number(myUsage?.member?.total_billable_loc || 0),
                    myOperationCount: Number(myUsage?.member?.operation_count || 0),
                    mySharePct: Number(myUsage?.member?.usage_share_percent || 0),
                    topMembers,
                    canViewTeamBreakdown,
                });
            } catch {
                if (!cancelled) setChip(null);
            } finally {
                if (!cancelled) setLoading(false);
            }
        };

        load();
        return () => {
            cancelled = true;
            if (closeTimerRef.current) {
                clearTimeout(closeTimerRef.current);
                closeTimerRef.current = null;
            }
        };
    }, [currentOrg?.id]);

    const openPopup = () => {
        if (closeTimerRef.current) {
            clearTimeout(closeTimerRef.current);
            closeTimerRef.current = null;
        }
        setIsOpen(true);
    };

    const closePopupSoon = () => {
        if (closeTimerRef.current) {
            clearTimeout(closeTimerRef.current);
        }
        closeTimerRef.current = setTimeout(() => {
            setIsOpen(false);
            closeTimerRef.current = null;
        }, 220);
    };

    if (!isCloudMode() || !currentOrg?.id) return null;

    const toneClass = chip?.blocked || chip?.customerState === 'action_needed' || chip?.customerState === 'payment_failed'
        ? 'bg-red-900/35 border-red-500/50 text-red-100 hover:bg-red-900/50'
        : chip && chip.usagePct >= 80
        ? 'bg-amber-900/35 border-amber-500/50 text-amber-100 hover:bg-amber-900/50'
        : 'bg-emerald-900/25 border-emerald-500/40 text-emerald-100 hover:bg-emerald-900/40';

    return (
        <div
            className="relative ml-2"
            onMouseEnter={openPopup}
            onMouseLeave={closePopupSoon}
        >
            <button
                onClick={() => navigate('/settings-subscriptions-overview')}
                className={classNames(
                    'px-3 py-2 rounded-lg border text-xs font-semibold transition-colors',
                    toneClass,
                )}
                title="Open billing and usage details"
                onFocus={openPopup}
                onBlur={closePopupSoon}
            >
                {loading ? 'Billing...' : chip ? `${planLabel(chip.planCode)} ${chip.usagePct}%` : 'Billing'}
            </button>
            {chip && isOpen && (
                <div className="pointer-events-auto absolute left-1/2 top-full z-50 mt-1 w-80 -translate-x-1/2 rounded-lg border border-slate-600 bg-slate-900/95 p-3 opacity-100 transition-all duration-150">
                    <p className="text-xs text-slate-200 font-semibold">Billing Usage Detail</p>
                    <p className="text-[11px] text-slate-400 mt-1">
                        Scope: organization usage in current billing period. Attribution is charged to the triggering actor.
                    </p>
                    <div className="mt-2 rounded bg-blue-950/40 border border-blue-700/50 p-2 text-[11px]">
                        <p className="text-blue-200 font-medium">Usage resets on {formatResetAt(chip.resetAt)}</p>
                        <p className="text-blue-300/80 mt-0.5">Local timezone. New cycle usage starts immediately after this time.</p>
                    </div>
                    <div className="mt-2 grid grid-cols-2 gap-2 text-[11px]">
                        <div className="rounded bg-slate-800/70 border border-slate-700 p-2">
                            <p className="text-slate-400">Plan</p>
                            <p className="text-slate-100 font-medium">{planLabel(chip.planCode)}</p>
                        </div>
                        <div className="rounded bg-slate-800/70 border border-slate-700 p-2">
                            <p className="text-slate-400">Org Usage</p>
                            <p className="text-slate-100 font-medium">{chip.locUsed.toLocaleString()} / {chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'Unlimited'} LOC</p>
                        </div>
                        <div className="rounded bg-slate-800/70 border border-slate-700 p-2">
                            <p className="text-slate-400">My Usage</p>
                            <p className="text-slate-100 font-medium">{chip.myUsageLoc.toLocaleString()} LOC</p>
                        </div>
                        <div className="rounded bg-slate-800/70 border border-slate-700 p-2">
                            <p className="text-slate-400">My Activity Share</p>
                            {chip.myOperationCount === 0 && chip.myUsageLoc === 0 ? (
                                <p className="text-slate-300">No billable activity this cycle.</p>
                            ) : (
                                <>
                                    <p className="text-slate-100 font-medium">{chip.myOperationCount.toLocaleString()} operations</p>
                                    <p className="text-slate-300">{chip.mySharePct.toFixed(1)}% of org usage</p>
                                </>
                            )}
                        </div>
                    </div>
                    <p className="text-[10px] text-slate-500 mt-2">Operations are billable actions. Share is your LOC contribution percentage out of org usage.</p>
                    {chip.canViewTeamBreakdown && chip.topMembers.length > 0 && (
                        <div className="mt-2 rounded bg-slate-800/70 border border-slate-700 p-2">
                            <p className="text-[11px] text-slate-300 mb-1">Top Contributors</p>
                            <div className="space-y-1 text-[11px]">
                                {chip.topMembers.map((member) => (
                                    <div key={`${member.label}-${member.kind}`} className="flex items-center justify-between text-slate-200">
                                        <span>{member.label}</span>
                                        <span>{member.loc.toLocaleString()} LOC ({member.share.toFixed(1)}%)</span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};

export type NavbarProps = {
    title: string;
    activePage?: string;
    onNavigate?: (page: string) => void;
    onLogout?: () => void;
};

type NavLink = {
    name: string;
    key: string;
    icon: React.ReactNode;
    path?: string;
    requiresOwnerOrAdmin?: boolean;
    requiresSuperAdmin?: boolean;
};

const baseNavLinks: NavLink[] = [
    { name: 'Dashboard', key: 'dashboard', icon: <Icons.Dashboard />, path: '/dashboard' },
    { name: 'Reviews', key: 'reviews', icon: <Icons.Reviews />, path: '/reviews' },
    { name: 'Git Providers', key: 'git', icon: <Icons.Git />, path: '/git', requiresOwnerOrAdmin: true },
    { name: 'AI Providers', key: 'ai', icon: <Icons.AI />, path: '/ai', requiresOwnerOrAdmin: true },
    { name: 'Settings', key: 'settings', icon: <Icons.Settings />, path: '/settings' },
];

const testNavLink: NavLink = {
    name: 'Test Middleware', 
    key: 'test-middleware', 
    icon: (
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
    ),
    path: '/test-middleware',
};

export const Navbar: React.FC<NavbarProps> = ({ title, activePage = 'dashboard', onNavigate, onLogout }) => {
    const [isOpen, setIsOpen] = useState(false);
    const { isDevMode } = useSystemInfo();
    const { isSuperAdmin, currentOrg } = useOrgContext();

    // Check if user can manage current org (owner or super admin)
    const canManageCurrentOrg = isSuperAdmin || currentOrg?.role === 'owner';

    // Filter nav links based on permissions
    const filteredBaseLinks = baseNavLinks.filter(link => {
        if (link.requiresSuperAdmin) {
            return isSuperAdmin;
        }
        if (link.requiresOwnerOrAdmin) {
            return canManageCurrentOrg;
        }
        return true;
    });

    // Conditionally include test middleware link based on dev mode
    const navLinks = isDevMode ? [...filteredBaseLinks, testNavLink] : filteredBaseLinks;

    const handleNavClick = (target: string) => {
        if (onNavigate) onNavigate(target);
        setIsOpen(false);
    };

    return (
        <nav className="bg-slate-900/95 backdrop-blur-sm shadow-lg border-b border-slate-700/60 sticky top-0 z-50">
            <div className="container mx-auto px-4 py-3 flex justify-between items-center">
                <div className="flex items-center">
                    <Link 
                        to="/"
                        onClick={() => handleNavClick('dashboard')}
                        className="cursor-pointer transition-transform hover:scale-105"
                        role="button"
                        aria-label="Go to home"
                    >
                        <img src="assets/logo-horizontal.svg" alt="LiveReview Logo" className="h-10 w-auto mr-3" />
                    </Link>
                </div>
                
                {/* Mobile menu button */}
                <div className="md:hidden">
                    <Button
                        variant="ghost"
                        onClick={() => setIsOpen(!isOpen)}
                        aria-label="Toggle menu"
                        className="text-slate-300"
                        icon={isOpen ? (
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        ) : (
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                            </svg>
                        )}
                    />
                </div>
                
                {/* Desktop menu */}
                <div className="hidden md:flex items-center space-x-2">
                    {/* Organization Selector */}
                    <OrganizationSelector 
                        position="navbar"
                        size="sm"
                        className="mr-4"
                    />
                    
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={() => handleNavClick(link.path || link.key)}
                            icon={link.icon}
                            className={classNames(
                                'text-sm font-medium transition-all duration-200',
                                activePage === link.key 
                                    ? 'bg-blue-600 text-white shadow-lg' 
                                    : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                            )}
                            as={Link}
                            to={link.path || `/${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    <BillingChip />
                    
                    {/* Upgrade / Manage Licenses */}
                    <UpgradeBadge />
                    
                    {/* Logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="ml-3 text-slate-300 hover:text-red-300 hover:bg-red-900/20 transition-colors"
                            icon={<svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                            </svg>}
                        >
                            Logout
                        </Button>
                    )}
                </div>
            </div>
            
            {/* Mobile menu dropdown */}
            {isOpen && (
                <div className="md:hidden px-4 py-3 space-y-2 bg-slate-800/95 border-t border-slate-700/60 backdrop-blur-sm">
                    {/* Mobile Organization Selector */}
                    <div className="mb-3">
                        <OrganizationSelector 
                            position="sidebar"
                            size="sm"
                        />
                    </div>
                    <div className="mb-3">
                        <BillingChip />
                    </div>
                    
                    {navLinks.map(link => (
                        <Button
                            key={link.key}
                            variant={activePage === link.key ? 'primary' : 'ghost'}
                            onClick={() => handleNavClick(link.path || link.key)}
                            icon={link.icon}
                            className={classNames(
                                'w-full justify-start text-sm font-medium',
                                activePage === link.key 
                                    ? 'bg-blue-600 text-white' 
                                    : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                            )}
                            iconPosition="left"
                            as={Link}
                            to={link.path || `/${link.key}`}
                        >
                            {link.name}
                        </Button>
                    ))}
                    
                    {/* Mobile Upgrade/Manage Licenses */}
                    <div className="pt-2 border-t border-slate-700">
                        <UpgradeBadge />
                    </div>
                    
                    {/* Mobile logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="w-full justify-start text-slate-300 hover:text-red-300 hover:bg-red-900/20"
                            iconPosition="left"
                            icon={<svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                            </svg>}
                        >
                            Logout
                        </Button>
                    )}
                </div>
            )}
        </nav>
    );
};
