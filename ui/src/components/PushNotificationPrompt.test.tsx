import React from 'react';
import { render } from '@testing-library/react';
import { PushNotificationPrompt, PUSH_ENABLE_ACTION_ID } from './PushNotificationPrompt';
import { runNotificationAction } from '../store/Notifications/actionRegistry';
import { subscribe } from '../utils/pushNotifications';
import { notify } from '../utils/notify';

jest.mock('../utils/pushNotifications', () => ({
  subscribe: jest.fn(),
}));
jest.mock('../utils/notify', () => ({
  notify: { success: jest.fn(), error: jest.fn() },
}));

describe('PushNotificationPrompt', () => {
  const originalNotification = (global as any).Notification;

  beforeEach(() => {
    (global as any).Notification = { requestPermission: jest.fn() };
  });

  afterEach(() => {
    (global as any).Notification = originalNotification;
    jest.clearAllMocks();
  });

  it('subscribes and shows a success toast when permission is granted', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('granted');
    (subscribe as jest.Mock).mockResolvedValue(undefined);

    render(<PushNotificationPrompt />);
    await runNotificationActionAsync();

    expect(subscribe).toHaveBeenCalled();
    expect(notify.success).toHaveBeenCalled();
  });

  it('does not subscribe when permission is denied', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('denied');

    render(<PushNotificationPrompt />);
    await runNotificationActionAsync();

    expect(subscribe).not.toHaveBeenCalled();
    expect(notify.success).not.toHaveBeenCalled();
  });

  it('shows an error toast when subscribing fails', async () => {
    (Notification.requestPermission as jest.Mock).mockResolvedValue('granted');
    (subscribe as jest.Mock).mockRejectedValue(new Error('boom'));

    render(<PushNotificationPrompt />);
    await runNotificationActionAsync();

    expect(notify.error).toHaveBeenCalled();
  });
});

async function runNotificationActionAsync() {
  runNotificationAction(PUSH_ENABLE_ACTION_ID);
  // Flush the microtask queue so the async handler completes.
  await new Promise((resolve) => setTimeout(resolve, 0));
  await new Promise((resolve) => setTimeout(resolve, 0));
}
