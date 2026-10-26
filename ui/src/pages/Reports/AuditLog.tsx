import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { authFetch } from '../../api/apiClient';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';
import { Alert, Button, Card, Input, Spinner } from '../../components/UIPrimitives';
import { Table, TableHead, TableHeaderCell, TableBody, TableRow, TableCell } from '../../components/DataTable/SimpleTable';

type AuditEvent = {
  id: number;
  occurred_at: string;
  org_id: number;
  actor_user_id?: number;
  actor_email?: string;
  actor_name?: string;
  actor_kind: string;
  actor_capacity?: string;
  auth_method?: string;
  api_key_id?: number;
  action: string;
  category: string;
  resource_type?: string;
  resource_id?: string;
  outcome: string;
  status_code?: number;
  severity: string;
  ip?: string;
  user_agent?: string;
  request_id?: string;
  method?: string;
  path?: string;
  details?: Record<string, unknown>;
  source?: string;
  seq?: number;
  hash?: string;
  action_label?: string;
  category_label?: string;
};

type ListResponse = { events: AuditEvent[]; next_cursor: string; has_more: boolean };
type FacetValue = { value: string; label?: string; count: number };
type Facets = {
  actions: FacetValue[];
  categories: FacetValue[];
  outcomes: FacetValue[];
  severities: FacetValue[];
  actors: FacetValue[];
};
type Meta = {
  hot_days: number;
  retention_days: number;
  archive_enabled: boolean;
  anchor_enabled: boolean;
  can_manage: boolean;
  now: string;
};
type AuditSettings = {
  enabled: boolean;
  hot_days: number;
  retention_days: number;
  archive_enabled: boolean;
  anchor_enabled: boolean;
  hash_chain_enabled: boolean;
  cron_expression: string;
};
type IntegrityReport = {
  org_id: number;
  rows: number;
  verified: number;
  first_broken_seq?: number;
  first_broken_reason?: string;
  chain_start?: string;
  chain_end?: string;
  intact: boolean;
};

type AIUsageRow = {
  accounted_at: string;
  review_id?: number;
  user_id?: number;
  operation_type: string;
  trigger_source: string;
  provider?: string;
  model?: string;
  input_tokens?: number;
  output_tokens?: number;
  cost_usd?: number;
  actor_kind?: string;
  actor_email?: string;
};

type RangePreset = '24h' | '7d' | '30d' | '90d' | 'custom';

const RANGE_LABELS: Record<Exclude<RangePreset, 'custom'>, string> = {
  '24h': '24 hours',
  '7d': '7 days',
  '30d': '30 days',
  '90d': '90 days',
};

const CATEGORY_LABELS: Record<string, string> = {
  auth: 'Sign-ins & access',
  user: 'Users & roles',
  org: 'Organization',
  api_key: 'API keys',
  billing: 'Billing',
  settings: 'Settings',
  connector: 'Integrations',
  repository: 'Repositories',
  review: 'Reviews',
  chat: 'Chat',
  export: 'Exports & downloads',
  audit: 'Audit log',
  ai: 'AI & models',
  admin: 'Administration',
  system: 'System',
  other: 'Other',
};

const OUTCOME_STYLES: Record<string, string> = {
  success: 'bg-emerald-900/40 text-emerald-300 border-emerald-700',
  failure: 'bg-amber-900/40 text-amber-300 border-amber-700',
  denied: 'bg-red-900/40 text-red-300 border-red-700',
  error: 'bg-red-900/40 text-red-300 border-red-700',
};

const SEVERITY_STYLES: Record<string, string> = {
  info: 'text-slate-400',
  warning: 'text-amber-400',
  critical: 'text-red-400',
};

function isoForPreset(preset: RangePreset, customFrom: string, customTo: string): { from: string; to: string } {
  if (preset === 'custom') {
    return {
      from: customFrom ? new Date(customFrom).toISOString() : new Date(Date.now() - 7 * 864e5).toISOString(),
      to: customTo ? new Date(customTo).toISOString() : new Date().toISOString(),
    };
  }
  const hours = preset === '24h' ? 24 : preset === '7d' ? 24 * 7 : preset === '30d' ? 24 * 30 : 24 * 90;
  const now = new Date();
  return { from: new Date(now.getTime() - hours * 3600e3).toISOString(), to: now.toISOString() };
}

function fmtTime(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString();
}

