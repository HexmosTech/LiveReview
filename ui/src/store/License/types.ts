export type LicenseStatus = 'missing' | 'active' | 'warning' | 'grace' | 'expired' | 'invalid';

export interface LicenseStateSlice {
  loading: boolean;
  updating: boolean;
  refreshing: boolean;
  revalidating: boolean;
  deleting: boolean;
  deleteConfirmOpen: boolean;
  modalOpen?: boolean; // central UI control for license modal
  lastError?: string;
  status: LicenseStatus;
  subject?: string;
  appName?: string;
  seatCount?: number;
  activeUsers?: number;
  assignedSeats?: number;
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
  revalidating: false,
  deleting: false,
  deleteConfirmOpen: false,
  status: 'missing',
  unlimited: false,
  loadedOnce: false,
  modalOpen: false,
};
