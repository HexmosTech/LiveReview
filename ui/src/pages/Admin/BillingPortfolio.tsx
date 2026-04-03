import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';

type PortfolioSummary = {
  total_orgs: number;
  active_orgs: number;
  total_billable_loc: number;
  total_operations: number;
  net_collected_cents: number;
  failed_payments: number;
  last_accounted_at?: string;
};

type PortfolioOrg = {
  org_id: number;
  org_name: string;
  current_plan_code?: string | null;
  loc_used_month?: number | null;
  loc_blocked?: boolean | null;
  billing_period_end?: string | null;
  total_billable_loc: number;
  operation_count: number;
  last_accounted_at?: string | null;
  net_collected_cents: number;
  failed_payments: number;
};

type OrgMemberUsage = {
  user_id?: number | null;
  actor_email?: string | null;
  actor_kind: string;
  total_billable_loc: number;
  operation_count: number;
  last_accounted_at?: string | null;
  usage_share_percent: number;
};

type OrgUsageOperation = {
  operation_type: string;
  trigger_source: string;
  operation_id: string;
  billable_loc: number;
  actor_email?: string;
  actor_kind?: string;
  accounted_at: string;
};

type OrgUsageDetails = {
  org_id: number;
  summary: {
    period_start: string;
    period_end: string;
    total_billable_loc: number;
    accounted_operations: number;
    latest_accounted_at?: string | null;
  };
  operations: {
    items: OrgUsageOperation[];
  };
};

const formatDate = (value?: string | null): string => {
  if (!value) return 'N/A';
  const dt = new Date(value);
  if (Number.isNaN(dt.getTime())) return 'N/A';
  return dt.toLocaleString();
};

const formatCurrency = (cents: number): string => {
  return `$${(Number(cents || 0) / 100).toFixed(2)}`;
};