function fmtDate(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function fmtClock(v: string): string {
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function fmtActor(ev: AuditEvent): string {
  if (ev.actor_name && ev.actor_email) return `${ev.actor_name} <${ev.actor_email}>`;
  return ev.actor_email || ev.actor_name || 'System';
}

function actionLabel(ev: AuditEvent): string {
  return ev.action_label || ev.action;
}

function downloadBlob(blob: Blob, filename: string) {
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.URL.revokeObjectURL(url);
}

const AuditLog: React.FC = () => {
  const { currentOrgId, currentOrg, allOrganizations, isSuperAdmin, loadAllOrgs } = useOrgContext();
  const canSeeOrg = isSuperAdmin || currentOrg?.role === 'owner';
  const canDownloadArchive = canSeeOrg;

  const [range, setRange] = useState<RangePreset>('7d');
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');
  const [category, setCategory] = useState('');
  const [action, setAction] = useState('');
  const [outcome, setOutcome] = useState('');
  const [actorEmail, setActorEmail] = useState('');
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [debouncedActor, setDebouncedActor] = useState('');
  const [allOrgs, setAllOrgs] = useState(false);
  const [adminOrgId, setAdminOrgId] = useState<string>('');

  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [live, setLive] = useState(true);
  const [selected, setSelected] = useState<AuditEvent | null>(null);
  const [facets, setFacets] = useState<Facets | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [settings, setSettings] = useState<AuditSettings | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [savingSettings, setSavingSettings] = useState(false);
  const [runningMaintenance, setRunningMaintenance] = useState(false);
  const [maintenanceMsg, setMaintenanceMsg] = useState('');
  const [integrityOrg, setIntegrityOrg] = useState('');
  const [integrity, setIntegrity] = useState<IntegrityReport | null>(null);
  const [checkingIntegrity, setCheckingIntegrity] = useState(false);
  const [exporting, setExporting] = useState('');
  const [tab, setTab] = useState<'activity' | 'ai'>('activity');
  const [aiRows, setAiRows] = useState<AIUsageRow[]>([]);
  const [aiLoading, setAiLoading] = useState(false);

  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedSearch(search), 300);
    return () => window.clearTimeout(t);
  }, [search]);

  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedActor(actorEmail), 300);
    return () => window.clearTimeout(t);
  }, [actorEmail]);

  const inFlight = useRef(false);
  const latestRef = useRef<string>('');
  const requestSeq = useRef(0);

  const effectiveOrgId = allOrgs ? adminOrgId : String(currentOrgId ?? '');
  const useAdminEndpoint = isSuperAdmin && allOrgs;

  const rangeValues = useMemo(() => isoForPreset(range, customFrom, customTo), [range, customFrom, customTo]);

  const basePath = useMemo(() => {
    if (useAdminEndpoint) return '/api/v1/admin/audit-events';
    return `/api/v1/orgs/${currentOrgId}/audit-events`;
  }, [useAdminEndpoint, currentOrgId]);

  const buildParams = useCallback(
    (cursor?: string, since?: string) => {
      const p = new URLSearchParams();
      p.set('limit', '50');
      p.set('from', rangeValues.from);
      p.set('to', rangeValues.to);
      if (category) p.set('category', category);
      if (action) p.set('action', action);
      if (outcome) p.set('outcome', outcome);
      if (debouncedActor && canSeeOrg) p.set('actor_email', debouncedActor);
      if (debouncedSearch.trim()) p.set('q', debouncedSearch.trim());
      if (cursor) p.set('cursor', cursor);
      if (since) p.set('since', since);
      if (useAdminEndpoint && adminOrgId) p.set('org_id', adminOrgId);
      return p.toString();
    },
    [rangeValues, category, action, outcome, debouncedActor, canSeeOrg, debouncedSearch, useAdminEndpoint, adminOrgId],
  );

  const loadFirst = useCallback(async () => {
    if (!basePath || basePath.includes('undefined')) return;
    const seq = ++requestSeq.current;
    setLoading(true);
    setError('');
    try {
      const res = await apiClient.get<ListResponse>(`${basePath}?${buildParams()}`);
      if (seq !== requestSeq.current) return;
      const list = res.events || [];
      setEvents(list);
      setNextCursor(res.next_cursor || '');
      latestRef.current = list[0]?.occurred_at || '';
    } catch (err: any) {
      if (seq !== requestSeq.current) return;
      setError(err?.message || 'Failed to load audit events');
      setEvents([]);
      setNextCursor('');
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [basePath, buildParams]);

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const res = await apiClient.get<ListResponse>(`${basePath}?${buildParams(nextCursor)}`);
      setEvents((prev) => [...prev, ...(res.events || [])]);
      setNextCursor(res.next_cursor || '');
    } catch (err: any) {
      setError(err?.message || 'Failed to load more events');
    } finally {
      setLoadingMore(false);
    }
  }, [basePath, buildParams, nextCursor, loadingMore]);

  // Initial load whenever the query changes.
  useEffect(() => {
    void loadFirst();
  }, [loadFirst]);

  // Load facets and metadata on mount (and when the org changes).
  useEffect(() => {
    if (!currentOrgId) return;
    let cancelled = false;
    (async () => {
      try {
        const qs = new URLSearchParams({ from: rangeValues.from, to: rangeValues.to }).toString();
        const f = await apiClient.get<{ facets: Facets }>(`${basePath}/facets?${qs}`);
        if (!cancelled) setFacets(f.facets);
      } catch {
        /* facets are optional */
      }
      try {
        const m = await apiClient.get<Meta>(`/api/v1/orgs/${currentOrgId}/audit-events/meta`);
        if (!cancelled) setMeta(m);
      } catch {
        /* meta is optional */
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentOrgId, basePath]);

  useEffect(() => {
    if (isSuperAdmin) {
      setIntegrityOrg(String(currentOrgId || ''));
      if (!allOrganizations.length) loadAllOrgs();
    }
  }, [isSuperAdmin, currentOrgId, allOrganizations.length, loadAllOrgs]);

  // Live tail: refresh only new rows every 5s, and only while the tab is visible.
  useEffect(() => {
    if (!live || !basePath) return;
    const tick = async () => {
      if (document.hidden || inFlight.current) return;
      inFlight.current = true;
      try {
        const base = latestRef.current || new Date(Date.now() - 60_000).toISOString();
        const overlap = new Date(new Date(base).getTime() - 1000).toISOString();
        const res = await apiClient.get<ListResponse>(`${basePath}?${buildParams(undefined, overlap)}`);
        const incoming = res.events || [];
        if (incoming.length) {
          setEvents((prev) => {
            const seen = new Set(prev.map((e) => e.id));
            const merged = [...incoming.filter((e) => !seen.has(e.id)), ...prev];
            merged.sort((a, b) => (a.occurred_at < b.occurred_at ? 1 : a.occurred_at > b.occurred_at ? -1 : b.id - a.id));
            latestRef.current = merged[0]?.occurred_at || latestRef.current;
            return merged;
          });
        }
      } catch {
        /* transient poll failures are fine */
      } finally {
        inFlight.current = false;
      }
    };
    const id = window.setInterval(tick, 5000);
    return () => window.clearInterval(id);
  }, [live, basePath, buildParams]);

  const refresh = () => {
    latestRef.current = '';
    void loadFirst();
  };

  useEffect(() => {
    if (tab !== 'ai' || !basePath) return;
    let cancelled = false;
    setAiLoading(true);
    (async () => {
      try {
        const qs = new URLSearchParams({ from: rangeValues.from, to: rangeValues.to, limit: '200' }).toString();
        const aiBase = `/api/v1/orgs/${currentOrgId}/audit-events`;
        const res = await apiClient.get<{ rows: AIUsageRow[] }>(`${aiBase}/ai-usage?${qs}`);
        if (!cancelled) setAiRows(res.rows || []);
      } catch (err: any) {
        if (!cancelled) setError(err?.message || 'Failed to load AI usage');
      } finally {
        if (!cancelled) setAiLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [tab, basePath, rangeValues, currentOrgId]);

  const exportAudit = async (format: 'csv' | 'ndjson' | 'package') => {
    if (!basePath) return;
    setExporting(format);
    setError('');
    try {
      const res = await authFetch(`${basePath}/export?${buildParams().replace(/(^|&)limit=[^&]*/, '')}&format=${format}`);
      if (!res.ok) {
        const text = await res.text().catch(() => '');
        throw new Error(text || `Export failed (${res.status})`);
      }
      const blob = await res.blob();
      const cd = res.headers.get('Content-Disposition') || '';
      const match = cd.match(/filename="?([^";]+)"?/i);
      const ext = format === 'csv' ? 'csv' : format === 'ndjson' ? 'ndjson' : 'zip';
      downloadBlob(blob, match?.[1] || `livereview-audit.${ext}`);
    } catch (err: any) {
      setError(err?.message || 'Export failed');
    } finally {
      setExporting('');
    }
  };

  const downloadArchive = async () => {
    if (!currentOrgId) return;
    const month = window.prompt('Archive month (YYYY-MM):');
    if (!month || !/^\d{4}-\d{2}$/.test(month)) return;
    setError('');
    try {
      const res = await authFetch(`/api/v1/orgs/${currentOrgId}/audit-events/archive?month=${encodeURIComponent(month)}`);
      if (!res.ok) {
        const text = await res.text().catch(() => '');
        throw new Error(text || `Archive download failed (${res.status})`);
      }
      const blob = await res.blob();
      downloadBlob(blob, `livereview-audit-${currentOrgId}-${month}.ndjson.gz`);
    } catch (err: any) {
      setError(err?.message || 'Archive download failed');
    }
  };

  const openSettings = async () => {
    setShowSettings(true);
    if (settings) return;
    try {
      const cfg = await apiClient.get<AuditSettings>('/api/v1/admin/audit-settings');
      setSettings(cfg);
    } catch (err: any) {
      setError(err?.message || 'Failed to load audit settings');
    }
  };

  const saveSettings = async () => {
    if (!settings) return;
    setSavingSettings(true);
    setError('');
    try {
      const saved = await apiClient.put<AuditSettings>('/api/v1/admin/audit-settings', settings);
      setSettings(saved);
      let m: Meta | null = null;
      try {
        m = await apiClient.get<Meta>(`/api/v1/orgs/${currentOrgId}/audit-events/meta`);
      } catch {
        m = null;
      }
      if (m) setMeta(m);
    } catch (err: any) {
      setError(err?.message || 'Failed to save audit settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const runMaintenance = async () => {
    setRunningMaintenance(true);
    setMaintenanceMsg('');
    setError('');
    try {
      await apiClient.post('/api/v1/admin/audit-maintenance/run', {});
      setMaintenanceMsg('Maintenance cycle started. Partitions, archives, and the anchor will update in the background.');
    } catch (err: any) {
      setError(err?.message || 'Failed to start audit maintenance');
    } finally {
      setRunningMaintenance(false);
    }
  };

  const runIntegrity = async () => {
    const orgId = Number(integrityOrg);
    if (!orgId) return;
    setCheckingIntegrity(true);
    setIntegrity(null);
    setError('');
    try {
      const report = await apiClient.get<IntegrityReport>(`/api/v1/admin/audit-events/integrity?org_id=${orgId}`);
      setIntegrity(report);
    } catch (err: any) {
      setError(err?.message || 'Integrity check failed');
    } finally {
      setCheckingIntegrity(false);
    }
  };

  const retentionWarning = meta
    ? `Events are retained for ${meta.retention_days} days total; the most recent ${meta.hot_days} days are instantly searchable.`
    : '';

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-3xl font-bold text-white mb-2">Audit log</h1>
          <p className="text-slate-300">
            {canSeeOrg
              ? 'Every privileged action in this organization, with actor, source, and outcome.'
              : 'Your account activity in this organization.'}
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <label className="flex items-center gap-2 text-xs text-slate-300">
            <input type="checkbox" checked={live} onChange={(e) => setLive(e.target.checked)} />
            Live
          </label>
          <Button size="sm" variant="outline" onClick={refresh}>Refresh</Button>
          <Button size="sm" variant="outline" onClick={() => void exportAudit('csv')} disabled={!!exporting}>
            {exporting === 'csv' ? 'Exporting…' : 'CSV'}
          </Button>
          <Button size="sm" variant="outline" onClick={() => void exportAudit('ndjson')} disabled={!!exporting}>
            {exporting === 'ndjson' ? 'Exporting…' : 'NDJSON'}
          </Button>
          <Button size="sm" variant="outline" onClick={() => void exportAudit('package')} disabled={!!exporting}>
            {exporting === 'package' ? 'Building…' : 'Evidence package'}
          </Button>
          {canDownloadArchive && (
            <Button size="sm" variant="outline" onClick={() => void downloadArchive()}>Archive</Button>
          )}
          {isSuperAdmin && (
            <Button size="sm" variant="outline" onClick={() => void openSettings()}>Retention</Button>
          )}
        </div>
      </div>

      <div className="space-y-4">
      {error && <Alert variant="error" onClose={() => setError('')}>{error}</Alert>}

      <div className="flex items-center gap-2">
        <button
          onClick={() => setTab('activity')}
          className={`px-3 py-1.5 rounded-md text-sm ${tab === 'activity' ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
        >
          Activity
        </button>
        <button
          onClick={() => setTab('ai')}
          className={`px-3 py-1.5 rounded-md text-sm ${tab === 'ai' ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
        >
          AI usage
        </button>
      </div>

      {retentionWarning && (
        <p className="text-xs text-slate-400">
          {retentionWarning}
          {meta?.archive_enabled ? ' Older months are archived and downloadable.' : ''}
        </p>
      )}
      {!canSeeOrg && (
        <p className="text-xs text-slate-400">
          You are viewing your own activity. Ask an organization owner for the full log.
        </p>
      )}

      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          {(Object.keys(RANGE_LABELS) as Array<Exclude<RangePreset, 'custom'>>).map((p) => (
            <button
              key={p}
              onClick={() => setRange(p)}
              className={`px-3 py-1.5 rounded-md text-sm ${range === p ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
            >
              {RANGE_LABELS[p]}
            </button>
          ))}
          <button
            onClick={() => setRange('custom')}
            className={`px-3 py-1.5 rounded-md text-sm ${range === 'custom' ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700'}`}
          >
            Custom
          </button>
          {range === 'custom' && (
            <div className="flex items-center gap-2">
              <input type="datetime-local" value={customFrom} onChange={(e) => setCustomFrom(e.target.value)}
                className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100" />
              <span className="text-slate-500 text-xs">to</span>
              <input type="datetime-local" value={customTo} onChange={(e) => setCustomTo(e.target.value)}
                className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100" />
            </div>
          )}
        </div>

        {tab === 'activity' && (
        <div className="flex flex-wrap items-center gap-2">
          <select value={category} onChange={(e) => setCategory(e.target.value)}
            className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100">
            <option value="">All categories</option>
            {(facets?.categories || []).map((c) => (
              <option key={c.value} value={c.value}>{c.label || CATEGORY_LABELS[c.value] || c.value} ({c.count})</option>
            ))}
          </select>
          <select value={action} onChange={(e) => setAction(e.target.value)}
            className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100 max-w-[220px]">
            <option value="">All actions</option>
            {(facets?.actions || []).map((a) => (
              <option key={a.value} value={a.value}>{a.label || a.value} ({a.count})</option>
            ))}
          </select>
          <select value={outcome} onChange={(e) => setOutcome(e.target.value)}
            className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100">
            <option value="">Any outcome</option>
            <option value="success">Success</option>
            <option value="failure">Failure</option>
            <option value="denied">Denied</option>
            <option value="error">Error</option>
          </select>
          {canSeeOrg && (
            <input value={actorEmail} onChange={(e) => setActorEmail(e.target.value)} placeholder="Actor email"
              className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100 w-48" />
          )}
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search action, resource, path…"
            className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100 w-64" />
          {isSuperAdmin && (
            <>
              <label className="flex items-center gap-1 text-sm text-slate-300">
                <input type="checkbox" checked={allOrgs} onChange={(e) => setAllOrgs(e.target.checked)} />
                All organizations
              </label>
              {allOrgs && (
                <select value={adminOrgId} onChange={(e) => setAdminOrgId(e.target.value)}
                  className="bg-slate-800 border border-slate-600 rounded-md px-3 py-1.5 text-sm text-slate-100">
                  <option value="">All orgs</option>
                  {(allOrganizations || []).map((o: any) => (
                    <option key={o.id} value={String(o.id)}>{o.name || o.org_name || o.id}</option>
                  ))}
                </select>
              )}
            </>
          )}
        </div>
        )}
      </div>

      {showSettings && isSuperAdmin && (
        <Card className="p-4 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Retention & integrity</h2>
            <Button size="sm" variant="ghost" onClick={() => setShowSettings(false)}>Close</Button>
          </div>
          {!settings ? (
            <Spinner />
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              <label className="text-xs text-slate-300 space-y-1">
                <span>Hot window (days, instantly searchable)</span>
                <Input type="number" value={settings.hot_days}
                  onChange={(e) => setSettings({ ...settings, hot_days: Number(e.target.value) })} />
              </label>
              <label className="text-xs text-slate-300 space-y-1">
                <span>Total retention (days)</span>
                <Input type="number" value={settings.retention_days}
                  onChange={(e) => setSettings({ ...settings, retention_days: Number(e.target.value) })} />
              </label>
              <label className="text-xs text-slate-300 space-y-1">
                <span>Maintenance cron (UTC)</span>
                <Input value={settings.cron_expression}
                  onChange={(e) => setSettings({ ...settings, cron_expression: e.target.value })} />
              </label>
              <div className="flex flex-col gap-2 text-xs text-slate-300">
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={settings.archive_enabled}
                    onChange={(e) => setSettings({ ...settings, archive_enabled: e.target.checked })} />
                  Archive months beyond the hot window
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={settings.hash_chain_enabled}
                    onChange={(e) => setSettings({ ...settings, hash_chain_enabled: e.target.checked })} />
                  Hash-chain every event
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={settings.anchor_enabled}
                    onChange={(e) => setSettings({ ...settings, anchor_enabled: e.target.checked })} />
                  Write a daily anchor file
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={settings.enabled}
                    onChange={(e) => setSettings({ ...settings, enabled: e.target.checked })} />
                  Maintenance job enabled
                </label>
              </div>
            </div>
          )}
          {settings && (
            <>
              <div className="flex items-center justify-end gap-2">
                <Button size="sm" variant="outline" onClick={() => void runMaintenance()} disabled={runningMaintenance}>
                  {runningMaintenance ? 'Starting…' : 'Run maintenance now'}
                </Button>
                <Button size="sm" onClick={() => void saveSettings()} disabled={savingSettings}>
                  {savingSettings ? 'Saving…' : 'Save policy'}
                </Button>
              </div>
              {maintenanceMsg && <p className="text-xs text-emerald-300">{maintenanceMsg}</p>}
            </>
          )}
          <div className="border-t border-slate-700 pt-3 space-y-2">
            <h3 className="text-xs font-medium text-slate-200">Verify hash chain</h3>
            <div className="flex items-center gap-2">
              <Input value={integrityOrg} onChange={(e) => setIntegrityOrg(e.target.value)} placeholder="Org ID" />
              <Button size="sm" variant="outline" onClick={() => void runIntegrity()} disabled={checkingIntegrity}>
                {checkingIntegrity ? 'Checking…' : 'Check'}
              </Button>
            </div>
            {integrity && (
              <div className={`text-xs rounded border px-3 py-1.5 ${integrity.intact ? 'border-emerald-700 bg-emerald-900/20 text-emerald-300' : 'border-red-700 bg-red-900/20 text-red-300'}`}>
                {integrity.intact ? 'Chain intact' : `Chain broken at seq ${integrity.first_broken_seq}: ${integrity.first_broken_reason}`}
                {' '}— verified {integrity.verified} of {integrity.rows} rows.
                {integrity.chain_start && integrity.chain_end && (
                  <span className="block text-slate-400 mt-1">
                    {fmtTime(integrity.chain_start)} → {fmtTime(integrity.chain_end)}
                  </span>
                )}
              </div>
            )}
          </div>
        </Card>
      )}

      {tab === 'activity' ? (
        <div>
          <div className="bg-[#1F2836] rounded-lg border border-slate-700 overflow-hidden">
            {loading ? (
              <div className="flex items-center justify-center py-24">
                <div className="text-center">
                  <Spinner size="lg" className="mb-4" />
                  <p className="text-slate-300">Loading audit events...</p>
                </div>
              </div>
            ) : events.length === 0 ? (
              <div className="px-6 py-16 text-center">
                <h3 className="text-xl font-medium text-slate-300">No audit events found</h3>
                <p className="text-slate-400 mt-2">Try a wider date range or clear the filters.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-[1100px] table-fixed">
                  <colgroup>
                    <col style={{ width: '12%' }} />
                    <col style={{ width: '23%' }} />
                    <col style={{ width: '22%' }} />
                    <col style={{ width: '11%' }} />
                    <col style={{ width: '12%' }} />
                    <col style={{ width: '20%' }} />
                  </colgroup>
                  <TableHead>
                    <TableHeaderCell>Time</TableHeaderCell>
                    <TableHeaderCell>Actor</TableHeaderCell>
                    <TableHeaderCell>Action</TableHeaderCell>
                    <TableHeaderCell>Resource</TableHeaderCell>
                    <TableHeaderCell>Outcome</TableHeaderCell>
                    <TableHeaderCell>Source</TableHeaderCell>
                  </TableHead>
                  <TableBody>
                    {events.map((ev) => (
                      <TableRow
                        key={`${ev.id}-${ev.occurred_at}`}
                        onClick={() => setSelected(ev)}
                        className="cursor-pointer"
                      >
                        <TableCell>
                          <div className="text-sm text-white">{fmtDate(ev.occurred_at)}</div>
                          <div className="text-xs text-slate-400">{fmtClock(ev.occurred_at)}</div>
                        </TableCell>
                        <TableCell className="text-slate-200">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${ev.severity === 'critical' ? 'bg-red-400' : ev.severity === 'warning' ? 'bg-amber-400' : 'bg-slate-500'}`} />
                            <span className="min-w-0 truncate">{fmtActor(ev)}</span>
                          </div>
                          <div className="text-[11px] text-slate-500">
                            {ev.actor_kind}
                            {ev.auth_method && ev.auth_method !== ev.actor_kind ? ` · ${ev.auth_method}` : ''}
                            {ev.actor_capacity === 'admin' ? ' · admin' : ''}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className={SEVERITY_STYLES[ev.severity] || 'text-slate-200'}>{actionLabel(ev)}</div>
                          <div className="text-[11px] text-slate-500 font-mono">{ev.action}</div>
                        </TableCell>
                        <TableCell className="text-slate-300">
                          {ev.resource_type ? `${ev.resource_type}${ev.resource_id ? ` #${ev.resource_id}` : ''}` : '—'}
                        </TableCell>
                        <TableCell>
                          <span className={`inline-block rounded-md border px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ${OUTCOME_STYLES[ev.outcome] || 'border-slate-700 text-slate-300'}`}>
                            {ev.outcome}{ev.status_code ? ` ${ev.status_code}` : ''}
                          </span>
                        </TableCell>
                        <TableCell className="text-slate-400">
                          <div>{ev.ip || '—'}</div>
                          <div className="text-[11px] text-slate-500">
                            {ev.source || 'api'}{ev.org_id && isSuperAdmin ? ` · org ${ev.org_id}` : ''}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
            {!loading && events.length > 0 && (
              <div className="flex items-center justify-between gap-3 border-t border-slate-700 px-6 py-3 text-xs text-slate-400">
                <span>{events.length.toLocaleString()} events loaded · UTC stored, local display</span>
                {nextCursor ? (
                  <Button size="sm" variant="outline" onClick={() => void loadMore()} disabled={loadingMore}>
                    {loadingMore ? 'Loading…' : 'Load older events'}
                  </Button>
                ) : (
                  <span>End of results</span>
                )}
              </div>
            )}
          </div>
        </div>
      ) : (
        <div>
          <div className="bg-[#1F2836] rounded-lg border border-slate-700 overflow-hidden">
            {aiLoading ? (
              <div className="flex items-center justify-center py-24">
                <div className="text-center">
                  <Spinner size="lg" className="mb-4" />
                  <p className="text-slate-300">Loading AI usage...</p>
                </div>
              </div>
            ) : aiRows.length === 0 ? (
              <div className="px-6 py-16 text-center">
                <h3 className="text-xl font-medium text-slate-300">No AI/model usage found</h3>
                <p className="text-slate-400 mt-2">Review runs are recorded when they consume tokens; chat turns are recorded after each reply.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table className="min-w-[1100px] table-fixed">
                  <colgroup>
                    <col style={{ width: '13%' }} />
                    <col style={{ width: '23%' }} />
                    <col style={{ width: '15%' }} />
                    <col style={{ width: '15%' }} />
                    <col style={{ width: '10%' }} />
                    <col style={{ width: '16%' }} />
                    <col style={{ width: '8%' }} />
                  </colgroup>
                  <TableHead>
                    <TableHeaderCell>Time</TableHeaderCell>
                    <TableHeaderCell>Provider / model</TableHeaderCell>
                    <TableHeaderCell>Purpose</TableHeaderCell>
                    <TableHeaderCell>Tokens</TableHeaderCell>
                    <TableHeaderCell>Cost</TableHeaderCell>
                    <TableHeaderCell>Actor</TableHeaderCell>
                    <TableHeaderCell>Review</TableHeaderCell>
                  </TableHead>
                  <TableBody>
                    {aiRows.map((r, i) => (
                      <TableRow key={`${r.accounted_at}-${i}`}>
                        <TableCell>
                          <div className="text-sm text-white">{fmtDate(r.accounted_at)}</div>
                          <div className="text-xs text-slate-400">{fmtClock(r.accounted_at)}</div>
                        </TableCell>
                        <TableCell className="text-slate-200">
                          <div>{r.provider || '—'}{r.model ? ` / ${r.model}` : ''}</div>
                          <div className="text-[11px] text-slate-500">{r.operation_type}{r.trigger_source ? ` · ${r.trigger_source}` : ''}</div>
                        </TableCell>
                        <TableCell className="text-slate-300">{r.operation_type || '—'}</TableCell>
                        <TableCell className="text-slate-300">{r.input_tokens ?? 0} in / {r.output_tokens ?? 0} out</TableCell>
                        <TableCell className="text-slate-300">{typeof r.cost_usd === 'number' ? `$${r.cost_usd.toFixed(4)}` : '—'}</TableCell>
                        <TableCell className="text-slate-300">{r.actor_email || (r.user_id ? `user #${r.user_id}` : r.actor_kind || 'system')}</TableCell>
                        <TableCell className="text-slate-300">{r.review_id ? `#${r.review_id}` : '—'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
            {!aiLoading && aiRows.length > 0 && (
              <div className="flex items-center justify-between gap-3 border-t border-slate-700 px-6 py-3 text-xs text-slate-400">
                <span>Provider, model, tokens, and cost · {aiRows.length.toLocaleString()} rows</span>
                <span>UTC stored, local display</span>
              </div>
            )}
          </div>
        </div>
      )}
      </div>

      {selected && (
        <div className="fixed inset-0 z-40 bg-black/50" onClick={() => setSelected(null)}>
          <div className="absolute right-0 top-0 h-full w-full max-w-2xl bg-slate-950 border-l border-slate-700 overflow-y-auto"
            onClick={(e) => e.stopPropagation()}>
            <div className="sticky top-0 bg-slate-950 border-b border-slate-800 px-4 py-3 flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-medium text-white">{actionLabel(selected)}</h2>
                <p className="text-[11px] text-slate-400 font-mono">{selected.action}</p>
              </div>
              <Button size="sm" variant="ghost" onClick={() => setSelected(null)}>Close</Button>
            </div>
            <div className="p-4 space-y-4 text-xs">
              <div className="grid grid-cols-2 gap-3">
                <Field label="When" value={fmtTime(selected.occurred_at)} />
                <Field label="Outcome" value={`${selected.outcome}${selected.status_code ? ` (${selected.status_code})` : ''}`} />
                <Field label="Actor" value={fmtActor(selected)} />
                <Field label="Actor kind" value={`${selected.actor_kind}${selected.actor_capacity ? ` / ${selected.actor_capacity}` : ''}`} />
                <Field label="Category" value={CATEGORY_LABELS[selected.category] || selected.category} />
                <Field label="Severity" value={selected.severity} />
                <Field label="Resource" value={selected.resource_type ? `${selected.resource_type}${selected.resource_id ? ` #${selected.resource_id}` : ''}` : '—'} />
                <Field label="IP" value={selected.ip || '—'} />
                <Field label="Method / path" value={`${selected.method || ''} ${selected.path || ''}`.trim() || '—'} />
                <Field label="Request ID" value={selected.request_id || '—'} />
                <Field label="API key" value={selected.api_key_id ? String(selected.api_key_id) : '—'} />
                <Field label="Chain seq" value={selected.seq ? String(selected.seq) : '—'} />
              </div>
              <div>
                <div className="text-slate-400 mb-1">Details</div>
                <pre className="bg-slate-900 border border-slate-800 rounded p-3 overflow-x-auto text-[11px] text-slate-200">
                  {JSON.stringify(selected.details || {}, null, 2)}
                </pre>
              </div>
              {selected.user_agent && (
                <div>
                  <div className="text-slate-400 mb-1">User agent</div>
                  <div className="text-slate-300 break-all">{selected.user_agent}</div>
                </div>
              )}
              {selected.hash && (
                <div>
                  <div className="text-slate-400 mb-1">Hash</div>
                  <div className="text-slate-400 font-mono break-all text-[10px]">{selected.hash}</div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

const Field: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div>
    <div className="text-slate-500">{label}</div>
    <div className="text-slate-200 break-all">{value}</div>
  </div>
);

export default AuditLog;
