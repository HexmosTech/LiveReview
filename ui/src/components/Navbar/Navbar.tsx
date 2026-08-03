import React, { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { OrganizationSelector } from '../OrganizationSelector';
import { useSystemInfo } from '../../hooks/useSystemInfo';
import { useOrgContext } from '../../hooks/useOrgContext';
import { isCloudMode } from '../../utils/deploymentMode';
import apiClient from '../../api/apiClient';
import { useAppSelector } from '../../store/configureStore';
import { buildMegaMenuSections, filterMegaMenuSection, MegaMenuContext } from './megaMenuData';
import { NavMegaMenu } from './NavMegaMenu';
import { shortcutKeyLabel } from '../../utils/platform';

type NavbarBillingStatusResponse = {
    billing: {
        current_plan_code: string;
        billing_period_end?: string;
        loc_used_month: number;
        trial_active?: boolean;
        trial_ends_at?: string | null;
        trial_can_cancel?: boolean;
        trial_eligibility?: {
            status?: 'eligible' | 'already_used' | 'reserved' | 'unknown';
            eligible?: boolean;
            reason?: string;
            consumed_at?: string | null;
        };
    };
    available_plans: Array<{
        plan_code: string;
        monthly_loc_limit: number;
        trial_days?: number;
    }>;
};

type NavbarQuotaStatusResponse = {
    plan_type?: string;
    envelope?: {
        usage_pct?: number;
        blocked?: boolean;
        loc_used_month?: number;
        loc_limit_month?: number;
        billing_period_end?: string;
        reset_at?: string;
        plan_code?: string;
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

const normalizePlanCode = (planCode?: string | null): string => {
    return String(planCode || '').trim().toLowerCase();
};

const isFreeLOCPlan = (planCode?: string | null): boolean => {
    const normalized = normalizePlanCode(planCode);
    return normalized === 'free' || normalized === 'free_30k';
};

const isLicensedSelfHostedStatus = (status?: string): boolean => {
    return ['active', 'warning', 'grace'].includes(normalizePlanCode(status));
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

const trialDaysRemaining = (value?: string | null): number | null => {
    const raw = String(value || '').trim();
    if (!raw) return null;
    const end = new Date(raw);
    if (Number.isNaN(end.getTime())) return null;
    const diffMs = end.getTime() - Date.now();
    if (diffMs <= 0) return 0;
    return Math.max(1, Math.ceil(diffMs / (24 * 60 * 60 * 1000)));
};

const formatTrialEndsAt = (value?: string | null): string => {
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
    const license = useAppSelector((state) => state.License);
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
        trialActive: boolean;
        trialEndsAt: string;
        trialDaysLeft: number | null;
        trialCanCancel: boolean;
        trialEligibleForFirstPaidPurchase: boolean;
        trialEligibilityStatus: string;
        trialPolicyDays: number;
        isFreePlan: boolean;
    } | null>(null);

    useEffect(() => {
        if (!currentOrg?.id) {
            setChip(null);
            return;
        }
        if (!isCloudMode() && license.loadedOnce && isLicensedSelfHostedStatus(license.status)) {
            setChip(null);
            return;
        }

        let cancelled = false;
        const load = async () => {
            setLoading(true);
            try {
                const canViewTeamBreakdown = isSuperAdmin || ['owner', 'admin', 'super_admin'].includes(String(currentOrg?.role || '').toLowerCase());

                if (!isCloudMode()) {
                    if (license.loadedOnce && isLicensedSelfHostedStatus(license.status)) {
                        setChip(null);
                        return;
                    }

                    const [quota, myUsage] = await Promise.all([
                        apiClient.get<NavbarQuotaStatusResponse>('/quota/status').catch((): null => null),
                        apiClient.get<NavbarMyUsageResponse>('/billing/usage/me').catch((): null => null),
                    ]);

                    if (cancelled) return;

                    if (!quota?.envelope) {
                        setChip(null);
                        return;
                    }

                    const planCode = license.loadedOnce && ['missing', 'invalid', 'expired'].includes(license.status)
                        ? 'free_30k'
                        : quota.envelope.plan_code || quota.plan_type || currentOrg?.plan_type || '';
                    const locLimit = quota.envelope.loc_limit_month ?? 30000;

                    if (!isFreeLOCPlan(planCode) && locLimit !== 30000) {
                        setChip(null);
                        return;
                    }

                    const locUsed = quota.envelope.loc_used_month ?? 0;
                    const usagePct = locLimit > 0 ? Math.min(100, Math.round((locUsed * 100) / locLimit)) : 0;
                    setChip({
                        planCode: 'free_30k',
                        usagePct,
                        customerState: 'none',
                        blocked: Boolean(quota.envelope.blocked),
                        locUsed,
                        locLimit,
                        resetAt: quota.envelope.billing_period_end ?? quota.envelope.reset_at ?? '',
                        myUsageLoc: Number(myUsage?.member?.total_billable_loc || 0),
                        myOperationCount: Number(myUsage?.member?.operation_count || 0),
                        mySharePct: Number(myUsage?.member?.usage_share_percent || 0),
                        topMembers: [],
                        canViewTeamBreakdown: false,
                        trialActive: false,
                        trialEndsAt: '',
                        trialDaysLeft: null,
                        trialCanCancel: false,
                        trialEligibleForFirstPaidPurchase: false,
                        trialEligibilityStatus: 'unknown',
                        trialPolicyDays: 7,
                        isFreePlan: true,
                    });
                    return;
                }

                const [billing, quota, upgrade, myUsage, teamUsage] = await Promise.all([
                    apiClient.get<NavbarBillingStatusResponse>('/billing/status'),
                    apiClient.get<NavbarQuotaStatusResponse>('/quota/status').catch((): null => null),
                    apiClient.get<NavbarUpgradeStatusResponse>('/billing/upgrade/request-status').catch((): null => null),
                    apiClient.get<NavbarMyUsageResponse>('/billing/usage/me').catch((): null => null),
                    canViewTeamBreakdown
                        ? apiClient.get<NavbarUsageMembersResponse>('/billing/usage/members?limit=3&offset=0').catch((): null => null)
                        : Promise.resolve(null),
                ]);

                if (cancelled) return;

                if (!billing?.billing) {
                    setChip(null);
                    return;
                }

                const planCode = billing.billing.current_plan_code || 'free_30k';
                const availablePlans: NavbarBillingStatusResponse['available_plans'] = billing.available_plans || [];
                const plan = availablePlans.find((item) => item.plan_code === planCode);
                const locUsed = Number(billing.billing.loc_used_month || quota?.envelope?.loc_used_month || 0);
                const locLimit = Number(plan?.monthly_loc_limit || quota?.envelope?.loc_limit_month || 0);

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
                const trialPolicyDays = availablePlans.reduce((max: number, item) => {
                    const trialDays = Number(item.trial_days || 0);
                    if (trialDays <= 0) {
                        return max;
                    }
                    return Math.max(max, trialDays);
                }, 0);
                const trialEligibilityStatus = String(billing.billing.trial_eligibility?.status || 'unknown').trim().toLowerCase();
                const trialEligibleForFirstPaidPurchase = Boolean(billing.billing.trial_eligibility?.eligible);
                const isFreePlan = String(planCode).trim().toLowerCase() === 'free_30k' || String(planCode).trim().toLowerCase() === 'free';

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
                    trialActive: Boolean(billing.billing.trial_active),
                    trialEndsAt: String(billing.billing.trial_ends_at || '').trim(),
                    trialDaysLeft: trialDaysRemaining(billing.billing.trial_ends_at),
                    trialCanCancel: Boolean(billing.billing.trial_can_cancel),
                    trialEligibleForFirstPaidPurchase,
                    trialEligibilityStatus,
                    trialPolicyDays: trialPolicyDays > 0 ? trialPolicyDays : 7,
                    isFreePlan,
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
    }, [currentOrg?.id, currentOrg?.plan_type, license.loadedOnce, license.status]);

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

    if (!currentOrg?.id || (!isCloudMode() && !chip)) return null;

    const toneClass = chip?.blocked || chip?.customerState === 'action_needed' || chip?.customerState === 'payment_failed'
        ? 'bg-red-900/35 border-red-500/50 text-red-100 hover:bg-red-900/50'
        : chip?.trialActive
            ? 'bg-sky-900/35 border-sky-500/50 text-sky-100 hover:bg-sky-900/50'
        : chip && chip.usagePct >= 80
            ? 'bg-amber-900/35 border-amber-500/50 text-amber-100 hover:bg-amber-900/50'
            : 'bg-emerald-900/25 border-emerald-500/40 text-emerald-100 hover:bg-emerald-900/40';
    const showFirstPaidTrialBadge = isCloudMode() && Boolean(chip && !chip.trialActive && chip.isFreePlan && chip.trialPolicyDays > 0);
    const firstPaidTrialBadgeText = chip?.trialEligibleForFirstPaidPurchase
        ? `Free ${chip.trialPolicyDays}-Day Trial Included`
        : chip?.trialEligibilityStatus === 'already_used'
            ? 'Trial already used'
            : chip?.trialEligibilityStatus === 'reserved'
                ? 'Trial reservation in progress'
                : `Up to ${chip?.trialPolicyDays || 7}-day trial`;
    const firstPaidTrialBadgeClass = chip?.trialEligibleForFirstPaidPurchase
        ? 'border-sky-400/50 bg-sky-900/35 text-sky-100'
        : chip?.trialEligibilityStatus === 'already_used'
            ? 'border-slate-600 bg-slate-800 text-slate-300'
            : 'border-amber-400/50 bg-amber-900/30 text-amber-100';

    return (
        <div
            className="relative ml-2"
            onMouseEnter={openPopup}
            onMouseLeave={closePopupSoon}
        >
            <button
                onClick={() => { if (isCloudMode()) navigate('/settings-subscriptions-overview') }}
                className={classNames(
                    'whitespace-nowrap px-2 py-1.5 2xl:px-3 2xl:py-2 rounded-lg border text-xs font-semibold transition-colors',
                    toneClass,
                    isCloudMode() ? 'cursor-pointer' : 'cursor-default'
                )}
                title="Open billing and usage details"
                onFocus={openPopup}
                onBlur={closePopupSoon}
            >
                {loading ? 'Billing...' : chip ? (chip.trialActive ? `Trial ${chip.trialDaysLeft ?? '?'}d left` : `${planLabel(chip.planCode)} ${chip.usagePct}%`) : 'Billing'}
            </button>
            {chip && isOpen && (
                <div className="pointer-events-auto absolute left-1/2 top-full z-[110] mt-1 w-80 -translate-x-1/2 rounded-lg border border-slate-600 bg-slate-900/95 p-3 opacity-100 transition-all duration-150">
                    <p className="text-xs text-slate-200 font-semibold">Billing Usage Detail</p>
                    <p className="text-[11px] text-slate-400 mt-1">
                        Scope: organization usage in current billing period. Attribution is charged to the triggering actor.
                    </p>
                    {chip.trialActive && (
                        <div className="mt-2 rounded bg-sky-950/50 border border-sky-500/50 p-2 text-[11px]">
                            <p className="text-sky-200 font-semibold">
                                Trial active {typeof chip.trialDaysLeft === 'number' ? `- ${chip.trialDaysLeft} day${chip.trialDaysLeft === 1 ? '' : 's'} left` : ''}
                            </p>
                            <p className="text-sky-300/90 mt-0.5">Ends on {formatTrialEndsAt(chip.trialEndsAt)}</p>
                            <button
                                onClick={(e) => {
                                    e.stopPropagation();
                                    navigate('/settings-subscriptions-assign');
                                }}
                                className="mt-1 w-full py-1 bg-sky-600 hover:bg-sky-500 rounded text-sky-50 font-semibold transition-colors"
                            >
                                {chip.trialCanCancel ? 'Cancel Trial Now' : 'Open Trial Details'}
                            </button>
                        </div>
                    )}
                    {/* LOC Warning / Blocked Banner */}
                    {chip.blocked && (
                        <div className="mt-2 rounded bg-red-950/60 border border-red-500/60 p-2 text-[11px]">
                            <p className="text-red-200 font-semibold">⛔ Monthly LOC Quota Exceeded</p>
                            <p className="text-red-300/90 mt-0.5">
                                You've used {chip.locUsed.toLocaleString()} of {chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A'} LOC. Reviews are blocked until quota resets.
                            </p>
                            <div className="mt-2 flex items-center gap-2">
                                {isCloudMode() && (
                                    <button
                                        onClick={(e) => { e.stopPropagation(); navigate('/settings-subscriptions-overview'); }}
                                        className="flex-1 py-1 bg-red-600 hover:bg-red-500 rounded text-red-50 font-semibold transition-colors"
                                    >
                                        Upgrade Now
                                    </button>
                                )}
                                {showFirstPaidTrialBadge && (
                                    <span className={`inline-flex items-center rounded border px-2 py-1 text-[10px] font-semibold uppercase tracking-wide ${firstPaidTrialBadgeClass}`}>
                                        {firstPaidTrialBadgeText}
                                    </span>
                                )}
                            </div>
                        </div>
                    )}
                    {!chip.blocked && chip.usagePct >= 100 && (
                        <div className="mt-2 rounded bg-amber-950/50 border border-amber-500/50 p-2 text-[11px]">
                            <p className="text-amber-200 font-semibold">LOC Usage Exhausted</p>
                            <p className="text-amber-300/90 mt-0.5">
                                You've used {chip.locUsed.toLocaleString()} of {chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A'} LOC ({chip.usagePct}%). Please upgrade to continue.
                            </p>
                            <div className="mt-1 flex items-center gap-2">
                                {isCloudMode() && (
                                    <button
                                        onClick={(e) => { e.stopPropagation(); navigate('/settings-subscriptions-overview'); }}
                                        className="flex-1 py-1 bg-amber-600 hover:bg-amber-500 rounded text-amber-50 font-semibold transition-colors"
                                    >
                                        Upgrade Plan
                                    </button>
                                )}
                                {showFirstPaidTrialBadge && (
                                    <span className={`inline-flex items-center rounded border px-2 py-1 text-[10px] font-semibold uppercase tracking-wide ${firstPaidTrialBadgeClass}`}>
                                        {firstPaidTrialBadgeText}
                                    </span>
                                )}
                            </div>
                        </div>
                    )}
                    {!chip.blocked && chip.usagePct >= 90 && chip.usagePct < 100 && (
                        <div className="mt-2 rounded bg-amber-950/50 border border-amber-500/50 p-2 text-[11px]">
                            <p className="text-amber-200 font-semibold">⚠️ LOC Usage Nearing Limit</p>
                            <p className="text-amber-300/90 mt-0.5">
                                You've used {chip.locUsed.toLocaleString()} of {chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A'} LOC ({chip.usagePct}%). Upgrade to avoid interruption.
                            </p>
                            <div className="mt-1 flex items-center gap-2">
                                {isCloudMode() && (
                                    <button
                                        onClick={(e) => { e.stopPropagation(); navigate('/settings-subscriptions-overview'); }}
                                        className="flex-1 py-1 bg-amber-600 hover:bg-amber-500 rounded text-amber-50 font-semibold transition-colors"
                                    >
                                        Upgrade Plan
                                    </button>
                                )}
                                {showFirstPaidTrialBadge && (
                                    <span className={`inline-flex items-center rounded border px-2 py-1 text-[10px] font-semibold uppercase tracking-wide ${firstPaidTrialBadgeClass}`}>
                                        {firstPaidTrialBadgeText}
                                    </span>
                                )}
                            </div>
                        </div>
                    )}
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
                    {!chip.blocked && chip.usagePct < 90 && (
                        <div className="mt-3 flex items-center gap-2">
                            {isCloudMode() && (
                                <button
                                    onClick={(e) => { e.stopPropagation(); navigate('/settings-subscriptions-overview'); }}
                                    className="flex-1 py-1.5 bg-blue-600 hover:bg-blue-500 rounded text-white text-xs font-semibold transition-colors shadow-sm"
                                >
                                    Upgrade Plan
                                </button>
                            )}
                            {showFirstPaidTrialBadge && (
                                <span className={`inline-flex items-center rounded border px-2 py-1 text-[10px] font-semibold uppercase tracking-wide ${firstPaidTrialBadgeClass}`}>
                                    {firstPaidTrialBadgeText}
                                </span>
                            )}
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
    { name: 'Explore', key: 'explore', icon: <Icons.FolderOpen />, path: '/explore/repositories' },
    { name: 'Providers', key: 'providers', icon: <Icons.Layers />, requiresOwnerOrAdmin: true },
    { name: 'Reports', key: 'reports', icon: <Icons.Reports />, path: '/reports', requiresOwnerOrAdmin: true },
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
    const [isMegaMenuOpen, setIsMegaMenuOpen] = useState(false);
    const [hoveredNavKey, setHoveredNavKey] = useState<string | null>(null);
    const [searchFocusToken, setSearchFocusToken] = useState(0);
    const megaMenuCloseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    // The mega menu should only open/stay open while hovering the logo or the nav-links-through-
    // search cluster (or the open panel itself) — not the org selector, billing chip, or logout,
    // which now sit to the right of Search but shouldn't trigger it.
    const logoRef = useRef<HTMLDivElement>(null);
    const navTriggerZoneRef = useRef<HTMLDivElement>(null);
    const megaMenuPanelRef = useRef<HTMLDivElement>(null);
    const wasMegaMenuOpenRef = useRef(false);
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

    const megaMenuContext: MegaMenuContext = {
        isSuperAdmin,
        hasOrg: Boolean(currentOrg),
        orgRole: currentOrg?.role,
        isDevMode,
    };

    const filteredMegaMenuSections = buildMegaMenuSections()
        .filter(section => {
            if (section.devOnly) return isDevMode;
            if (section.requiresSuperAdmin) return isSuperAdmin;
            if (section.requiresOwnerOrAdmin) return canManageCurrentOrg;
            return true;
        })
        .map(section => filterMegaMenuSection(section, megaMenuContext));

    const handleNavClick = (target: string) => {
        if (onNavigate) onNavigate(target);
        setIsOpen(false);
        setIsMegaMenuOpen(false);
    };

    const openMegaMenu = () => {
        if (megaMenuCloseTimerRef.current) {
            clearTimeout(megaMenuCloseTimerRef.current);
            megaMenuCloseTimerRef.current = null;
        }
        wasMegaMenuOpenRef.current = true;
        setIsMegaMenuOpen(true);
    };

    // Whether the cursor is "inside" the nav can't be tracked with plain onMouseEnter/onMouseLeave
    // on <nav>: the mega-menu panel resizes as you type in search (results list grows/shrinks),
    // and that alone can shift the panel's edge out from under a stationary cursor and fire a
    // spurious mouseleave — closing the menu while you're still actively using it. Tracking real
    // mouse movement globally and checking actual DOM containment (via elementFromPoint, which
    // correctly includes the absolutely-positioned panel since it's still a real DOM descendant
    // of <nav>) only reacts to the cursor actually moving, so it isn't fooled by layout shifts.
    useEffect(() => {
        const handleMouseMove = (event: MouseEvent) => {
            const elementUnderCursor = document.elementFromPoint(event.clientX, event.clientY);
            const isInsideTriggerZone = Boolean(
                elementUnderCursor && (
                    logoRef.current?.contains(elementUnderCursor) ||
                    navTriggerZoneRef.current?.contains(elementUnderCursor) ||
                    megaMenuPanelRef.current?.contains(elementUnderCursor)
                )
            );

            if (isInsideTriggerZone) {
                if (megaMenuCloseTimerRef.current) {
                    clearTimeout(megaMenuCloseTimerRef.current);
                    megaMenuCloseTimerRef.current = null;
                }
                // Focus the search box the moment the menu actually opens (not on every mousemove
                // tick while it's already open, which would fight the user for focus while typing).
                if (!wasMegaMenuOpenRef.current) {
                    wasMegaMenuOpenRef.current = true;
                    setSearchFocusToken((token) => token + 1);
                }
                setIsMegaMenuOpen(true);
            } else if (!megaMenuCloseTimerRef.current) {
                megaMenuCloseTimerRef.current = setTimeout(() => {
                    setIsMegaMenuOpen(false);
                    setHoveredNavKey(null);
                    wasMegaMenuOpenRef.current = false;
                    megaMenuCloseTimerRef.current = null;
                }, 200);
            }
        };
        document.addEventListener('mousemove', handleMouseMove);
        return () => document.removeEventListener('mousemove', handleMouseMove);
    }, []);

    // Ctrl+K / Cmd+K opens the mega menu and focuses its search box from anywhere in the app.
    useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
                event.preventDefault();
                if (megaMenuCloseTimerRef.current) {
                    clearTimeout(megaMenuCloseTimerRef.current);
                    megaMenuCloseTimerRef.current = null;
                }
                wasMegaMenuOpenRef.current = true;
                setIsMegaMenuOpen(true);
                setSearchFocusToken((token) => token + 1);
            }
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, []);

    return (
        <nav
            className="relative bg-slate-900/95 backdrop-blur-sm shadow-lg border-b border-slate-700/60 sticky top-0 z-50"
        >
            <div className="container mx-auto px-4 py-3 flex justify-between items-center">
                <div ref={logoRef} className="flex items-center">
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
                <div className="hidden md:flex items-center space-x-1 2xl:space-x-2">
                    <div ref={navTriggerZoneRef} className="flex items-center space-x-1 2xl:space-x-2">
                        <div
                            className={classNames(
                                'flex items-center gap-1 2xl:gap-2 transition-opacity duration-200',
                                isMegaMenuOpen && 'opacity-50'
                            )}
                            onMouseLeave={() => setHoveredNavKey(null)}
                        >
                        {navLinks.map(link => {
                            const linkProps = link.path ? { as: Link, to: link.path } : {};
                            return (
                                <Button
                                    key={link.key}
                                    variant="ghost"
                                    onClick={link.path ? () => handleNavClick(link.path as string) : undefined}
                                    onMouseEnter={() => setHoveredNavKey(link.key)}
                                    icon={activePage === link.key ? link.icon : <span className="text-slate-400">{link.icon}</span>}
                                    className={classNames(
                                        'relative !px-2.5 2xl:!px-4 text-sm font-medium transition-all duration-200 focus:!ring-0 focus:!ring-offset-0',
                                        activePage === link.key
                                            ? 'text-white font-semibold hover:bg-transparent'
                                            : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                                    )}
                                    {...linkProps}
                                >
                                    {link.name}
                                    {activePage === link.key && (
                                        <span
                                            aria-hidden="true"
                                            className="absolute -bottom-3 left-2 right-2 h-0.5 rounded-full bg-blue-500"
                                        />
                                    )}
                                </Button>
                            );
                        })}
                    </div>

                    {/* Divider */}
                    <div className="h-6 w-px bg-slate-700/60" />

                    <Button
                        variant="ghost"
                        onMouseEnter={() => {
                            openMegaMenu();
                            setSearchFocusToken((token) => token + 1);
                        }}
                        onClick={() => {
                            openMegaMenu();
                            setSearchFocusToken((token) => token + 1);
                        }}
                        icon={<span className="text-slate-400"><Icons.Search /></span>}
                        aria-label="Search"
                        className="!px-2.5 2xl:!px-4 text-sm font-medium transition-all duration-200 text-slate-300 hover:text-white hover:bg-slate-700/60 focus:!ring-0 focus:!ring-offset-0"
                    >
                        Search
                        <span className="ml-2 2xl:ml-2.5 whitespace-nowrap font-mono text-[11px] text-slate-500" style={{ letterSpacing: '0.02em' }}>
                            {shortcutKeyLabel()}
                        </span>
                    </Button>
                    </div>

                    {/* Organization Selector */}
                    <OrganizationSelector
                        position="navbar"
                        size="sm"
                        className="ml-2 2xl:ml-4"
                    />

                    <BillingChip />

                    {/* Logout button */}
                    {onLogout && (
                        <Button
                            variant="ghost"
                            onClick={onLogout}
                            className="!px-2.5 2xl:!px-4 ml-1 2xl:ml-3 text-slate-300 hover:text-red-300 hover:bg-red-900/20 transition-colors"
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
                            onClick={() => handleNavClick(link.path || '/git')}
                            icon={link.icon}
                            className={classNames(
                                'w-full justify-start text-sm font-medium',
                                activePage === link.key
                                    ? 'bg-blue-600 text-white'
                                    : 'text-slate-300 hover:text-white hover:bg-slate-700/60'
                            )}
                            iconPosition="left"
                            as={Link}
                            to={link.path || '/git'}
                        >
                            {link.name}
                        </Button>
                    ))}

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

            <div ref={megaMenuPanelRef}>
                <NavMegaMenu
                    isOpen={isMegaMenuOpen}
                    onClose={() => setIsMegaMenuOpen(false)}
                    sections={filteredMegaMenuSections}
                    highlightKey={hoveredNavKey}
                    searchFocusToken={searchFocusToken}
                />
            </div>
        </nav>
    );
};