const BillingPortfolio: React.FC = () => {
  const navigate = useNavigate();
  const { isSuperAdmin } = useOrgContext();

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>('');
  const [errorStatus, setErrorStatus] = useState<number | null>(null);
  const [summary, setSummary] = useState<PortfolioSummary | null>(null);
  const [orgs, setOrgs] = useState<PortfolioOrg[]>([]);
  const [selectedOrgId, setSelectedOrgId] = useState<number | null>(null);
  const [members, setMembers] = useState<OrgMemberUsage[]>([]);
  const [usageDetails, setUsageDetails] = useState<OrgUsageDetails | null>(null);

  const selectedOrg = useMemo(() => orgs.find((item) => item.org_id === selectedOrgId) || null, [orgs, selectedOrgId]);
  const isEndpointUnavailable = errorStatus === 404;

  const loadPortfolio = async () => {
    setLoading(true);
    setError('');
    setErrorStatus(null);
    try {
      const [summaryResp, orgsResp] = await Promise.all([
        apiClient.get<PortfolioSummary>('/admin/billing/portfolio/summary'),
        apiClient.get<{ organizations: PortfolioOrg[] }>('/admin/billing/portfolio/orgs?limit=100&offset=0'),
      ]);

      const nextOrgs = orgsResp?.organizations || [];
      setSummary(summaryResp || null);
      setOrgs(nextOrgs);

      if (nextOrgs.length > 0) {
        setSelectedOrgId((current) => current || nextOrgs[0].org_id);
      }
    } catch (err: any) {
      setErrorStatus(Number(err?.status || 0) || null);
      if (Number(err?.status || 0) === 404) {
        setError('Admin billing portfolio endpoints are not available on this backend deployment yet.');
      } else {
        setError(err?.message || 'Failed to load billing portfolio');
      }
      setSummary(null);
      setOrgs([]);
      setSelectedOrgId(null);
    } finally {
      setLoading(false);
    }
  };

  const loadOrgDetails = async (orgId: number) => {
    if (!orgId) return;
    try {
      const [membersResp, usageResp] = await Promise.all([
        apiClient.get<{ members: OrgMemberUsage[] }>(`/admin/billing/portfolio/orgs/${orgId}/members?limit=30&offset=0`),
        apiClient.get<OrgUsageDetails>(`/admin/billing/portfolio/orgs/${orgId}/usage?limit=20&offset=0`),
      ]);
      setMembers(membersResp?.members || []);
      setUsageDetails(usageResp || null);
    } catch (err: any) {
      setMembers([]);
      setUsageDetails(null);
      setError(err?.message || 'Failed to load organization billing details');
    }
  };

  useEffect(() => {
    if (!isSuperAdmin) return;
    loadPortfolio();
  }, [isSuperAdmin]);

  useEffect(() => {
    if (!selectedOrgId) return;
    loadOrgDetails(selectedOrgId);
  }, [selectedOrgId]);

  if (!isSuperAdmin) {
    return (
      <div className="p-6">
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white">Superadmin Access Required</h2>
          <p className="text-slate-300 mt-2">This billing portfolio page is available only for superadmin users.</p>
          <button
            className="mt-4 px-4 py-2 rounded bg-blue-600 hover:bg-blue-500 text-white"
            onClick={() => navigate('/dashboard')}
          >
            Back to Dashboard
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-white">Billing Portfolio</h1>
          <p className="text-slate-300">Superadmin view for usage, payment health, and net collections.</p>
        </div>
        <button
          className="px-4 py-2 rounded bg-slate-700 hover:bg-slate-600 text-white"
          onClick={loadPortfolio}
          disabled={loading}
        >
          {loading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      {error && (
        <div className="bg-red-900/30 border border-red-600/40 rounded-lg p-3 text-red-200 text-sm">{error}</div>
      )}

      {isEndpointUnavailable && (
        <div className="bg-slate-800/70 border border-slate-700 rounded-lg p-4">
          <p className="text-sm text-slate-200 font-medium">Portfolio API is not available on the connected backend.</p>
          <p className="text-xs text-slate-400 mt-1">
            Global cross-organization portfolio metrics require the latest backend deployment that includes /admin/billing/portfolio routes.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              className="px-3 py-2 rounded bg-blue-600 hover:bg-blue-500 text-white text-sm"
              onClick={() => navigate('/settings-subscriptions-breakdown')}
            >
              Open Breakdown Tab
            </button>
            <button
              className="px-3 py-2 rounded bg-slate-700 hover:bg-slate-600 text-white text-sm"
              onClick={loadPortfolio}
              disabled={loading}
            >
              {loading ? 'Retrying...' : 'Retry Portfolio'}
            </button>
          </div>
        </div>
      )}

      {!loading && !summary && orgs.length === 0 && (
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
          <p className="text-sm text-slate-200 font-medium">No portfolio data to display.</p>
          <p className="text-xs text-slate-400 mt-1">
            {errorStatus === 404
              ? 'This environment does not expose /admin/billing/portfolio APIs yet. Use Subscription settings for org-level billing details.'
              : 'No organizations with billing portfolio data were returned.'}
          </p>
          <button
            className="mt-3 px-3 py-2 rounded bg-blue-600 hover:bg-blue-500 text-white text-sm"
            onClick={() => navigate('/settings-subscriptions-overview')}
          >
            Open Subscription Billing
          </button>
        </div>
      )}

      {summary && (
        <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-6 gap-3">
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Active Orgs</p>
            <p className="text-white font-semibold text-lg">{summary.active_orgs}</p>
          </div>
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Total Orgs</p>
            <p className="text-white font-semibold text-lg">{summary.total_orgs}</p>
          </div>
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Billable LOC</p>
            <p className="text-white font-semibold text-lg">{summary.total_billable_loc.toLocaleString()}</p>
          </div>
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Operations</p>
            <p className="text-white font-semibold text-lg">{summary.total_operations.toLocaleString()}</p>
          </div>
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Net Collected</p>
            <p className="text-white font-semibold text-lg">{formatCurrency(summary.net_collected_cents)}</p>
          </div>
          <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
            <p className="text-slate-400 text-xs">Failed Payments</p>
            <p className="text-white font-semibold text-lg">{summary.failed_payments.toLocaleString()}</p>
          </div>
        </div>
      )}

      {!isEndpointUnavailable && <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
          <h2 className="text-white font-semibold mb-3">Organizations</h2>
          <div className="overflow-x-auto border border-slate-700 rounded-lg">
            <table className="min-w-full text-xs text-left">
              <thead className="bg-slate-900/80 text-slate-300">
                <tr>
                  <th className="px-3 py-2">Org</th>
                  <th className="px-3 py-2">Plan</th>
                  <th className="px-3 py-2">LOC</th>
                  <th className="px-3 py-2">Net</th>
                  <th className="px-3 py-2">Failed</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700 bg-slate-950/30">
                {orgs.length === 0 && (
                  <tr>
                    <td colSpan={5} className="px-3 py-3 text-slate-400">No organizations found.</td>
                  </tr>
                )}
                {orgs.map((org) => (
                  <tr
                    key={org.org_id}
                    className={selectedOrgId === org.org_id ? 'bg-blue-900/20' : 'cursor-pointer hover:bg-slate-900/40'}
                    onClick={() => setSelectedOrgId(org.org_id)}
                  >
                    <td className="px-3 py-2 text-slate-100">{org.org_name}</td>
                    <td className="px-3 py-2 text-slate-300">{org.current_plan_code || 'N/A'}</td>
                    <td className="px-3 py-2 text-white">{org.total_billable_loc.toLocaleString()}</td>
                    <td className="px-3 py-2 text-white">{formatCurrency(org.net_collected_cents)}</td>
                    <td className="px-3 py-2 text-white">{org.failed_payments}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4 space-y-4">
          <h2 className="text-white font-semibold">Organization Details</h2>
          {!selectedOrg && <p className="text-slate-400 text-sm">Select an organization to inspect details.</p>}
          {selectedOrg && (
            <>
              <div className="bg-slate-900/60 border border-slate-700 rounded p-3 text-sm">
                <p className="text-slate-300">{selectedOrg.org_name}</p>
                <p className="text-slate-400 text-xs mt-1">
                  Last accounted: {formatDate(selectedOrg.last_accounted_at)}
                </p>
                <p className="text-slate-400 text-xs">Billing period end: {formatDate(selectedOrg.billing_period_end)}</p>
              </div>

              <div>
                <p className="text-slate-300 text-sm mb-2">Top Members</p>
                <div className="overflow-x-auto border border-slate-700 rounded-lg">
                  <table className="min-w-full text-xs text-left">
                    <thead className="bg-slate-900/80 text-slate-300">
                      <tr>
                        <th className="px-3 py-2">Member</th>
                        <th className="px-3 py-2">Kind</th>
                        <th className="px-3 py-2">LOC</th>
                        <th className="px-3 py-2">Share</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-700 bg-slate-950/30">
                      {members.length === 0 && (
                        <tr>
                          <td colSpan={4} className="px-3 py-3 text-slate-400">No member usage data.</td>
                        </tr>
                      )}
                      {members.map((member, idx) => (
                        <tr key={`${member.user_id || member.actor_email || member.actor_kind}-${idx}`}>
                          <td className="px-3 py-2 text-slate-100">{member.actor_email || 'System'}</td>
                          <td className="px-3 py-2 text-slate-300">{member.actor_kind || 'unknown'}</td>
                          <td className="px-3 py-2 text-white">{member.total_billable_loc.toLocaleString()}</td>
                          <td className="px-3 py-2 text-white">{member.usage_share_percent.toFixed(2)}%</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              <div>
                <p className="text-slate-300 text-sm mb-2">Recent Usage Operations</p>
                <div className="overflow-x-auto border border-slate-700 rounded-lg">
                  <table className="min-w-full text-xs text-left">
                    <thead className="bg-slate-900/80 text-slate-300">
                      <tr>
                        <th className="px-3 py-2">When</th>
                        <th className="px-3 py-2">Actor</th>
                        <th className="px-3 py-2">Type</th>
                        <th className="px-3 py-2">LOC</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-700 bg-slate-950/30">
                      {(usageDetails?.operations?.items || []).length === 0 && (
                        <tr>
                          <td colSpan={4} className="px-3 py-3 text-slate-400">No operations in current billing period.</td>
                        </tr>
                      )}
                      {(usageDetails?.operations?.items || []).map((item) => (
                        <tr key={`${item.operation_id}-${item.accounted_at}`}>
                          <td className="px-3 py-2 text-slate-300">{formatDate(item.accounted_at)}</td>
                          <td className="px-3 py-2 text-slate-100">{item.actor_email || (item.actor_kind === 'system' ? 'System' : 'Unknown')}</td>
                          <td className="px-3 py-2 text-white">{item.operation_type}</td>
                          <td className="px-3 py-2 text-white">{item.billable_loc.toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </>
          )}
        </div>
      </div>}
    </div>
  );
};

export default BillingPortfolio;
