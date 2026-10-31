// Shared connector error alert banner — used on any Explore page that wants to
// surface org-level connector credential/mismatch errors without duplicating logic.
//
// Usage:
//   const { alerts, dismissAlert } = useConnectorAlerts();
//   {alerts.map(a => <ConnectorAlertBanner key={a.connector.id} alert={a} onDismiss={dismissAlert} />)}

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { Alert, Button } from '../UIPrimitives';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { getRepositories } from '../../api/repositories';
import { Repository } from '../../types/explore';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import {
  add as addNotification,
  dismiss as dismissNotification,
  dismissPermanently as dismissNotificationPermanently,
} from '../../store/Notifications/slice';
import { useDashboardQuery, ConnectorSetupProgress } from '../../api/dashboard';

// ── Provider helpers ──────────────────────────────────────────────────────────

export function normalizeProvider(provider: string): string {
  const n = (provider || '').toLowerCase();
  if (n.startsWith('github')) return 'github';
  if (n.startsWith('gitlab')) return 'gitlab';
  if (n.startsWith('bitbucket')) return 'bitbucket';
  if (n.startsWith('gitea')) return 'gitea';
  if (n.startsWith('azuredevops')) return 'azuredevops';
  return n;
}

export function providerLabel(provider: string): string {
  switch (normalizeProvider(provider)) {
    case 'github': return 'GitHub';
    case 'gitlab': return 'GitLab';
    case 'bitbucket': return 'Bitbucket';
    case 'gitea': return 'Gitea';
    case 'azuredevops': return 'Azure DevOps';
    default: return provider;
  }
}

export function wikiUrlForProvider(provider: string): string {
  const norm = (provider || '').toLowerCase();
  if (norm.startsWith('gitlab-self') || norm === 'gitlab-self-hosted')
    return 'https://github.com/HexmosTech/LiveReview/wiki/Self%E2%80%90Hosted-GitLab';
  if (norm.startsWith('gitlab')) return 'https://github.com/HexmosTech/LiveReview/wiki/Gitlab';
  if (norm.startsWith('github')) return 'https://github.com/HexmosTech/LiveReview/wiki/Github';
  if (norm.startsWith('bitbucket')) return 'https://github.com/HexmosTech/LiveReview/wiki/BitBucket';
  return 'https://github.com/HexmosTech/LiveReview/wiki';
}

// ── Alert types ───────────────────────────────────────────────────────────────

export type ConnectorErrorKind = 'auth' | 'mismatch';

export interface ConnectorAlert {
  connector: ConnectorResponse;
  kind: ConnectorErrorKind;
  repoCount: number;
}

export function computeConnectorAlerts(
  repos: Repository[],
  connectors: ConnectorResponse[]
): ConnectorAlert[] {
  const connectorMap = new Map(connectors.map((c) => [c.id, c]));
  const counts = new Map<number, { auth: number; mismatch: number }>();

  for (const repo of repos) {
    if (repo.last_sync_status !== 'error' || !repo.last_sync_error) continue;
    const err = repo.last_sync_error.toLowerCase();
    const isForbidden =
      err.includes('forbidden') || err.includes('403') || err.includes('access forbidden');
    const isAuth =
      !isForbidden &&
      (err.includes('auth') ||
        err.includes('token') ||
        err.includes('401') ||
        err.includes('unauthorized') ||
        err.includes('credential'));
    const isMismatch =
      err.includes('mismatch') ||
      err.includes('does not match') ||
      err.includes('configuration error') ||
      err.includes('incompatible');
    if (!isAuth && !isMismatch) continue;

    const existing = counts.get(repo.connector_id) ?? { auth: 0, mismatch: 0 };
    if (isAuth) existing.auth += 1;
    if (isMismatch) existing.mismatch += 1;
    counts.set(repo.connector_id, existing);
  }

  const alerts: ConnectorAlert[] = [];
  for (const [connectorId, { auth, mismatch }] of counts) {
    const connector = connectorMap.get(connectorId);
    if (!connector) continue;
    if (auth > 0) alerts.push({ connector, kind: 'auth', repoCount: auth });
    else if (mismatch > 0) alerts.push({ connector, kind: 'mismatch', repoCount: mismatch });
  }
  return alerts;
}

// ── Session-persisted dismiss state ──────────────────────────────────────────

const LOCAL_KEY = 'dismissed_connector_alerts';

