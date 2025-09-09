import reducer, { fetchLicenseStatus, submitLicenseToken, triggerLicenseRefresh } from '../store/License/slice';
import { initialLicenseState } from '../store/License/types';
import { AnyAction } from 'redux';

// Minimal reducer tests (no mock fetch wiring; we just test state transitions on pending/fulfilled/rejected)

describe('license slice reducer', () => {
  it('should return initial state', () => {
    // @ts-ignore
    expect(reducer(undefined, { type: 'unknown' })).toEqual(initialLicenseState);
  });

  it('handles fetch pending/fulfilled', () => {
    const pendingState = reducer(initialLicenseState, { type: fetchLicenseStatus.pending.type });
    expect(pendingState.loading).toBe(true);

    const fulfilledState = reducer(pendingState, { type: fetchLicenseStatus.fulfilled.type, payload: { status: 'active', unlimited: false } });
    expect(fulfilledState.loading).toBe(false);
    expect(fulfilledState.status).toBe('active');
  });

  it('handles submit rejected', () => {
    const updating = reducer(initialLicenseState, { type: submitLicenseToken.pending.type });
    expect(updating.updating).toBe(true);
    const rejected = reducer(updating, { type: submitLicenseToken.rejected.type, error: { message: 'boom' } as any as Error });
    expect(rejected.updating).toBe(false);
    expect(rejected.lastError).toBe('boom');
  });

  it('handles refresh fulfilled', () => {
    const refreshing = reducer(initialLicenseState, { type: triggerLicenseRefresh.pending.type });
    expect(refreshing.refreshing).toBe(true);
    const done = reducer(refreshing, { type: triggerLicenseRefresh.fulfilled.type, payload: { status: 'warning', unlimited: false } });
    expect(done.refreshing).toBe(false);
    expect(done.status).toBe('warning');
  });
});
