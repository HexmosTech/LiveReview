import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { PushNotificationBanner } from './PushNotificationBanner';
import * as pushNotifications from '../../utils/pushNotifications';
import { notify } from '../../utils/notify';

jest.mock('../../utils/pushNotifications', () => ({
  isPushSupported: jest.fn(),
  subscribe: jest.fn(),
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
    (global as any).Notification = { permission: 'default', requestPermission: jest.fn() };
  });

  afterEach(() => {
    (global as any).Notification = originalNotification;
    jest.clearAllMocks();
  });

  it('renders nothing when push is unsupported', () => {
    (pushNotifications.isPushSupported as jest.Mock).mockReturnValue(false);
    const { container } = render(<PushNotificationBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when permission is already granted or denied', () => {
    (global as any).Notification.permission = 'granted';
    const { container } = render(<PushNotificationBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when previously dismissed', () => {
    localStorage.setItem(DISMISSED_KEY, '1');
    const { container } = render(<PushNotificationBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows an inline Enable CTA and a close button when eligible, without auto-dismissing', () => {
    render(<PushNotificationBanner />);
    expect(screen.getByText(/browser notification/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /enable notifications/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /dismiss/i })).toBeInTheDocument();
  });

  it('subscribes and hides the banner when Enable is clicked and permission is granted', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('granted');
    (pushNotifications.subscribe as jest.Mock).mockResolvedValue(undefined);

    render(<PushNotificationBanner />);
    fireEvent.click(screen.getByRole('button', { name: /enable notifications/i }));

    await waitFor(() => expect(pushNotifications.subscribe).toHaveBeenCalled());
    expect(notify.success).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText(/browser notification/i)).not.toBeInTheDocument());
  });

  it('closing the banner persists the dismissal so it does not reappear', () => {
    render(<PushNotificationBanner />);
    fireEvent.click(screen.getByRole('button', { name: /dismiss/i }));

    expect(screen.queryByText(/browser notification/i)).not.toBeInTheDocument();
    expect(localStorage.getItem(DISMISSED_KEY)).toBe('1');
  });
});