function loadDismissed(): Set<number> {
  try {
    const raw = localStorage.getItem(LOCAL_KEY);
    return raw ? new Set<number>(JSON.parse(raw)) : new Set<number>();
  } catch {
    return new Set<number>();
  }
}

function saveDismissed(ids: Set<number>): void {
  try {
    localStorage.setItem(LOCAL_KEY, JSON.stringify([...ids]));
  } catch { /* ignore */ }
}

// ── Hook ──────────────────────────────────────────────────────────────────────

/** Fetches connectors + error-status repos and returns visible connector alerts
 *  plus a dismiss callback. Dismissals are shared across tabs via localStorage. */
export function useConnectorAlerts(): {
  alerts: ConnectorAlert[];
  dismissAlert: (connectorId: number) => void;
  refresh: () => Promise<void>;
} {
  const dispatch = useAppDispatch();
  const location = useLocation();
  const notificationItems = useAppSelector((state) => state.Notifications.items);
  const persistedNotifications = useAppSelector((state) => state.Notifications.persisted);
  const [allAlerts, setAllAlerts] = useState<ConnectorAlert[]>([]);
  const [dismissed, setDismissed] = useState<Set<number>>(loadDismissed);
  const lastFetchTimeRef = useRef<number>(0);

  const fetchAlerts = useCallback(async () => {
    try {
      const [connectors, repoResp] = await Promise.all([
        getConnectors(),
        // Only pull error repos — targeted scan, not the full table.
        getRepositories({ syncStatus: 'error', page: 1, perPage: 100 }),
      ]);
      const repos: Repository[] = repoResp.repositories || [];
      const computed = computeConnectorAlerts(repos, connectors);
      setAllAlerts(computed);
      lastFetchTimeRef.current = Date.now();

      // Mirror into notifications store as well
      computed.forEach((alert) => {
        const pName = providerLabel(alert.connector.provider);
        const wiki = wikiUrlForProvider(alert.connector.provider);
        const title =
          alert.kind === 'auth'
            ? `Your ${pName} provider credential expired, please renew it.`
            : `Provider Mismatch: ${pName} (${alert.connector.connection_name})`;
        const message =
          alert.kind === 'auth'
            ? `Connector "${alert.connector.connection_name}": ${alert.repoCount} ${
                alert.repoCount === 1 ? 'repository' : 'repositories'
              } could not sync due to expired or invalid credentials.`
            : `${alert.repoCount} ${
                alert.repoCount === 1 ? 'repository' : 'repositories'
              } have a mismatched provider configured.`;

        dispatch(
          addNotification({
            dedupeKey: `connector-${alert.kind}-${alert.connector.id}`,
            severity: 'error',
            title,
            message,
            source: 'connector',
            toast: false,
            persistDismiss: true,
            actions: [
              { label: 'Renew Credentials', actionId: `/git/connector/${alert.connector.id}` },
              { label: `${pName} Setup Guide`, actionId: wiki },
            ],
          })
        );
      });
    } catch {
      // Non-critical — silently skip alerts on fetch error
    }
  }, [dispatch]);

  // Initial fetch and route change check (debounced so we don't spam on rapid clicks)
  useEffect(() => {
    const now = Date.now();
    if (now - lastFetchTimeRef.current < 10_000) {
      return;
    }
    fetchAlerts();
  }, [location.pathname, fetchAlerts]);

  // Also listen for custom refresh events (e.g. after a manual connector sync)
  useEffect(() => {
    const handleSyncComplete = () => {
      fetchAlerts();
    };
    window.addEventListener('connector-sync-finished', handleSyncComplete);
    return () => window.removeEventListener('connector-sync-finished', handleSyncComplete);
  }, [fetchAlerts]);

  const dismissAlert = (connectorId: number) => {
    setDismissed((prev) => {
      const next = new Set([...prev, connectorId]);
      saveDismissed(next);
      return next;
    });
    dispatch(dismissNotificationPermanently(`connector-auth-${connectorId}`));
    dispatch(dismissNotificationPermanently(`connector-mismatch-${connectorId}`));
    dispatch(dismissNotificationPermanently(`connector-${connectorId}`));
  };

  const isAlertDismissed = (connectorId: number, kind: string): boolean => {
    if (dismissed.has(connectorId)) return true;
    const key = `connector-${kind}-${connectorId}`;
    if (persistedNotifications?.[key]?.dismissed) return true;
    const item = notificationItems.find((n) => n.id === key);
    if (item?.dismissed) return true;
    return false;
  };

  const alerts = allAlerts.filter((a) => !isAlertDismissed(a.connector.id, a.kind));
  return { alerts, dismissAlert, refresh: fetchAlerts };
}

