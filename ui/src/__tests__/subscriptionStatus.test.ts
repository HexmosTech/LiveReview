import {
  getSubscriptionBadgeClassByLabel,
  getSubscriptionStatusLabel,
  isTerminalSubscriptionStatus,
} from '../utils/subscriptionStatus';

describe('subscriptionStatus utility', () => {
  test('returns pending expiry when cancel_at_period_end is set', () => {
    expect(
      getSubscriptionStatusLabel({
        status: 'active',
        pendingCancel: true,
        isTeamPlan: true,
      })
    ).toBe('PENDING EXPIRY');
  });

  test('maps terminal completed status to expired label', () => {
    expect(
      getSubscriptionStatusLabel({
        status: 'completed',
      })
    ).toBe('EXPIRED');
  });

  test('returns active for team active state', () => {
    expect(
      getSubscriptionStatusLabel({
        status: 'active',
        isTeamPlan: true,
      })
    ).toBe('ACTIVE');
  });

  test('returns trial active label when trial is active', () => {
    expect(
      getSubscriptionStatusLabel({
        status: 'active',
        trialActive: true,
      })
    ).toBe('TRIAL ACTIVE');
  });

  test('terminal detector includes cancelled and expired family', () => {
    expect(isTerminalSubscriptionStatus('cancelled')).toBe(true);
    expect(isTerminalSubscriptionStatus('expired')).toBe(true);
    expect(isTerminalSubscriptionStatus('completed')).toBe(true);
    expect(isTerminalSubscriptionStatus('active')).toBe(false);
  });

  test('badge class helper returns known class for pending expiry', () => {
    expect(getSubscriptionBadgeClassByLabel('PENDING EXPIRY')).toContain('amber');
  });
});