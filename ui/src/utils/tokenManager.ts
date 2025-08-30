/**
 * Enhanced Token Manager with proactive refresh
 * Handles token lifecycle and automatic refresh before expiration
 */

import { AppStore } from '../store/configureStore';
import { setTokens, logout } from '../store/Auth/reducer';
import { refreshToken } from '../api/auth';

class TokenManager {
  private store: AppStore | null = null;
  private refreshTimer: NodeJS.Timeout | null = null;
  private isRefreshing = false;

  // Token lifetime constants (should match backend)
  private readonly ACCESS_TOKEN_DURATION = 8 * 60 * 60 * 1000; // 8 hours in ms
  private readonly REFRESH_THRESHOLD = 0.8; // Refresh at 80% of token lifetime

  public injectStore(store: AppStore) {
    this.store = store;
    this.scheduleTokenRefresh();
  }

  /**
   * Calculate when to refresh the token (80% of its lifetime)
   */
  private getRefreshTime(): number {
    return this.ACCESS_TOKEN_DURATION * this.REFRESH_THRESHOLD;
  }

  /**
   * Schedule automatic token refresh
   */
  private scheduleTokenRefresh() {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }

    const { accessToken } = this.store?.getState().Auth || {};
    if (!accessToken) return;

    // Refresh at 80% of token lifetime (6.4 hours for 8-hour tokens)
    const refreshTime = this.getRefreshTime();
    
    console.log(`Scheduling token refresh in ${Math.round(refreshTime / 1000 / 60)} minutes`);
    
    this.refreshTimer = setTimeout(() => {
      this.proactiveRefresh();
    }, refreshTime);
  }

  /**
   * Proactively refresh the token before it expires
   */
  private async proactiveRefresh() {
    if (this.isRefreshing || !this.store) return;

    const { refreshToken: currentRefreshToken } = this.store.getState().Auth;
    if (!currentRefreshToken) return;

    try {
      this.isRefreshing = true;
      console.log('Proactively refreshing access token...');
      
      const newTokens = await refreshToken(currentRefreshToken);
      this.store.dispatch(setTokens(newTokens));
      
      console.log('Token refreshed successfully');
      
      // Schedule the next refresh
      this.scheduleTokenRefresh();
      
    } catch (error) {
      console.error('Proactive token refresh failed:', error);
      // Don't logout on proactive refresh failure - wait for actual API call to handle it
    } finally {
      this.isRefreshing = false;
    }
  }

  /**
   * Handle token updates (reschedule refresh)
   */
  public onTokenUpdate() {
    this.scheduleTokenRefresh();
  }

  /**
   * Clear all timers on logout
   */
  public onLogout() {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
      this.refreshTimer = null;
    }
    this.isRefreshing = false;
  }

  /**
   * Check if user has been offline and might need token refresh
   */
  public async handleOnlineStatusChange() {
    if (!navigator.onLine) return;

    const { accessToken, refreshToken: currentRefreshToken } = this.store?.getState().Auth || {};
    
    if (!accessToken || !currentRefreshToken || this.isRefreshing) return;

    // Try to decode token to check expiration
    try {
      const payload = JSON.parse(atob(accessToken.split('.')[1]));
      const now = Date.now() / 1000;
      const timeUntilExpiry = payload.exp - now;

      // If token expires in less than 1 hour, refresh it
      if (timeUntilExpiry < 3600) {
        console.log('Token expires soon after going online, refreshing...');
        await this.proactiveRefresh();
      }
    } catch (error) {
      console.error('Failed to parse token:', error);
    }
  }
}

// Export singleton instance
export const tokenManager = new TokenManager();

// Listen for online/offline events
window.addEventListener('online', () => {
  console.log('Back online - checking token status');
  tokenManager.handleOnlineStatusChange();
});

window.addEventListener('offline', () => {
  console.log('Gone offline - token refresh will pause');
});