export type SubscriptionStatusLabel =
  | 'LOADING'
  | 'FREE PLAN'
  | 'TRIAL ACTIVE'
  | 'ACTIVE'
  | 'PENDING EXPIRY'
  | 'CANCELLED'
  | 'EXPIRED'
  | 'HALTED'
  | 'PAST DUE'
  | 'INCOMPLETE'
  | 'PENDING'
  | 'AUTHENTICATED'
  | 'CREATED'
  | 'PAUSED'
  | 'RESUMED';

const normalizeStatusLabelSpacing = (value: string): string => value.replace(/_/g, ' ').toUpperCase();

export const normalizeSubscriptionStatus = (rawStatus?: string | null): string => String(rawStatus || '').trim().toLowerCase();

export const isTerminalSubscriptionStatus = (rawStatus?: string | null): boolean => {
  const normalized = normalizeSubscriptionStatus(rawStatus);
  return normalized === 'cancelled' || normalized === 'expired' || normalized === 'completed' || normalized === 'halted' || normalized === 'past_due' || normalized === 'incomplete';
};

const terminalStatusToLabel = (normalizedStatus: string): SubscriptionStatusLabel => {
  if (normalizedStatus === 'cancelled') {
    return 'CANCELLED';
  }
  if (normalizedStatus === 'expired' || normalizedStatus === 'completed') {
    return 'EXPIRED';
  }
  if (normalizedStatus === 'halted') {
    return 'HALTED';
  }
  if (normalizedStatus === 'past_due') {
    return 'PAST DUE';
  }
  return 'INCOMPLETE';
};

type SubscriptionStatusLabelInput = {
  status?: string | null;
  pendingCancel?: boolean;
  statusLoading?: boolean;
  trialActive?: boolean;
  isTeamPlan?: boolean;
  autoDowngradedToFree?: boolean;
};

export const getSubscriptionStatusLabel = ({
  status,
  pendingCancel = false,
  statusLoading = false,
  trialActive = false,
  isTeamPlan = false,
  autoDowngradedToFree = false,
}: SubscriptionStatusLabelInput): SubscriptionStatusLabel => {
  if (statusLoading) {
    return 'LOADING';
  }

  const normalizedStatus = normalizeSubscriptionStatus(status);

  if (pendingCancel) {
    return 'PENDING EXPIRY';
  }

  if (autoDowngradedToFree) {
    return 'EXPIRED';
  }

  if (isTerminalSubscriptionStatus(normalizedStatus)) {
    return terminalStatusToLabel(normalizedStatus);
  }

  if (trialActive) {
    return 'TRIAL ACTIVE';
  }

  if (isTeamPlan || normalizedStatus === 'active') {
    return 'ACTIVE';
  }

  if (normalizedStatus === 'pending') {
    return 'PENDING';
  }

  if (normalizedStatus === 'authenticated') {
    return 'AUTHENTICATED';
  }

  if (normalizedStatus === 'created') {
    return 'CREATED';
  }

  if (normalizedStatus === 'paused') {
    return 'PAUSED';
  }

  if (normalizedStatus === 'resumed') {
    return 'RESUMED';
  }

  if (normalizedStatus && normalizedStatus !== 'free') {
    return normalizeStatusLabelSpacing(normalizedStatus) as SubscriptionStatusLabel;
  }

  return 'FREE PLAN';
};

export const getSubscriptionBadgeClassByLabel = (label: SubscriptionStatusLabel): string => {
  if (label === 'ACTIVE') {
    return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/40';
  }
  if (label === 'PENDING EXPIRY') {
    return 'bg-amber-500/10 text-amber-400 border-amber-500/40';
  }
  if (label === 'TRIAL ACTIVE') {
    return 'bg-sky-900/40 text-sky-200 border border-sky-500/40';
  }
  if (label === 'EXPIRED') {
    return 'bg-red-500/10 text-red-400 border-red-500/40';
  }
  if (label === 'CANCELLED') {
    return 'bg-slate-500/10 text-slate-400 border-slate-500/40';
  }
  if (label === 'HALTED') {
    return 'bg-orange-500/10 text-orange-400 border-orange-500/40';
  }
  if (label === 'PAST DUE' || label === 'INCOMPLETE' || label === 'PENDING') {
    return 'bg-yellow-500/10 text-yellow-400 border-yellow-500/40';
  }
  if (label === 'AUTHENTICATED' || label === 'CREATED' || label === 'PAUSED' || label === 'RESUMED') {
    return 'bg-blue-500/10 text-blue-400 border-blue-500/40';
  }
  if (label === 'LOADING') {
    return 'bg-slate-700 text-slate-300 border border-slate-600';
  }
  return 'bg-slate-700 text-slate-300 border border-slate-600';
};