import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Area, AreaChart, Brush, CartesianGrid, Legend, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';

type MultiOption = { value: string; label: string };

// ---- types ----------------------------------------------------------------

type Summary = {
  total_findings: number;
  total_reviews: number;
  critical_count: number;
  high_count: number;
  medium_count: number;
  low_count: number;
  info_count: number;
  high_confidence_count: number;
  medium_confidence_count: number;
  low_confidence_count: number;
};

type DistRow = { dimension: string; value: string; count: number };
type TrendRow = { bucket: string; count: number; review_count: number };
type BreakdownRow = {
  org_id?: number;
  org_name?: string;
  repository: string;
  provider: string;
  count: number;
  review_count?: number;
};
type FindingRow = {
  comment_id: number;
  review_id: number;
  org_id: number;
  repository: string;
  provider: string;
  file_path?: string;
  line_number?: number;
  severity: string;
  confidence: string;
  type: string;
  category: string;
  subcategory: string;
  content: string;
  created_at: string;
};

type FindingsSortBy =
  | 'created_at'
  | 'severity'
  | 'confidence'
  | 'type'
  | 'category'
  | 'subcategory'
  | 'repository'
  | 'provider'
  | 'file_path'
  | 'line_number';

type RelationRow = {
  category: string;
  subcategory: string;
  count: number;
};

type ExportPreview = {
  findings: number;
  severity_distribution: number;
  category_distribution: number;
  trend: number;
  breakdown: number;
};

type Filters = {
  since: string;
  until: string;
  severity: string;
  confidence: string;
  issueType: string;
  category: string;
  subcategory: string;
  repository: string;
  provider: string;
  orgId: string;
  grain: string;
};

// ---- helpers ---------------------------------------------------------------

const severityBadge = (s: string) => {
  const v = (s || '').toLowerCase();
  if (v === 'critical') return 'bg-red-700 text-red-100';
  if (v === 'high' || v === 'error') return 'bg-orange-700 text-orange-100';
  if (v === 'medium' || v === 'warning') return 'bg-yellow-700 text-yellow-100';
  if (v === 'low') return 'bg-blue-800 text-blue-100';
  return 'bg-slate-700 text-slate-200';
};

const confidenceBadge = (s: string) => {
  const v = (s || '').toLowerCase();
  if (v === 'high') return 'bg-emerald-800 text-emerald-100';
  if (v === 'medium') return 'bg-sky-800 text-sky-100';
  return 'bg-slate-700 text-slate-300';
};

const formatDate = (v?: string) => {
  if (!v) return 'N/A';
  try { return new Date(v).toLocaleString(); } catch { return v; }
};

const parseMulti = (v: string): string[] =>
  (v || '')
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean);

const parseYMD = (v: string): Date | null => {
  if (!v) return null;
  const [y, m, d] = v.split('-').map((x) => Number(x));
  if (!y || !m || !d) return null;
  return new Date(y, m - 1, d, 0, 0, 0, 0);
};

const toSafeNumber = (value: unknown): number => {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
};

const startOfDay = (d: Date): Date => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const startOfMonth = (d: Date): Date => new Date(d.getFullYear(), d.getMonth(), 1);
const startOfWeekMon = (d: Date): Date => {
  const c = startOfDay(d);
  const day = c.getDay();
  const delta = day === 0 ? 6 : day - 1;
  c.setDate(c.getDate() - delta);
  return c;
};

const startOfGrain = (d: Date, grain: string): Date => {
  if (grain === 'month') return startOfMonth(d);
  if (grain === 'week') return startOfWeekMon(d);
  return startOfDay(d);
};

const addGrain = (d: Date, grain: string): Date => {
  const next = new Date(d);
  if (grain === 'month') {
    next.setMonth(next.getMonth() + 1);
    return next;
  }
  if (grain === 'week') {
    next.setDate(next.getDate() + 7);
    return next;
  }
  next.setDate(next.getDate() + 1);
  return next;
};

