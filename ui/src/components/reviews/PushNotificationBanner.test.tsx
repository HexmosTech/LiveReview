import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import '@testing-library/jest-dom';
import { PushNotificationBanner } from './PushNotificationBanner';
import * as pushNotifications from '../../utils/pushNotifications';
import { notify } from '../../utils/notify';

const renderBanner = () => render(<PushNotificationBanner />, { wrapper: MemoryRouter });

jest.mock('../../utils/pushNotifications', () => ({
  isPushSupported: jest.fn(),
  subscribe: jest.fn(),
  getExistingSubscription: jest.fn(),
}));
jest.mock('../../utils/notify', () => ({
  notify: { success: jest.fn(), error: jest.fn() },
}));

const DISMISSED_KEY = 'livereview:push-notify-banner-dismissed';

describe('PushNotificationBanner', () => {
  const originalNotification = (global as any).Notification;

  beforeEach(() => {
    localStorage.clear();
    (pushNotifications.isPushSupported as jest.Mock).mockReturnValue(true);
    (pushNotifications.getExistingSubscription as jest.Mock).mockResolvedValue(null);
    (global as any).Notification = { permission: 'default', requestPermission: jest.fn() };
  });

  afterEach(() => {
    (global as any).Notification = originalNotification;
    jest.clearAllMocks();
  });

  it('renders nothing when push is unsupported', async () => {
    (pushNotifications.isPushSupported as jest.Mock).mockReturnValue(false);
    const { container } = renderBanner();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it('renders nothing when already subscribed on this browser', async () => {
    (pushNotifications.getExistingSubscription as jest.Mock).mockResolvedValue({ endpoint: 'x' });
    const { container } = renderBanner();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it('renders nothing when previously dismissed', async () => {
    localStorage.setItem(DISMISSED_KEY, '1');
    const { container } = renderBanner();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it('shows an inline Enable CTA and a close button when eligible, without auto-dismissing', async () => {
    renderBanner();
    await waitFor(() => expect(screen.getByText(/browser notification/i)).toBeInTheDocument());
    expect(screen.getByRole('button', { name: /enable notifications/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /dismiss/i })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /settings/i })).toHaveAttribute('href', '/settings#notifications');
  });

  it('shows blocked guidance instead of the CTA when permission was previously denied', async () => {
    (global as any).Notification.permission = 'denied';
    renderBanner();
    await waitFor(() => expect(screen.getByText(/blocked for this site/i)).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: /enable notifications/i })).not.toBeInTheDocument();
    expect(screen.getByText(/address bar/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /settings/i })).toBeInTheDocument();
  });

  it('subscribes and hides the banner when Enable is clicked and permission is granted', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('granted');
    (pushNotifications.subscribe as jest.Mock).mockResolvedValue(undefined);

    renderBanner();
    await waitFor(() => screen.getByRole('button', { name: /enable notifications/i }));
    fireEvent.click(screen.getByRole('button', { name: /enable notifications/i }));

    await waitFor(() => expect(pushNotifications.subscribe).toHaveBeenCalled());
    expect(notify.success).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText(/browser notification/i)).not.toBeInTheDocument());
  });

  it('keeps the banner visible and lets the user retry if subscribe() fails after permission is granted', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('granted');
    (pushNotifications.subscribe as jest.Mock).mockRejectedValue(new Error('network error'));

    renderBanner();
    await waitFor(() => screen.getByRole('button', { name: /enable notifications/i }));
    fireEvent.click(screen.getByRole('button', { name: /enable notifications/i }));

    await waitFor(() => expect(notify.error).toHaveBeenCalled());
    expect(screen.getByRole('button', { name: /enable notifications/i })).toBeInTheDocument();
  });

  it('closing the banner persists the dismissal so it does not reappear', async () => {
    renderBanner();
    await waitFor(() => screen.getByRole('button', { name: /dismiss/i }));
    fireEvent.click(screen.getByRole('button', { name: /dismiss/i }));

    expect(screen.queryByText(/browser notification/i)).not.toBeInTheDocument();
    expect(localStorage.getItem(DISMISSED_KEY)).toBe('1');
  });
});
