export type LicenseStatus = 'missing' | 'active' | 'warning' | 'grace' | 'expired' | 'invalid';

export interface LicenseStateSlice {
  loading: boolean;
  updating: boolean;
  refreshing: boolean;
  modalOpen?: boolean; // central UI control for license modal
  lastError?: string;
  status: LicenseStatus;
  subject?: string;
  appName?: string;
  seatCount?: number;
  unlimited: boolean;
  expiresAt?: string;
  lastValidatedAt?: string;
  lastValidationCode?: string;
  loadedOnce: boolean;
}

export const initialLicenseState: LicenseStateSlice = {
  loading: false,
  updating: false,
  refreshing: false,
  status: 'missing',
  unlimited: false,
  loadedOnce: false,
  modalOpen: false,
};