const trendKey = (d: Date): string => {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${dd}`;
};

type FilledTrendRow = { bucket: string; count: number; review_count: number; key: string };

const buildFullRangeTrend = (rows: TrendRow[], since: string, until: string, grain: string): FilledTrendRow[] => {
  if (rows.length === 0 && !since && !until) return [];

  const rowDates = rows
    .map((r) => new Date(r.bucket))
    .filter((d) => !Number.isNaN(d.getTime()));

  const from = parseYMD(since) || rowDates[0] || new Date();
  const to = parseYMD(until) || rowDates[rowDates.length - 1] || from;

  const start = startOfGrain(from, grain);
  const end = startOfDay(to);

  const existingFindings = new Map<string, number>();
  const existingReviews = new Map<string, number>();
  rows.forEach((r) => {
    const d = new Date(r.bucket);
    if (Number.isNaN(d.getTime())) return;
    const key = trendKey(startOfGrain(d, grain));
    existingFindings.set(key, (existingFindings.get(key) || 0) + r.count);
    existingReviews.set(key, (existingReviews.get(key) || 0) + (r.review_count || 0));
  });

  const out: FilledTrendRow[] = [];
  let cursor = new Date(start);
  let guard = 0;
  while (cursor <= end && guard < 5000) {
    const key = trendKey(startOfGrain(cursor, grain));
    out.push({
      bucket: key,
      key,
      count: existingFindings.get(key) || 0,
      review_count: existingReviews.get(key) || 0,
    });
    cursor = addGrain(cursor, grain);
    guard += 1;
  }
  return out;
};

const MultiSelectField = ({
  label,
  options,
  value,
  onChange,
  hint,
}: {
  label: string;
  options: MultiOption[];
  value: string;
  onChange: (next: string) => void;
  hint?: string;
}) => {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const rootRef = useRef<HTMLDivElement | null>(null);
  const selected = parseMulti(value);
  const allSelected = selected.length === 0 || selected.length === options.length;
  const selectedSet = new Set(allSelected ? options.map((o) => o.value) : selected);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, query]);

  const toggle = (target: string) => {
    const next = new Set(allSelected ? options.map((o) => o.value) : selected);
    if (next.has(target)) next.delete(target);
    else next.add(target);
    if (next.size === 0 || next.size === options.length) {
      onChange('');
      return;
    }
    onChange(Array.from(next).join(','));
  };

  const summary = allSelected
    ? `All (${options.length})`
    : `${selected.length} selected`;

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (ev: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(ev.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  return (
    <div ref={rootRef} className="relative">
      <label className="text-slate-400 text-[11px] mb-0.5 block">{label}</label>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full bg-slate-900 border border-slate-600 rounded px-2 py-1 text-left text-xs text-white hover:border-slate-500"
      >
        <span className="flex items-center justify-between gap-2">
          <span className="truncate">{summary}</span>
          <span className="text-slate-400">▾</span>
        </span>
      </button>
      {open && (
        <div className="absolute z-30 mt-1 w-full rounded border border-slate-600 bg-slate-900 shadow-xl p-2 space-y-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search..."
            className="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-100"
          />
          <button
            type="button"
            className="w-full text-left text-xs px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200"
            onClick={() => {
              onChange('');
              setOpen(false);
            }}
          >
            All ({options.length})
          </button>
          <div className="max-h-44 overflow-y-auto space-y-1 pr-1">
            {filtered.map((o) => (
              <label key={o.value} className="flex items-center gap-2 text-xs text-slate-200 px-2 py-1 rounded hover:bg-slate-800 cursor-pointer">
                <input
                  type="checkbox"
                  checked={selectedSet.has(o.value)}
                  onChange={() => toggle(o.value)}
                  className="accent-blue-500"
                />
                <span className="truncate">{o.label}</span>
              </label>
            ))}
            {filtered.length === 0 && <p className="text-[11px] text-slate-500 px-2 py-1">No matches</p>}
          </div>
        </div>
      )}
      {hint && <span className="text-[10px] text-slate-500 mt-0.5 block">{hint}</span>}
    </div>
  );
};

const buildQS = (f: Filters, extra: Record<string, string> = {}): string => {
  const p = new URLSearchParams();
  if (f.since) p.set('since', f.since);
  if (f.until) p.set('until', f.until);
  parseMulti(f.severity).forEach((v) => p.append('severity', v));
  parseMulti(f.confidence).forEach((v) => p.append('confidence', v));
  parseMulti(f.issueType).forEach((v) => p.append('type', v));
  parseMulti(f.category).forEach((v) => p.append('category', v));
  parseMulti(f.subcategory).forEach((v) => p.append('subcategory', v));
  if (f.repository) p.set('repository', f.repository);
  parseMulti(f.provider).forEach((v) => p.append('provider', v));
  if (f.orgId) p.set('org_id', f.orgId);
  Object.entries(extra).forEach(([k, v]) => { if (v) p.set(k, v); });
  const s = p.toString();
  return s ? `?${s}` : '';
};

const dateToInput = (d: Date): string => {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
};

const emptyFilters = (): Filters => ({
  // Sensible defaults: last 30 days through today.
  since: dateToInput(new Date(Date.now() - 30 * 24 * 60 * 60 * 1000)),
  until: dateToInput(new Date()),
  severity: '', confidence: '', issueType: '',
  category: '', subcategory: '',
  repository: '', provider: '',
  orgId: '',
  grain: 'day',
});

const DATASETS = [
  'findings',
  'severity_distribution',
  'category_distribution',
  'trend',
  'breakdown',
] as const;

type DatasetName = typeof DATASETS[number];

// ---- component -------------------------------------------------------------

const TaxonomyReports: React.FC = () => {
  const { isSuperAdmin } = useOrgContext();
  const [mode, setMode] = useState<'overview' | 'explore'>('overview');

  const [filters, setFilters] = useState<Filters>(emptyFilters());
  const [summary, setSummary] = useState<Summary | null>(null);
  const [severityDist, setSeverityDist] = useState<DistRow[]>([]);
  const [categoryDist, setCategoryDist] = useState<DistRow[]>([]);
  const [subcategoryDist, setSubcategoryDist] = useState<DistRow[]>([]);
  const [trend, setTrend] = useState<TrendRow[]>([]);
  const [breakdown, setBreakdown] = useState<BreakdownRow[]>([]);
  const [findings, setFindings] = useState<FindingRow[]>([]);
  const [relations, setRelations] = useState<RelationRow[]>([]);
  const [findingsTotal, setFindingsTotal] = useState(0);
  const [findingsOffset, setFindingsOffset] = useState(0);
  const [findingsSortBy, setFindingsSortBy] = useState<FindingsSortBy>('created_at');
  const [findingsSortDir, setFindingsSortDir] = useState<'asc' | 'desc'>('desc');
  const [findingsColumnFilters, setFindingsColumnFilters] = useState<Record<string, string>>({
    severity: '',
    confidence: '',
    type: '',
    category: '',
    subcategory: '',
    repository: '',
    provider: '',
    file_path: '',
    line_number: '',
    content: '',
    created_at: '',
  });
  const findingsLimit = 25;
  const [loading, setLoading] = useState(false);
  const [exportingDataset, setExportingDataset] = useState<string | null>(null);
  const [showExportDialog, setShowExportDialog] = useState(false);
  const [exportPreview, setExportPreview] = useState<ExportPreview | null>(null);
  const [selectedDatasets, setSelectedDatasets] = useState<Record<DatasetName, boolean>>({
    findings: true,
    severity_distribution: true,
    category_distribution: true,
    trend: true,
    breakdown: true,
  });
  const [exportFormat, setExportFormat] = useState<'csv' | 'xlsx'>('csv');
  const [expandedRows, setExpandedRows] = useState<Record<number, boolean>>({});
  const [error, setError] = useState('');
  const [expandedCategories, setExpandedCategories] = useState<Record<string, boolean>>({});
  const [previewDataset, setPreviewDataset] = useState<DatasetName>('findings');

  const severityOptions: MultiOption[] = useMemo(() => [
    { value: 'Critical', label: 'Critical' },
    { value: 'High', label: 'High' },
    { value: 'Medium', label: 'Medium' },
    { value: 'Low', label: 'Low' },
    { value: 'Info', label: 'Info' },
  ], []);
  const confidenceOptions: MultiOption[] = useMemo(() => [
    { value: 'High', label: 'High' },
    { value: 'Medium', label: 'Medium' },
    { value: 'Low', label: 'Low' },
  ], []);
  const typeOptions: MultiOption[] = useMemo(() => [
    { value: 'Bug', label: 'Bug' },
    { value: 'Risk', label: 'Risk' },
    { value: 'Optimization', label: 'Optimization' },
    { value: 'Code Smell', label: 'Code Smell' },
    { value: 'Best Practice', label: 'Best Practice' },
    { value: 'Technical Debt', label: 'Technical Debt' },
  ], []);
  const providerOptions: MultiOption[] = useMemo(() => [
    { value: 'github', label: 'GitHub' },
    { value: 'gitlab', label: 'GitLab' },
    { value: 'bitbucket', label: 'Bitbucket' },
    { value: 'gitea', label: 'Gitea' },
  ], []);

  const toggleCategory = (cat: string) =>
    setExpandedCategories((prev) => ({ ...prev, [cat]: !prev[cat] }));

  const baseEndpoint = isSuperAdmin ? '/admin/reports/taxonomy' : '/reports/taxonomy';

  const load = useCallback(async (f: Filters, offset = 0, findingsQuery?: { sortBy: FindingsSortBy; sortDir: 'asc' | 'desc'; columnFilters: Record<string, string> }) => {
    setLoading(true);
    setError('');
    try {
      const qs = buildQS(f);
      const trendQs = buildQS(f, { grain: f.grain || 'day' });
      const activeFindingsQuery = findingsQuery || { sortBy: findingsSortBy, sortDir: findingsSortDir, columnFilters: findingsColumnFilters };
      const findingsExtra: Record<string, string> = {
        limit: String(findingsLimit),
        offset: String(offset),
        findings_sort_by: activeFindingsQuery.sortBy,
        findings_sort_dir: activeFindingsQuery.sortDir,
      };
      Object.entries(activeFindingsQuery.columnFilters).forEach(([k, v]) => {
        const trimmed = v.trim();
        if (trimmed) findingsExtra[`findings_filter_${k}`] = trimmed;
      });
      const findingsQs = buildQS(f, findingsExtra);

      const [
        summaryRes,
        sevDistRes, catDistRes, subDistRes,
        trendRes, breakdownRes, findingsRes,
        relationsRes,
      ] = await Promise.all([
        apiClient.get<{ data: Summary }>(`${baseEndpoint}/summary${qs}`),
        apiClient.get<{ data: { rows: DistRow[] } }>(`${baseEndpoint}/distribution/severity${qs}`),
        apiClient.get<{ data: { rows: DistRow[] } }>(`${baseEndpoint}/distribution/category${qs}`),
        apiClient.get<{ data: { rows: DistRow[] } }>(`${baseEndpoint}/distribution/subcategory${qs}`),
        apiClient.get<{ data: { rows: TrendRow[] } }>(`${baseEndpoint}/trend${trendQs}`),
        apiClient.get<{ data: { rows: BreakdownRow[] } }>(`${baseEndpoint}/breakdown${qs}`),
        apiClient.get<{ data: { total: number; rows: FindingRow[] } }>(`${baseEndpoint}/findings${findingsQs}`),
        apiClient.get<{ data: { rows: RelationRow[] } }>(`${baseEndpoint}/relations${qs}`),
      ]);

      setSummary((summaryRes as any)?.data ?? summaryRes);
      setSeverityDist(((sevDistRes as any)?.data?.rows ?? (sevDistRes as any)?.rows) || []);
      setCategoryDist(((catDistRes as any)?.data?.rows ?? (catDistRes as any)?.rows) || []);
      setSubcategoryDist(((subDistRes as any)?.data?.rows ?? (subDistRes as any)?.rows) || []);

      const rawTrend = (((trendRes as any)?.data?.rows ?? (trendRes as any)?.rows) || []) as any[];
      setTrend(rawTrend.map((r) => {
        const mapped = {
          bucket: r.bucket,
          count: toSafeNumber(r.count ?? 0),
          review_count: toSafeNumber(
            r.review_count
              ?? r.reviewCount
              ?? r.ReviewCount
              ?? r.reviews_count
              ?? r.reviewsCount
              ?? r.reviews
              ?? r.total_reviews
              ?? r.totalReviews
              ?? 0,
          ),
        };
        return mapped;
      }));

      const rawBreakdown = (((breakdownRes as any)?.data?.rows ?? (breakdownRes as any)?.rows) || []) as any[];
      setBreakdown(rawBreakdown.map((r) => ({
        ...r,
        count: toSafeNumber(r.count ?? 0),
        review_count: toSafeNumber(
          r.review_count
            ?? r.reviewCount
            ?? r.ReviewCount
            ?? r.reviews_count
            ?? r.reviewsCount
            ?? r.reviews
            ?? r.total_reviews
            ?? r.totalReviews
            ?? 0,
        ),
      })));
      const fData = (findingsRes as any)?.data ?? findingsRes;
      setFindings(fData?.rows || []);
      setFindingsTotal(fData?.total || 0);
      setRelations((((relationsRes as any)?.data?.rows ?? (relationsRes as any)?.rows) || []).filter((r: RelationRow) => r.category && r.subcategory));
      setFindingsOffset(offset);
    } catch (err: any) {
      setError(err?.message || 'Failed to load report data');
    } finally {
      setLoading(false);
    }
  }, [baseEndpoint, findingsColumnFilters, findingsSortBy, findingsSortDir]);

  useEffect(() => { load(filters, 0); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleApply = () => { load(filters, 0); };
  const handleReset = () => { const f = emptyFilters(); setFilters(f); load(f, 0); };

  const applyFindingsQuery = () => {
    load(filters, 0, { sortBy: findingsSortBy, sortDir: findingsSortDir, columnFilters: findingsColumnFilters });
  };

  const resetFindingsQuery = () => {
    const resetFilters = {
      severity: '',
      confidence: '',
      type: '',
      category: '',
      subcategory: '',
      repository: '',
      provider: '',
      file_path: '',
      line_number: '',
      content: '',
      created_at: '',
    };
    setFindingsSortBy('created_at');
    setFindingsSortDir('desc');
    setFindingsColumnFilters(resetFilters);
    load(filters, 0, { sortBy: 'created_at', sortDir: 'desc', columnFilters: resetFilters });
  };

  const relationMap = useMemo(() => {
    const map = new Map<string, string[]>();
    relations.forEach((r) => {
      if (!map.has(r.category)) map.set(r.category, []);
      map.get(r.category)!.push(r.subcategory);
    });
    map.forEach((v, k) => map.set(k, Array.from(new Set(v)).sort((a, b) => a.localeCompare(b))));
    return map;
  }, [relations]);

  const selectedCategories = useMemo(() => parseMulti(filters.category), [filters.category]);

  const categoryOptions = useMemo(() => {
    const fromRelations = Array.from(relationMap.keys());
    const fromDist = categoryDist.map((r) => r.value).filter(Boolean);
    return Array.from(new Set([...fromRelations, ...fromDist])).sort((a, b) => a.localeCompare(b));
  }, [relationMap, categoryDist]);

  const subcategoryOptions = useMemo(() => {
    if (selectedCategories.length > 0) {
      const set = new Set<string>();
      selectedCategories.forEach((c) => {
        (relationMap.get(c) || []).forEach((s) => set.add(s));
      });
      return Array.from(set).sort((a, b) => a.localeCompare(b));
    }
    const fromRelations = relations.map((r) => r.subcategory).filter(Boolean);
    const fromDist = subcategoryDist.map((r) => r.value).filter(Boolean);
    return Array.from(new Set([...fromRelations, ...fromDist])).sort((a, b) => a.localeCompare(b));
  }, [selectedCategories, relationMap, relations, subcategoryDist]);

  const categoryMultiOptions = useMemo(() => categoryOptions.map((v) => ({ value: v, label: v })), [categoryOptions]);
  const subcategoryMultiOptions = useMemo(() => subcategoryOptions.map((v) => ({ value: v, label: v })), [subcategoryOptions]);

  const setF = (key: keyof Filters) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const value = e.target.value;
    setFilters(prev => ({ ...prev, [key]: value }));
  };

  const applyDatePreset = (days: number | 'all') => {
    const now = new Date();
    const until = dateToInput(now);
    const since = days === 'all' ? '' : dateToInput(new Date(Date.now() - days * 24 * 60 * 60 * 1000));
    setFilters((prev) => ({ ...prev, since, until }));
    load({ ...filters, since, until }, 0);
  };

  const loadExportPreview = useCallback(async () => {
    try {
      const qs = buildQS(filters, { grain: filters.grain || 'day' });
      const res = await apiClient.get<{ data: { rows: ExportPreview } }>(`${baseEndpoint}/export/preview${qs}`);
      const rows = ((res as any)?.data?.data?.rows ?? (res as any)?.data?.rows ?? (res as any)?.rows) as ExportPreview;
      setExportPreview(rows);
    } catch {
      setExportPreview(null);
    }
  }, [baseEndpoint, filters]);

  useEffect(() => {
    if (showExportDialog) loadExportPreview();
  }, [showExportDialog, loadExportPreview]);

  const runExport = async (dataset: string) => {
    try {
      setExportingDataset(dataset);
      setError('');

      const path = isSuperAdmin
        ? '/api/v1/admin/reports/taxonomy/export'
        : '/api/v1/reports/taxonomy/export';
      const qs = buildQS(filters, { dataset, grain: filters.grain || 'day' });
      const url = `${path}${qs}`;

      const accessToken = localStorage.getItem('accessToken');
      const currentOrgId = localStorage.getItem('currentOrgId');

      const headers: HeadersInit = {};
      if (accessToken) {
        headers['Authorization'] = `Bearer ${accessToken}`;
      }
      if (!isSuperAdmin && currentOrgId) {
        headers['X-Org-Context'] = currentOrgId;
      }

      const response = await fetch(url, {
        method: 'GET',
        headers,
        credentials: 'same-origin',
      });

      if (!response.ok) {
        const text = await response.text().catch(() => '');
        throw new Error(text || `Export failed with status ${response.status}`);
      }

      const blob = await response.blob();
      const objectUrl = window.URL.createObjectURL(blob);
      const a = document.createElement('a');

      const contentDisposition = response.headers.get('Content-Disposition') || '';
      const match = contentDisposition.match(/filename="?([^";]+)"?/i);
      const fallbackName = `impact-report-${dataset}-${new Date().toISOString().slice(0, 10)}.csv`;
      a.href = objectUrl;
      a.download = match?.[1] || fallbackName;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(objectUrl);
    } catch (err: any) {
      setError(err?.message || 'Failed to export CSV');
    } finally {
      setExportingDataset(null);
    }
  };

  const runMultiExport = async () => {
    const selected = DATASETS.filter((d) => selectedDatasets[d]);
    if (selected.length === 0) return;
    if (exportFormat === 'csv') {
      for (const ds of selected) {
        // eslint-disable-next-line no-await-in-loop
        await runExport(ds);
      }
      setShowExportDialog(false);
      return;
    }

    try {
      setError('');
      setExportingDataset('xlsx-package');

      const path = isSuperAdmin
        ? '/api/v1/admin/reports/taxonomy/export/xlsx'
        : '/api/v1/reports/taxonomy/export/xlsx';
      const qs = buildQS(filters, {
        grain: filters.grain || 'day',
        datasets: selected.join(','),
      });
      const url = `${path}${qs}`;

      const accessToken = localStorage.getItem('accessToken');
      const currentOrgId = localStorage.getItem('currentOrgId');
      const headers: HeadersInit = {};
      if (accessToken) {
        headers['Authorization'] = `Bearer ${accessToken}`;
      }
      if (!isSuperAdmin && currentOrgId) {
        headers['X-Org-Context'] = currentOrgId;
      }

      const response = await fetch(url, {
        method: 'GET',
        headers,
        credentials: 'same-origin',
      });
      if (!response.ok) {
        const text = await response.text().catch(() => '');
        throw new Error(text || `XLSX export failed with status ${response.status}`);
      }

      const blob = await response.blob();
      const objectUrl = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      const contentDisposition = response.headers.get('Content-Disposition') || '';
      const match = contentDisposition.match(/filename="?([^";]+)"?/i);
      const fallbackName = `impact-report-export-${new Date().toISOString().slice(0, 10)}.xlsx`;
      a.href = objectUrl;
      a.download = match?.[1] || fallbackName;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(objectUrl);
      setShowExportDialog(false);
    } catch (err: any) {
      setError(err?.message || 'Failed to export XLSX package');
    } finally {
      setExportingDataset(null);
    }
  };

  const toggleExpandedRow = (commentID: number) => {
    setExpandedRows((prev) => ({ ...prev, [commentID]: !prev[commentID] }));
  };

  const findingsColumns = useMemo<ColumnDef<FindingRow>[]>(() => [
    {
      id: 'details',
      header: 'Details',
      size: 70,
      minSize: 60,
      maxSize: 90,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <button
            onClick={() => toggleExpandedRow(r.comment_id)}
            className="text-slate-400 hover:text-white"
            title={expandedRows[r.comment_id] ? 'Collapse details' : 'Expand details'}
          >
            {expandedRows[r.comment_id] ? '−' : '+'}
          </button>
        );
      },
    },
    {
      accessorKey: 'severity',
      header: 'Severity',
      size: 110,
      cell: ({ row }) => (
        <span className={`inline-block rounded px-1.5 py-0.5 text-xs font-semibold uppercase ${severityBadge(row.original.severity)}`}>
          {row.original.severity || '—'}
        </span>
      ),
    },
    {
      accessorKey: 'confidence',
      header: 'Confidence',
      size: 110,
      cell: ({ row }) => (
        <span className={`inline-block rounded px-1.5 py-0.5 text-xs ${confidenceBadge(row.original.confidence)}`}>
          {row.original.confidence || '—'}
        </span>
      ),
    },
    {
      accessorKey: 'type',
      header: 'Type',
      size: 130,
      cell: ({ row }) => <span className="text-slate-300 whitespace-nowrap">{row.original.type || '—'}</span>,
    },
    {
      accessorKey: 'category',
      header: 'Category',
      size: 160,
      cell: ({ row }) => <span className="text-slate-200 whitespace-nowrap">{row.original.category || '—'}</span>,
    },
    {
      accessorKey: 'subcategory',
      header: 'Subcategory',
      size: 180,
      cell: ({ row }) => <span className="text-slate-400">{row.original.subcategory || '—'}</span>,
    },
    {
      accessorKey: 'repository',
      header: 'Repository',
      size: 220,
      minSize: 170,
      cell: ({ row }) => <span className="text-slate-300 font-mono whitespace-normal break-all" title={row.original.repository}>{row.original.repository || '—'}</span>,
    },
    {
      id: 'file',
      header: 'File',
      size: 360,
      minSize: 220,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <span className="text-slate-400 font-mono whitespace-normal break-all" title={r.file_path ?? ''}>
            {r.file_path ? `${r.file_path}${r.line_number ? `:${r.line_number}` : ''}` : '—'}
          </span>
        );
      },
    },
    {
      id: 'issue',
      header: 'Issue',
      size: 400,
      minSize: 240,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <span className="text-slate-200 whitespace-normal" title={r.content}>
            {r.content.length > 140 ? r.content.slice(0, 140) + '…' : r.content}
          </span>
        );
      },
    },
    {
      accessorKey: 'created_at',
      header: 'Created',
      size: 170,
      minSize: 140,
      cell: ({ row }) => <span className="text-slate-400 whitespace-nowrap">{formatDate(row.original.created_at)}</span>,
    },
  ], [expandedRows]);

  const sortByForHeader: Record<string, FindingsSortBy | null> = {
    details: null,
    severity: 'severity',
    confidence: 'confidence',
    type: 'type',
    category: 'category',
    subcategory: 'subcategory',
    repository: 'repository',
    file: 'file_path',
    issue: null,
    created_at: 'created_at',
  };

  const sortIndicator = (sortBy: FindingsSortBy | null) => {
    if (!sortBy) return '';
    if (findingsSortBy !== sortBy) return '↕';
    return findingsSortDir === 'asc' ? '↑' : '↓';
  };

  const toggleFindingsSort = (sortBy: FindingsSortBy | null) => {
    if (!sortBy) return;
    const nextDir: 'asc' | 'desc' = findingsSortBy === sortBy && findingsSortDir === 'asc' ? 'desc' : 'asc';
    setFindingsSortBy(sortBy);
    setFindingsSortDir(nextDir);
    load(filters, 0, { sortBy, sortDir: nextDir, columnFilters: findingsColumnFilters });
  };

  const findingsTable = useReactTable({
    data: findings,
    columns: findingsColumns,
    getCoreRowModel: getCoreRowModel(),
    columnResizeMode: 'onChange',
  });

  const findingsPages = Math.ceil(findingsTotal / findingsLimit);
  const currentPage = Math.floor(findingsOffset / findingsLimit) + 1;
  const filledTrend = useMemo(
    () => buildFullRangeTrend(trend, filters.since, filters.until, filters.grain || 'day'),
    [trend, filters.since, filters.until, filters.grain],
  );
  const topBreakdown = breakdown.slice(0, 5);
  const riskScore = useMemo(() => {
    if (!summary || summary.total_findings === 0) return 0;
    const weighted = summary.critical_count * 4 + summary.high_count * 2 + summary.medium_count;
    return Math.round((weighted / summary.total_findings) * 100);
  }, [summary]);

  // ---- render helpers ------------------------------------------------------

  const StatCard = ({ label, value, sub }: { label: string; value: string | number; sub?: string }) => (
    <div className="bg-slate-800/70 border border-slate-700 rounded p-3">
      <p className="text-slate-400 text-xs">{label}</p>
      <p className="text-white font-semibold text-lg">{typeof value === 'number' ? value.toLocaleString() : value}</p>
      {sub && <p className="text-slate-400 text-xs mt-0.5">{sub}</p>}
    </div>
  );

  const DistTable = ({
    title,
    rows,
    dimLabel = 'Value',
    onRowClick,
  }: {
    title: string;
    rows: DistRow[];
    dimLabel?: string;
    onRowClick?: (row: DistRow) => void;
  }) => {
    const maxCount = Math.max(1, ...rows.map((r) => r.count));
    return (
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
        <h3 className="text-white font-semibold mb-3 text-sm">{title}</h3>
        {rows.length === 0 ? (
          <p className="text-slate-400 text-xs">No data for this dimension yet.</p>
        ) : (
          <div className="space-y-1.5">
            {rows.map((r, i) => {
              const barPct = Math.max(2, Math.round((r.count / maxCount) * 100));
              return (
                <button
                  key={i}
                  className={`w-full text-left rounded px-2 py-1.5 hover:bg-slate-700/40 transition-colors ${onRowClick ? 'cursor-pointer' : 'cursor-default'}`}
                  onClick={() => onRowClick?.(r)}
                >
                  <div className="flex justify-between text-xs mb-1">
                    <span className="text-slate-200 truncate max-w-[60%]">{r.value || '(empty)'}</span>
                    <span className="text-slate-400 ml-2 shrink-0">{r.count.toLocaleString()}</span>
                  </div>
                  <div className="h-1.5 bg-slate-700 rounded overflow-hidden">
                    <div className="h-1.5 bg-blue-500 rounded" style={{ width: `${barPct}%` }} />
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    );
  };

  const TrendAreaChart = ({
    rows,
    height = 120,
    showBrush = true,
    showLegend = true,
  }: {
    rows: FilledTrendRow[];
    height?: number;
    showBrush?: boolean;
    showLegend?: boolean;
  }) => {
    if (rows.length === 0) {
      return <p className="text-slate-400 text-xs">No trend data in the selected range.</p>;
    }
    const tooltipStyle = {
      background: '#0f172a',
      border: '1px solid #334155',
      borderRadius: 8,
      color: '#e2e8f0',
      fontSize: 12,
    } as const;

    return (
      <div className="space-y-2">
        <div className="w-full rounded bg-slate-900/50 border border-slate-700 p-2" style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={rows} margin={{ top: 8, right: 12, left: 0, bottom: showBrush ? 26 : 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis dataKey="bucket" tick={{ fill: '#94a3b8', fontSize: 10 }} axisLine={{ stroke: '#475569' }} tickLine={{ stroke: '#475569' }} />
              <YAxis tick={{ fill: '#94a3b8', fontSize: 10 }} axisLine={{ stroke: '#475569' }} tickLine={{ stroke: '#475569' }} />
              <Tooltip contentStyle={tooltipStyle} labelStyle={{ color: '#cbd5e1', fontSize: 11 }} />
              {showLegend && <Legend wrapperStyle={{ color: '#cbd5e1', fontSize: 11 }} />}
              <Area name="Findings" type="monotone" dataKey="count" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.2} strokeWidth={2} />
              <Line name="Reviews" type="monotone" dataKey="review_count" stroke="#22c55e" strokeWidth={2} dot={false} />
              {showBrush && (
                <Brush dataKey="bucket" height={14} stroke="#334155" fill="#0f172a" travellerWidth={6} />
              )}
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    );
  };

  // ---- main render ---------------------------------------------------------

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-white">Impact Report</h1>
          <p className="text-slate-400 text-sm mt-0.5">
            Explore review findings by severity, confidence, type, category, and subcategory.
            {isSuperAdmin && <span className="ml-2 text-xs bg-purple-800 text-purple-200 px-1.5 py-0.5 rounded">Super-admin: global view</span>}
          </p>
          <p className="text-slate-500 text-xs mt-1">
            Default view: last 30 days. Use Reset to return to defaults.
          </p>
        </div>
        <div className="flex gap-2 flex-wrap">
          <button
            onClick={() => setShowExportDialog(true)}
            disabled={loading}
            className="px-3 py-1.5 rounded bg-blue-700 hover:bg-blue-600 text-white text-xs border border-blue-500"
          >
            Export Data
          </button>
        </div>
      </div>

      <div className="flex gap-2">
        <button
          onClick={() => setMode('overview')}
          className={`px-3 py-1.5 rounded text-xs border ${mode === 'overview' ? 'bg-slate-700 border-slate-500 text-white' : 'bg-slate-900 border-slate-700 text-slate-300'}`}
        >Executive Overview</button>
        <button
          onClick={() => setMode('explore')}
          className={`px-3 py-1.5 rounded text-xs border ${mode === 'explore' ? 'bg-slate-700 border-slate-500 text-white' : 'bg-slate-900 border-slate-700 text-slate-300'}`}
        >Exploration</button>
      </div>

      {/* Filters */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-3">
        <p className="text-slate-400 text-xs mb-3">
          All filter fields are optional. If left blank, that dimension matches all values.
        </p>
        <div className="flex items-center gap-1.5 flex-wrap mb-2">
          <span className="text-slate-500 text-xs">Presets:</span>
          <button className="px-2 py-1 rounded bg-slate-700 hover:bg-slate-600 text-[11px] text-white" onClick={() => applyDatePreset(7)}>Last 7d</button>
          <button className="px-2 py-1 rounded bg-slate-700 hover:bg-slate-600 text-[11px] text-white" onClick={() => applyDatePreset(30)}>Last 30d</button>
          <button className="px-2 py-1 rounded bg-slate-700 hover:bg-slate-600 text-[11px] text-white" onClick={() => applyDatePreset(90)}>Last 3m</button>
          <button className="px-2 py-1 rounded bg-slate-700 hover:bg-slate-600 text-[11px] text-white" onClick={() => applyDatePreset(180)}>Last 6m</button>
          <button className="px-2 py-1 rounded bg-slate-700 hover:bg-slate-600 text-[11px] text-white" onClick={() => applyDatePreset('all')}>All</button>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 gap-2">
          <div className="flex flex-col gap-1">
            <label className="text-slate-400 text-[11px]">Since</label>
            <input type="date" value={filters.since} onChange={setF('since')}
              className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-white text-xs" />
          </div>
          <div className="flex flex-col gap-1">
            <label className="text-slate-400 text-[11px]">Until</label>
            <input type="date" value={filters.until} onChange={setF('until')}
              className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-white text-xs" />
          </div>
          <MultiSelectField
            label="Severity"
            options={severityOptions}
            value={filters.severity}
            onChange={(next) => setFilters((prev) => ({ ...prev, severity: next }))}
          />
          <MultiSelectField
            label="Confidence"
            options={confidenceOptions}
            value={filters.confidence}
            onChange={(next) => setFilters((prev) => ({ ...prev, confidence: next }))}
          />
          <MultiSelectField
            label="Type"
            options={typeOptions}
            value={filters.issueType}
            onChange={(next) => setFilters((prev) => ({ ...prev, issueType: next }))}
          />
          <MultiSelectField
            label="Category"
            options={categoryMultiOptions}
            value={filters.category}
            onChange={(next) => {
              const picked = parseMulti(next);
              const allowedSubs = new Set<string>();
              picked.forEach((c) => (relationMap.get(c) || []).forEach((s) => allowedSubs.add(s)));
              const nextSub = picked.length === 0
                ? ''
                : parseMulti(filters.subcategory).filter((s) => allowedSubs.has(s)).join(',');
              setFilters((prev) => ({ ...prev, category: next, subcategory: nextSub }));
            }}
          />
          <MultiSelectField
            label="Subcategory"
            options={subcategoryMultiOptions}
            value={filters.subcategory}
            onChange={(next) => setFilters((prev) => ({ ...prev, subcategory: next }))}
          />
          <div className="flex flex-col gap-1">
            <label className="text-slate-400 text-[11px]">Repository (optional)</label>
            <input value={filters.repository} onChange={setF('repository')} placeholder="e.g. owner/repo"
              className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-white text-xs" />
          </div>
          <MultiSelectField
            label="Provider"
            options={providerOptions}
            value={filters.provider}
            onChange={(next) => setFilters((prev) => ({ ...prev, provider: next }))}
          />
          <div className="flex flex-col gap-1">
            <label className="text-slate-400 text-[11px]">Trend grain</label>
            <select value={filters.grain} onChange={setF('grain')}
              className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-white text-xs">
              {['day', 'week', 'month'].map(g => <option key={g} value={g}>{g}</option>)}
            </select>
          </div>
          {isSuperAdmin && (
            <div className="flex flex-col gap-1">
              <label className="text-slate-400 text-[11px]">Org ID (optional)</label>
              <input value={filters.orgId} onChange={setF('orgId')} placeholder="0 = all"
                className="bg-slate-900 border border-slate-600 rounded px-2 py-1 text-white text-xs" />
            </div>
          )}
          <div className="flex items-end justify-end gap-2 md:col-span-2 xl:col-span-2">
            <button onClick={handleReset} disabled={loading}
              className="px-3 py-1.5 rounded bg-slate-700 hover:bg-slate-600 text-white text-xs min-w-24">
              Reset
            </button>
            <button onClick={handleApply} disabled={loading}
              className="px-4 py-1.5 rounded bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium min-w-28">
              {loading ? 'Loading…' : 'Apply'}
            </button>
          </div>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/30 border border-red-600/40 rounded-lg p-3 text-red-200 text-sm">{error}</div>
      )}

      {!loading && !error && summary && summary.total_findings === 0 && (
        <div className="bg-amber-900/20 border border-amber-700/40 rounded-lg p-3 text-amber-100 text-sm">
          No findings matched the current filters. This is usually either a clean result set for the selected period or filters that are too narrow.
          Try widening the date range first.
        </div>
      )}

      {/* KPI Summary */}
      {summary && (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-10 gap-3">
          <StatCard label="Total Findings" value={summary.total_findings} />
          <StatCard label="Reviews Performed" value={summary.total_reviews} />
          <StatCard label="Critical" value={summary.critical_count} />
          <StatCard label="High / Error" value={summary.high_count} />
          <StatCard label="Medium / Warning" value={summary.medium_count} />
          <StatCard label="Low" value={summary.low_count} />
          <StatCard label="Info" value={summary.info_count} />
          <StatCard label="High Confidence" value={summary.high_confidence_count} />
          <StatCard label="Med Confidence" value={summary.medium_confidence_count} />
          <StatCard label="Low Confidence" value={summary.low_confidence_count} />
        </div>
      )}

      {mode === 'overview' && summary && (
        <>
          {/* Executive summary strip */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            {/* Critical + Warnings alert cards side-by-side */}
            <div className="grid grid-cols-2 gap-3">
              <div className={`rounded-lg p-4 border ${summary.critical_count > 0 ? 'bg-red-900/30 border-red-700' : 'bg-slate-800/70 border-slate-700'}`}>
                <p className="text-red-300 text-xs uppercase tracking-widest mb-1">Critical</p>
                <p className={`text-3xl font-bold ${summary.critical_count > 0 ? 'text-red-300' : 'text-slate-500'}`}>{summary.critical_count.toLocaleString()}</p>
                {summary.total_findings > 0 && (
                  <p className="text-red-400/70 text-[11px] mt-1">{Math.round((summary.critical_count / summary.total_findings) * 100)}% of total</p>
                )}
              </div>
              <div className={`rounded-lg p-4 border ${summary.medium_count + summary.high_count > 0 ? 'bg-yellow-900/20 border-yellow-700/60' : 'bg-slate-800/70 border-slate-700'}`}>
                <p className="text-yellow-300 text-xs uppercase tracking-widest mb-1">Warnings</p>
                <p className={`text-3xl font-bold ${summary.medium_count + summary.high_count > 0 ? 'text-yellow-300' : 'text-slate-500'}`}>{(summary.medium_count + summary.high_count).toLocaleString()}</p>
                {summary.total_findings > 0 && (
                  <p className="text-yellow-400/70 text-[11px] mt-1">{Math.round(((summary.medium_count + summary.high_count) / summary.total_findings) * 100)}% of total</p>
                )}
              </div>
            </div>

            {/* Top Sources */}
            <div className="bg-slate-800/70 border border-slate-700 rounded-lg p-4">
              <p className="text-slate-400 text-xs uppercase tracking-widest mb-2">Top Sources</p>
              {topBreakdown.length === 0 ? (
                <p className="text-slate-400 text-xs mt-2">No repository data in selected range.</p>
              ) : (
                <div className="space-y-2 mt-1">
                  {topBreakdown.map((r, i) => {
                    const maxBreakdown = Math.max(1, topBreakdown[0]?.count || 1);
                    const w = Math.max(4, Math.round((r.count / maxBreakdown) * 100));
                    return (
                      <div key={i}>
                        <div className="flex justify-between text-xs mb-0.5">
                          <span className="text-slate-200 truncate max-w-[70%]">{r.repository || 'Unknown repo'}</span>
                          <span className="text-slate-400 ml-1">{r.count}</span>
                        </div>
                        <div className="h-1.5 bg-slate-700 rounded">
                          <div className="h-1.5 bg-amber-500 rounded" style={{ width: `${w}%` }} />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            {/* Quality momentum */}
            <div className="bg-slate-800/70 border border-slate-700 rounded-lg p-4">
              <p className="text-slate-400 text-xs uppercase tracking-widest mb-2">Finding Volume</p>
              <TrendAreaChart rows={filledTrend} height={170} showBrush={false} showLegend={false} />
            </div>
          </div>

          {/* Category breakdown bars (click to explore) */}
          <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-white font-semibold text-sm">Findings by Category</h3>
              <button
                className="text-blue-400 hover:text-blue-300 text-xs"
                onClick={() => setMode('explore')}
              >Explore all →</button>
            </div>
            {categoryDist.length === 0 ? (
              <p className="text-slate-400 text-xs">No category data in selected range.</p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-2">
                {categoryDist.slice(0, 10).map((row) => {
                  const maxCat = Math.max(1, categoryDist[0]?.count || 1);
                  const width = Math.max(2, Math.round((row.count / maxCat) * 100));
                  return (
                    <button
                      key={row.value}
                      onClick={() => {
                        setFilters((prev) => ({ ...prev, category: row.value, subcategory: '' }));
                        setMode('explore');
                      }}
                      className="w-full text-left group"
                    >
                      <div className="flex justify-between text-xs mb-1">
                        <span className="text-slate-200 group-hover:text-white truncate">{row.value || '(empty)'}</span>
                        <span className="text-slate-400 ml-2">{row.count.toLocaleString()}</span>
                      </div>
                      <div className="h-2 bg-slate-700 rounded overflow-hidden">
                        <div className="h-2 bg-emerald-600 group-hover:bg-emerald-400 transition-colors" style={{ width: `${width}%` }} />
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          {/* Severity composition - horizontal stacked bars */}
          {summary.total_findings > 0 && (
            <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
              <h3 className="text-white font-semibold mb-3 text-sm">Severity Composition</h3>
              <div className="flex h-4 rounded overflow-hidden w-full">
                {summary.critical_count > 0 && (
                  <div
                    className="bg-red-600"
                    style={{ width: `${(summary.critical_count / summary.total_findings) * 100}%` }}
                    title={`Critical: ${summary.critical_count}`}
                  />
                )}
                {summary.high_count > 0 && (
                  <div
                    className="bg-orange-500"
                    style={{ width: `${(summary.high_count / summary.total_findings) * 100}%` }}
                    title={`High: ${summary.high_count}`}
                  />
                )}
                {summary.medium_count > 0 && (
                  <div
                    className="bg-yellow-500"
                    style={{ width: `${(summary.medium_count / summary.total_findings) * 100}%` }}
                    title={`Medium: ${summary.medium_count}`}
                  />
                )}
                {summary.low_count > 0 && (
                  <div
                    className="bg-blue-500"
                    style={{ width: `${(summary.low_count / summary.total_findings) * 100}%` }}
                    title={`Low: ${summary.low_count}`}
                  />
                )}
                {summary.info_count > 0 && (
                  <div
                    className="bg-slate-500"
                    style={{ width: `${(summary.info_count / summary.total_findings) * 100}%` }}
                    title={`Info: ${summary.info_count}`}
                  />
                )}
              </div>
              <div className="flex gap-4 mt-2 flex-wrap">
                {[
                  { label: 'Critical', count: summary.critical_count, color: 'bg-red-600' },
                  { label: 'High', count: summary.high_count, color: 'bg-orange-500' },
                  { label: 'Medium', count: summary.medium_count, color: 'bg-yellow-500' },
                  { label: 'Low', count: summary.low_count, color: 'bg-blue-500' },
                  { label: 'Info', count: summary.info_count, color: 'bg-slate-500' },
                ].filter((x) => x.count > 0).map(({ label, count, color }) => (
                  <div key={label} className="flex items-center gap-1 text-xs">
                    <div className={`w-2 h-2 rounded-sm ${color}`} />
                    <span className="text-slate-300">{label}</span>
                    <span className="text-slate-400">{count.toLocaleString()}</span>
                    <span className="text-slate-600">({Math.round((count / summary.total_findings) * 100)}%)</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}

      {mode === 'explore' && (
        <>
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
            {/* Severity distribution */}
            <DistTable
              title="By Severity"
              rows={severityDist}
              dimLabel="Severity"
              onRowClick={(row) => {
                const curr = new Set(parseMulti(filters.severity));
                if (curr.has(row.value)) curr.delete(row.value);
                else curr.add(row.value);
                setFilters((prev) => ({ ...prev, severity: Array.from(curr).join(',') }));
              }}
            />

            {/* Category -> Subcategory accordion */}
            <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
              <h3 className="text-white font-semibold mb-3 text-sm">By Category / Subcategory</h3>
              <p className="text-slate-500 text-xs mb-3">Click category rows to expand and toggle filter membership.</p>
              {categoryDist.length === 0 ? (
                <p className="text-slate-400 text-xs">No category data in selected range.</p>
              ) : (
                <div className="space-y-0.5">
                  {categoryDist.map((catRow) => {
                    const subs = relationMap.get(catRow.value) || [];
                    const subDist = subcategoryDist.filter((s) => subs.includes(s.value));
                    const isExpanded = !!expandedCategories[catRow.value];
                    const catMax = Math.max(1, categoryDist[0]?.count || 1);
                    const barW = Math.max(2, Math.round((catRow.count / catMax) * 100));
                    const currentCats = new Set(parseMulti(filters.category));
                    const isActive = currentCats.has(catRow.value);
                    return (
                      <div key={catRow.value}>
                        <button
                          className={`w-full flex items-center gap-2 rounded px-2 py-1.5 text-left group ${isActive ? 'bg-blue-900/30' : 'hover:bg-slate-700/40'}`}
                          title="Click to expand/collapse and toggle category filter"
                          onClick={() => {
                            toggleCategory(catRow.value);
                            const next = new Set(parseMulti(filters.category));
                            if (next.has(catRow.value)) next.delete(catRow.value);
                            else next.add(catRow.value);
                            const nextCats = Array.from(next);
                            const allowedSubs = new Set<string>();
                            nextCats.forEach((c) => (relationMap.get(c) || []).forEach((s) => allowedSubs.add(s)));
                            const nextSub = parseMulti(filters.subcategory).filter((s) => allowedSubs.has(s)).join(',');
                            setFilters((prev) => ({ ...prev, category: nextCats.join(','), subcategory: nextSub }));
                          }}
                        >
                          <span className="text-slate-500 group-hover:text-slate-200 w-4 shrink-0 text-xs">
                            {subs.length > 0 ? (isExpanded ? '▾' : '▸') : '·'}
                          </span>
                          <span className="flex-1 min-w-0">
                            <span className="flex justify-between text-xs mb-0.5">
                              <span className={`truncate ${isActive ? 'text-blue-300 font-medium' : 'text-slate-200'}`}>{catRow.value || '(empty)'}</span>
                              <span className="text-slate-400 ml-2 shrink-0">{catRow.count.toLocaleString()}</span>
                            </span>
                            <span className="h-1 bg-slate-700 rounded overflow-hidden block">
                              <span className="h-1 bg-emerald-600 rounded block" style={{ width: `${barW}%` }} />
                            </span>
                          </span>
                        </button>
                        {isExpanded && (
                          <div className="ml-6 mt-0.5 mb-1 space-y-0.5 border-l border-slate-700 pl-3">
                            {(subDist.length > 0 ? subDist : subs.map((s) => ({ dimension: 'subcategory', value: s, count: 0 }))).map((sub) => {
                              const subMax = Math.max(1, subDist[0]?.count || 1);
                              const subBar = subDist.length > 0 ? Math.max(2, Math.round((sub.count / subMax) * 100)) : 0;
                              const subSet = new Set(parseMulti(filters.subcategory));
                              const isSubActive = subSet.has(sub.value);
                              return (
                                <button
                                  key={sub.value}
                                  onClick={() => {
                                    const nextSub = new Set(parseMulti(filters.subcategory));
                                    if (nextSub.has(sub.value)) nextSub.delete(sub.value);
                                    else nextSub.add(sub.value);
                                    const nextCats = new Set(parseMulti(filters.category));
                                    nextCats.add(catRow.value);
                                    setFilters((prev) => ({ ...prev, category: Array.from(nextCats).join(','), subcategory: Array.from(nextSub).join(',') }));
                                  }}
                                  className={`w-full text-left rounded px-2 py-1 text-xs ${isSubActive ? 'bg-blue-900/30 text-blue-300' : 'hover:bg-slate-700/30 text-slate-300'}`}
                                >
                                  <div className="flex justify-between mb-0.5">
                                    <span className="truncate">{sub.value}</span>
                                    {sub.count > 0 && <span className="text-slate-400 ml-2">{sub.count.toLocaleString()}</span>}
                                  </div>
                                  {subBar > 0 && (
                                    <div className="h-0.5 bg-slate-700 rounded overflow-hidden">
                                      <div className="h-0.5 bg-blue-500 rounded" style={{ width: `${subBar}%` }} />
                                    </div>
                                  )}
                                </button>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

      {/* Findings explorer */}
      <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-white font-semibold text-sm">
            Findings Explorer
            <span className="ml-2 text-slate-400 text-xs font-normal">
              {findingsTotal.toLocaleString()} total · page {currentPage} of {findingsPages || 1}
            </span>
          </h3>
          <div className="flex gap-2">
            <button
              onClick={applyFindingsQuery}
              disabled={loading}
              className="px-3 py-1 rounded bg-blue-700 hover:bg-blue-600 text-white text-xs disabled:opacity-40"
            >Apply Table Filters</button>
            <button
              onClick={resetFindingsQuery}
              disabled={loading}
              className="px-3 py-1 rounded bg-slate-700 hover:bg-slate-600 text-white text-xs disabled:opacity-40"
            >Clear Table Filters</button>
            <button
              disabled={findingsOffset === 0 || loading}
              onClick={() => load(filters, Math.max(0, findingsOffset - findingsLimit))}
              className="px-3 py-1 rounded bg-slate-700 hover:bg-slate-600 text-white text-xs disabled:opacity-40"
            >← Prev</button>
            <button
              disabled={findingsOffset + findingsLimit >= findingsTotal || loading}
              onClick={() => load(filters, findingsOffset + findingsLimit)}
              className="px-3 py-1 rounded bg-slate-700 hover:bg-slate-600 text-white text-xs disabled:opacity-40"
            >Next →</button>
          </div>
        </div>
        <p className="text-slate-500 text-[11px] mb-3">Sort on header click, then Apply Table Filters to query the full dataset. Drag header separators to resize columns.</p>
        {findings.length === 0 ? (
          <p className="text-slate-400 text-xs">No findings match the current filters. Try removing some filters or broadening the date range.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-xs text-left">
              <thead className="bg-slate-900/80 text-slate-300">
                {findingsTable.getHeaderGroups().map((headerGroup) => (
                  <tr key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <th
                        key={header.id}
                        className="px-2 py-2 relative"
                        style={{ width: header.getSize() }}
                      >
                        {header.isPlaceholder ? null : (
                          <button
                            type="button"
                            onClick={() => toggleFindingsSort(sortByForHeader[header.id] ?? null)}
                            className="w-full text-left text-slate-300 hover:text-white"
                          >
                            <span className="inline-flex items-center gap-1">
                              <span>{flexRender(header.column.columnDef.header, header.getContext())}</span>
                              <span className="text-[10px] text-slate-500">{sortIndicator(sortByForHeader[header.id] ?? null)}</span>
                            </span>
                          </button>
                        )}
                        {header.column.getCanResize() && (
                          <div
                            onMouseDown={header.getResizeHandler()}
                            onTouchStart={header.getResizeHandler()}
                            className={`absolute right-0 top-0 h-full w-1 cursor-col-resize select-none touch-none ${header.column.getIsResizing() ? 'bg-blue-500' : 'bg-slate-600/60 hover:bg-blue-500'}`}
                          />
                        )}
                      </th>
                    ))}
                  </tr>
                ))}
                <tr className="bg-slate-900/40">
                  <th className="px-2 py-1" />
                  <th className="px-2 py-1"><input value={findingsColumnFilters.severity} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, severity: e.target.value }))} placeholder="severity" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.confidence} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, confidence: e.target.value }))} placeholder="confidence" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.type} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, type: e.target.value }))} placeholder="type" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.category} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, category: e.target.value }))} placeholder="category" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.subcategory} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, subcategory: e.target.value }))} placeholder="subcategory" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.repository} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, repository: e.target.value }))} placeholder="repository" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.file_path} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, file_path: e.target.value }))} placeholder="file path" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.content} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, content: e.target.value }))} placeholder="issue text" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                  <th className="px-2 py-1"><input value={findingsColumnFilters.created_at} onChange={(e) => setFindingsColumnFilters((prev) => ({ ...prev, created_at: e.target.value }))} placeholder="created at" className="w-full bg-slate-800 border border-slate-700 rounded px-1.5 py-1 text-[11px] text-slate-100" /></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700">
                {findingsTable.getRowModel().rows.map((row) => {
                  const record = row.original;
                  return (
                    <React.Fragment key={row.id}>
                      <tr className="hover:bg-slate-900/30 align-top">
                        {row.getVisibleCells().map((cell) => (
                          <td key={cell.id} className="px-2 py-2" style={{ width: cell.column.getSize() }}>
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </td>
                        ))}
                      </tr>
                      {expandedRows[record.comment_id] && (
                        <tr className="bg-slate-900/40">
                          <td className="px-2 py-2" />
                          <td className="px-2 py-2 text-slate-300" colSpan={findingsColumns.length - 1}>
                            <div className="space-y-2 text-xs">
                              <div>
                                <span className="text-slate-500">Full file path:</span>
                                <span className="ml-2 text-slate-200 font-mono break-all">{record.file_path || '—'}</span>
                              </div>
                              <div>
                                <span className="text-slate-500">Full issue text:</span>
                                <p className="mt-1 text-slate-200 whitespace-pre-wrap leading-relaxed">{record.content || '—'}</p>
                              </div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </React.Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
            {/* Trend chart */}
            <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
              <h3 className="text-white font-semibold mb-4 text-sm">Finding Volume by {filters.grain || 'day'}</h3>
              <TrendAreaChart rows={filledTrend} height={250} showBrush={true} showLegend={true} />
            </div>

            {/* Breakdown */}
            <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
              <h3 className="text-white font-semibold mb-3 text-sm">Breakdown by {isSuperAdmin ? 'Org / ' : ''}Repository / Provider</h3>
              {breakdown.length === 0 ? (
                <p className="text-slate-400 text-xs">No breakdown data available.</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="min-w-full text-xs text-left">
                    <thead className="bg-slate-900/80 text-slate-300">
                      <tr>
                        {isSuperAdmin && <><th className="px-3 py-2">Org ID</th><th className="px-3 py-2">Org</th></>}
                        <th className="px-3 py-2">Repository</th>
                        <th className="px-3 py-2">Provider</th>
                        <th className="px-3 py-2 text-right">Findings</th>
                        <th className="px-3 py-2 text-right">Reviews</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-700">
                      {breakdown.map((r, i) => (
                        <tr key={i} className="hover:bg-slate-900/30">
                          {isSuperAdmin && <><td className="px-3 py-1.5 text-slate-400">{r.org_id ?? '—'}</td><td className="px-3 py-1.5 text-slate-200">{r.org_name ?? '—'}</td></>}
                          <td className="px-3 py-1.5 text-slate-100">{r.repository || '—'}</td>
                          <td className="px-3 py-1.5 text-slate-300">{r.provider || '—'}</td>
                          <td className="px-3 py-1.5 text-white text-right">{r.count.toLocaleString()}</td>
                          <td className="px-3 py-1.5 text-emerald-300 text-right">{(r.review_count || 0).toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
      </>
      )}

      {showExportDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="w-full max-w-3xl bg-slate-900 border border-slate-700 rounded-lg p-5 space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-white text-lg font-semibold">Export Data</h3>
              <button className="text-slate-400 hover:text-white text-sm" onClick={() => setShowExportDialog(false)}>Close</button>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
              <div className="space-y-2 lg:col-span-1">
                <p className="text-slate-400 text-xs font-medium">Datasets / Sheets</p>
                {DATASETS.map((ds) => {
                  const active = previewDataset === ds;
                  return (
                    <div key={ds} className={`flex items-center gap-2 border rounded px-2 py-1.5 ${active ? 'border-blue-500 bg-blue-900/20' : 'border-slate-700 bg-slate-800'}`}>
                      <input
                        type="checkbox"
                        checked={selectedDatasets[ds]}
                        onChange={(e) => setSelectedDatasets((prev) => ({ ...prev, [ds]: e.target.checked }))}
                        className="accent-blue-500"
                      />
                      <button
                        type="button"
                        onClick={() => setPreviewDataset(ds)}
                        className="text-left flex-1"
                      >
                        <p className="text-sm text-slate-100">{ds.replace(/_/g, ' ')}</p>
                        <p className="text-[11px] text-slate-500">{exportPreview?.[ds]?.toLocaleString() ?? '…'} rows</p>
                      </button>
                    </div>
                  );
                })}
              </div>

              <div className="lg:col-span-2">
                <p className="text-slate-400 text-xs font-medium mb-2">Preview: {previewDataset.replace(/_/g, ' ')}</p>
                {previewDataset === 'findings' && (
                  findings.length === 0 ? (
                    <p className="text-slate-500 text-xs">No findings in current filter range.</p>
                  ) : (
                    <div className="overflow-x-auto rounded border border-slate-700">
                      <table className="min-w-full text-xs text-left">
                        <thead className="bg-slate-800 text-slate-400">
                          <tr>
                            <th className="px-2 py-1.5 font-medium">Severity</th>
                            <th className="px-2 py-1.5 font-medium">Category</th>
                            <th className="px-2 py-1.5 font-medium">Subcategory</th>
                            <th className="px-2 py-1.5 font-medium">File</th>
                            <th className="px-2 py-1.5 font-medium">Issue</th>
                            <th className="px-2 py-1.5 font-medium">Repository</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-700/60">
                          {findings.slice(0, 8).map((r) => (
                            <tr key={r.comment_id} className="bg-slate-900/60">
                              <td className="px-2 py-1.5"><span className={`inline-block rounded px-1.5 py-0.5 text-xs font-semibold uppercase ${severityBadge(r.severity)}`}>{r.severity || '—'}</span></td>
                              <td className="px-2 py-1.5 text-slate-200 whitespace-nowrap">{r.category || '—'}</td>
                              <td className="px-2 py-1.5 text-slate-400">{r.subcategory || '—'}</td>
                              <td className="px-2 py-1.5 text-slate-300 font-mono max-w-[14rem] truncate" title={r.file_path ?? ''}>{r.file_path ? `${r.file_path}:${r.line_number ?? '?'}` : '—'}</td>
                              <td className="px-2 py-1.5 text-slate-200 max-w-xs truncate" title={r.content}>{r.content.slice(0, 90)}{r.content.length > 90 ? '…' : ''}</td>
                              <td className="px-2 py-1.5 text-slate-400 font-mono max-w-[9rem] truncate">{r.repository || '—'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )
                )}

                {previewDataset === 'severity_distribution' && (
                  <div className="overflow-x-auto rounded border border-slate-700">
                    <table className="min-w-full text-xs text-left">
                      <thead className="bg-slate-800 text-slate-400">
                        <tr><th className="px-2 py-1.5">Dimension</th><th className="px-2 py-1.5">Value</th><th className="px-2 py-1.5 text-right">Count</th></tr>
                      </thead>
                      <tbody className="divide-y divide-slate-700/60">
                        {severityDist.slice(0, 8).map((r, i) => (
                          <tr key={`${r.value}-${i}`} className="bg-slate-900/60">
                            <td className="px-2 py-1.5 text-slate-400">{r.dimension || 'severity'}</td>
                            <td className="px-2 py-1.5 text-slate-200">{r.value || '—'}</td>
                            <td className="px-2 py-1.5 text-right text-slate-100">{r.count.toLocaleString()}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                {previewDataset === 'category_distribution' && (
                  <div className="overflow-x-auto rounded border border-slate-700">
                    <table className="min-w-full text-xs text-left">
                      <thead className="bg-slate-800 text-slate-400">
                        <tr><th className="px-2 py-1.5">Dimension</th><th className="px-2 py-1.5">Value</th><th className="px-2 py-1.5 text-right">Count</th></tr>
                      </thead>
                      <tbody className="divide-y divide-slate-700/60">
                        {[...categoryDist.slice(0, 4), ...subcategoryDist.slice(0, 4)].map((r, i) => (
                          <tr key={`${r.value}-${i}`} className="bg-slate-900/60">
                            <td className="px-2 py-1.5 text-slate-400">{r.dimension || 'category'}</td>
                            <td className="px-2 py-1.5 text-slate-200">{r.value || '—'}</td>
                            <td className="px-2 py-1.5 text-right text-slate-100">{r.count.toLocaleString()}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                {previewDataset === 'trend' && (
                  <div className="space-y-2">
                    <TrendAreaChart rows={filledTrend} height={180} showBrush={false} showLegend={true} />
                    <div className="overflow-x-auto rounded border border-slate-700">
                      <table className="min-w-full text-xs text-left">
                        <thead className="bg-slate-800 text-slate-400">
                          <tr>
                            <th className="px-2 py-1.5">Bucket</th>
                            <th className="px-2 py-1.5 text-right">Findings</th>
                            <th className="px-2 py-1.5 text-right">Reviews</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-700/60">
                          {filledTrend.slice(0, 8).map((r, i) => (
                            <tr key={`${r.bucket}-${i}`} className="bg-slate-900/60">
                              <td className="px-2 py-1.5 text-slate-200">{r.bucket}</td>
                              <td className="px-2 py-1.5 text-right text-slate-100">{r.count.toLocaleString()}</td>
                              <td className="px-2 py-1.5 text-right text-emerald-300">{r.review_count.toLocaleString()}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}

                {previewDataset === 'breakdown' && (
                  <div className="overflow-x-auto rounded border border-slate-700">
                    <table className="min-w-full text-xs text-left">
                      <thead className="bg-slate-800 text-slate-400">
                        <tr>
                          <th className="px-2 py-1.5">Repository</th>
                          <th className="px-2 py-1.5">Provider</th>
                          <th className="px-2 py-1.5 text-right">Findings</th>
                          <th className="px-2 py-1.5 text-right">Reviews</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-700/60">
                        {breakdown.slice(0, 8).map((r, i) => (
                          <tr key={`${r.repository}-${r.provider}-${i}`} className="bg-slate-900/60">
                            <td className="px-2 py-1.5 text-slate-200">{r.repository || '—'}</td>
                            <td className="px-2 py-1.5 text-slate-400">{r.provider || '—'}</td>
                            <td className="px-2 py-1.5 text-right text-slate-100">{r.count.toLocaleString()}</td>
                            <td className="px-2 py-1.5 text-right text-emerald-300">{(r.review_count || 0).toLocaleString()}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>

            <div className="flex items-center gap-3">
              <span className="text-slate-300 text-sm">Format</span>
              <label className="text-sm text-slate-200 flex items-center gap-1"><input type="radio" checked={exportFormat === 'csv'} onChange={() => setExportFormat('csv')} /> CSV</label>
              <label className="text-sm text-slate-200 flex items-center gap-1"><input type="radio" checked={exportFormat === 'xlsx'} onChange={() => setExportFormat('xlsx')} /> XLSX (single package)</label>
            </div>

            <p className="text-[11px] text-slate-500">
              XLSX exports selected datasets as separate sheets in one workbook.
            </p>

            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowExportDialog(false)}
                className="px-3 py-1.5 rounded bg-slate-700 hover:bg-slate-600 text-white text-xs"
              >Cancel</button>
              <button
                onClick={runMultiExport}
                disabled={exportingDataset !== null}
                className="px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-500 text-white text-xs"
              >{exportingDataset ? 'Exporting…' : 'Export selected'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default TaxonomyReports;
