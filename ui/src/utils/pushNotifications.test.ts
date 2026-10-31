import { isPushSupported, subscribe, unsubscribe, getExistingSubscription } from './pushNotifications';
import * as pushApi from '../api/push';

jest.mock('../api/push', () => ({
  getVapidPublicKey: jest.fn(),
  createPushSubscription: jest.fn(),
  deletePushSubscription: jest.fn(),
}));

describe('pushNotifications', () => {
  afterEach(() => {
    jest.restoreAllMocks();
    jest.clearAllMocks();
    delete (navigator as any).serviceWorker;
    delete (window as any).PushManager;
  });

  describe('isPushSupported', () => {
    it('returns false when serviceWorker/PushManager are missing', () => {
      expect(isPushSupported()).toBe(false);
    });

    it('returns true when serviceWorker, PushManager, and a secure context are present', () => {
      Object.defineProperty(navigator, 'serviceWorker', { value: {}, configurable: true });
      (window as any).PushManager = function () {};
      Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });
      expect(isPushSupported()).toBe(true);
    });
  });

  describe('subscribe', () => {
    it('registers the service worker, subscribes pushManager, and posts the subscription to the backend', async () => {
      const mockSubscription = {
        endpoint: 'https://push.example.com/abc',
        toJSON: () => ({ keys: { p256dh: 'p256dh-value', auth: 'auth-value' } }),
      };
      const mockRegistration = {
        pushManager: { subscribe: jest.fn().mockResolvedValue(mockSubscription) },
      };

      Object.defineProperty(navigator, 'serviceWorker', {
        value: { register: jest.fn().mockResolvedValue(mockRegistration) },
        configurable: true,
      });
      (window as any).PushManager = function () {};
      Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });

      (pushApi.getVapidPublicKey as jest.Mock).mockResolvedValue('AAAA');
      (pushApi.createPushSubscription as jest.Mock).mockResolvedValue(undefined);

      await subscribe();

      expect(navigator.serviceWorker.register).toHaveBeenCalledWith('/sw.js');
      expect(mockRegistration.pushManager.subscribe).toHaveBeenCalledWith(
        expect.objectContaining({ userVisibleOnly: true })
      );
      expect(pushApi.createPushSubscription).toHaveBeenCalledWith({
        endpoint: 'https://push.example.com/abc',
        keys: { p256dh: 'p256dh-value', auth: 'auth-value' },
      });
    });
  });

  describe('unsubscribe', () => {
    it('does nothing when there is no existing subscription', async () => {
      Object.defineProperty(navigator, 'serviceWorker', {
        value: { getRegistration: jest.fn().mockResolvedValue(undefined) },
        configurable: true,
      });
      (window as any).PushManager = function () {};
      Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });

      await unsubscribe();
      expect(pushApi.deletePushSubscription).not.toHaveBeenCalled();
    });

    it('unsubscribes locally and removes the subscription from the backend when one exists', async () => {
      const mockSubscription = {
        endpoint: 'https://push.example.com/abc',
        unsubscribe: jest.fn().mockResolvedValue(true),
      };
      const mockRegistration = {
        pushManager: { getSubscription: jest.fn().mockResolvedValue(mockSubscription) },
      };
      Object.defineProperty(navigator, 'serviceWorker', {
        value: { getRegistration: jest.fn().mockResolvedValue(mockRegistration) },
        configurable: true,
      });
      (window as any).PushManager = function () {};
      Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true });

      await unsubscribe();

      expect(mockSubscription.unsubscribe).toHaveBeenCalled();
      expect(pushApi.deletePushSubscription).toHaveBeenCalledWith('https://push.example.com/abc');
    });
  });

  describe('getExistingSubscription', () => {
    it('returns null when push is unsupported', async () => {
      await expect(getExistingSubscription()).resolves.toBeNull();
    });
  });
});
