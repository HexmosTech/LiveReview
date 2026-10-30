import apiClient from './apiClient';

export interface NotificationPreferences {
  notify_first_finding: boolean;
  notify_completion: boolean;
}

export const getVapidPublicKey = async (): Promise<string> => {
  const res = await apiClient.get<{ public_key: string }>('/push/vapid-public-key');
  return res.public_key;
};

export interface PushSubscriptionKeys {
  endpoint: string;
  keys: { p256dh: string; auth: string };
}

export const createPushSubscription = async (subscription: PushSubscriptionKeys): Promise<void> => {
  await apiClient.post('/push/subscriptions', subscription);
};

export const deletePushSubscription = async (endpoint: string): Promise<void> => {
  await apiClient.delete(`/push/subscriptions?endpoint=${encodeURIComponent(endpoint)}`);
};

export const getNotificationPreferences = async (): Promise<NotificationPreferences> => {
  return apiClient.get<NotificationPreferences>('/users/me/notification-preferences');
};

export const updateNotificationPreferences = async (prefs: NotificationPreferences): Promise<NotificationPreferences> => {
  return apiClient.put<NotificationPreferences>('/users/me/notification-preferences', prefs);
};