// ── ExternalLinkIcon ──────────────────────────────────────────────────────────

const ExternalLinkIcon: React.FC<{ className?: string }> = ({ className = 'w-3 h-3' }) => (
  <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
  </svg>
);

// ── Dismissible banner (single connector) ─────────────────────────────────────

export const ConnectorAlertBanner: React.FC<{
  alert: ConnectorAlert;
  onDismiss: (connectorId: number) => void;
}> = ({ alert, onDismiss }) => {
  const navigate = useNavigate();
  const { connector, kind, repoCount } = alert;
  const wikiUrl = wikiUrlForProvider(connector.provider);
  const pName = providerLabel(connector.provider);
  const repoLabel = repoCount === 1 ? '1 repository' : `${repoCount} repositories`;

  const title =
    kind === 'auth'
      ? `${pName} credential expired — please renew it`
      : `Provider mismatch detected for ${pName} ("${connector.connection_name}")`;

  const body =
    kind === 'auth'
      ? `Connector "${connector.connection_name}": ${repoLabel} could not sync because the access token has expired, been revoked, or lacks required permissions.`
      : `${repoLabel} cannot sync — the repositories belong to a different Git provider than ${pName}.`;

  return (
    <Alert
      variant="warning"
      onClose={() => onDismiss(connector.id)}
      className="w-full"
    >
      <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="sm:pr-4">
          <div className="font-semibold text-amber-100 mb-1">{title}</div>
          <p className="text-sm opacity-90 mb-2">{body}</p>
          <div className="flex items-center gap-4 flex-wrap">
            <Link
              to={`/git/connector/${connector.id}`}
              className="inline-flex items-center gap-1.5 text-xs font-medium underline underline-offset-2 opacity-90 hover:opacity-100"
            >
              Renew {pName} credentials
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
              </svg>
            </Link>
            <a
              href={wikiUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 text-xs font-medium underline underline-offset-2 opacity-75 hover:opacity-100"
            >
              {pName} setup guide
              <ExternalLinkIcon />
            </a>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:ml-4 sm:flex-nowrap flex-shrink-0">
          <Button
            variant="ghost"
            size="sm"
            className="text-current hover:bg-black/10 hover:opacity-90"
            onClick={(e) => {
              e.stopPropagation();
              onDismiss(connector.id);
            }}
          >
            Don&apos;t show again
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="!border-current !text-current hover:opacity-80"
            onClick={(e) => {
              e.stopPropagation();
              navigate(`/git/connector/${connector.id}`);
            }}
          >
            View Details
          </Button>
        </div>
      </div>
    </Alert>
  );
};

// ── Connector setup progress helpers & component ──────────────────────────────

export function getPhaseVariant(phase: string): 'info' | 'warning' | 'success' | 'error' {
  switch (phase) {
    case 'discovering': return 'info';
    case 'installing': return 'warning';
    case 'ready': return 'success';
    case 'error': return 'error';
    default: return 'info';
  }
}

export function getPhaseMessage(
  phase: string,
  connectorName: string,
  provider: string,
  totalProjects: number,
  connectedProjects: number
): string {
  switch (phase) {
    case 'discovering':
      return `${connectorName} (${provider}): Discovering projects...`;
    case 'installing': {
      const percent = totalProjects > 0 ? Math.round((connectedProjects / totalProjects) * 100) : 0;
      return `${connectorName} (${provider}): Installing webhooks ${connectedProjects}/${totalProjects} (${percent}%)`;
    }
    case 'ready':
      return `${connectorName} (${provider}): Ready - ${totalProjects} projects connected`;
    case 'error':
      return `${connectorName} (${provider}): Setup failed - click to retry`;
    default:
      return `${connectorName} (${provider})`;
  }
}

export const ConnectorSetupProgressBanner: React.FC<{
  connector: ConnectorSetupProgress;
  onDismissSession: (connectorId: number) => void;
  onDismissPermanent: (connectorId: number) => void;
}> = ({ connector, onDismissSession, onDismissPermanent }) => {
  const navigate = useNavigate();

  return (
    <Alert
      variant={getPhaseVariant(connector.phase)}
      onClose={() => onDismissSession(connector.connector_id)}
      className="w-full cursor-pointer hover:opacity-95"
    >
      <div
        className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
        onClick={() => navigate(`/git/connector/${connector.connector_id}`)}
      >
        <span className="sm:pr-4">
          {getPhaseMessage(
            connector.phase,
            connector.connector_name,
            connector.provider,
            connector.total_projects,
            connector.connected_projects
          )}
        </span>
        <div className="flex flex-wrap items-center gap-2 sm:ml-4 sm:flex-nowrap flex-shrink-0">
          <Button
            variant="ghost"
            size="sm"
            className="text-current hover:bg-black/10 hover:opacity-90"
            onClick={(e) => {
              e.stopPropagation();
              onDismissPermanent(connector.connector_id);
            }}
          >
            Don&apos;t show again
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="!border-current !text-current hover:opacity-80"
            onClick={(e) => {
              e.stopPropagation();
              navigate(`/git/connector/${connector.connector_id}`);
            }}
          >
            View Details
          </Button>
        </div>
      </div>
    </Alert>
  );
};

// ── Global layout alert container (renders at top of all pages) ───────────────

import { ConnectorAuthAlert, ConnectorSetupProgressAlert } from '../../store/Notifications/catalog';

const isCredentialAlertRoute = (pathname: string): boolean => {
  return ConnectorAuthAlert.matchesRoute(pathname);
};

const isDashboardRoute = (pathname: string): boolean => {
  return ConnectorSetupProgressAlert.matchesRoute(pathname);
};

export const GlobalConnectorAlerts: React.FC = () => {
  const location = useLocation();
  const dispatch = useAppDispatch();
  const notificationItems = useAppSelector((state) => state.Notifications.items);
  const persistedNotifications = useAppSelector((state) => state.Notifications.persisted);
  const { data: dashboardData } = useDashboardQuery();

  const { alerts, dismissAlert } = useConnectorAlerts();

  const connectorNotificationId = (connectorId: number): string => `connector-${connectorId}`;

  const isConnectorDismissed = (connectorId: number): boolean => {
    const id = connectorNotificationId(connectorId);
    const item = notificationItems.find((n) => n.id === id);
    if (item?.dismissed) return true;
    return !!persistedNotifications?.[id]?.dismissed;
  };

  const dismissSetup = (connectorId: number): void => {
    dispatch(dismissNotificationPermanently(connectorNotificationId(connectorId)));
  };

  // Mirror connector setup progress into Notifications store
  useEffect(() => {
    const progress = dashboardData?.connector_setup_progress || [];
    progress.forEach((connector) => {
      dispatch(
        addNotification({
          dedupeKey: connectorNotificationId(connector.connector_id),
          severity: getPhaseVariant(connector.phase),
          message: getPhaseMessage(
            connector.phase,
            connector.connector_name,
            connector.provider,
            connector.total_projects,
            connector.connected_projects
          ),
          source: 'connector',
          toast: false,
          persistDismiss: true,
        })
      );
    });
  }, [dashboardData?.connector_setup_progress, dispatch]);

  const connectorsNeedingSetup = (dashboardData?.connector_setup_progress || []).filter(
    (c) => !isConnectorDismissed(c.connector_id)
  );

  const showCredentialAlerts = isCredentialAlertRoute(location.pathname);
  const showSetupProgress = isDashboardRoute(location.pathname);

  const visibleAlerts = showCredentialAlerts ? alerts : [];
  const visibleSetup = showSetupProgress ? connectorsNeedingSetup : [];

  const hasAlerts = visibleAlerts.length > 0 || visibleSetup.length > 0;
  if (!hasAlerts) return null;

  return (
    <div className="container mx-auto px-4 pt-6" data-testid="global-connector-alerts">
      <div className="space-y-3">
        {visibleAlerts.map((alert) => (
          <ConnectorAlertBanner
            key={`error-${alert.kind}-${alert.connector.id}`}
            alert={alert}
            onDismiss={dismissAlert}
          />
        ))}
        {visibleSetup.map((connector) => (
          <ConnectorSetupProgressBanner
            key={`setup-${connector.connector_id}`}
            connector={connector}
            onDismissSession={dismissSetup}
            onDismissPermanent={dismissSetup}
          />
        ))}
      </div>
    </div>
  );
};

