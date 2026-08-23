// Client-side re-bucketing + trend smoothing for the Day/Week/Month toggle
// on flat trend-line charts. The backend emits daily-granularity data_sql
// for these (see the livi.charts.basics lawbook rule), so switching
// granularity here just re-aggregates the already-fetched rows instead of
// round-tripping to the server. This also builds the same
// raw + rolling-average + period-baseline layered view the counted-event
// law describes, so a flat chart gets the same reading aid regardless of
// which shape the model happened to pick.
export type Granularity = 'day' | 'week' | 'month';

interface TemporalEncoding {
  field: string;
  type: string;
  timeUnit?: string;
  title?: string;
  [key: string]: unknown;
}

interface QuantEncoding {
  field: string;
  type?: string;
  title?: string;
  [key: string]: unknown;
}

function getEncoding(spec: Record<string, unknown>): Record<string, any> | null {
  const enc = (spec as any).encoding;
  return enc && typeof enc === 'object' ? enc : null;
}

// Only the simple single-mark shape (no facet/layer/concat) is supported -
// that covers the plain trend-line case this toggle targets. Anything else
// (the layered rolling-average charts some laws already build themselves)
// just doesn't get a toggle, since re-bucketing a pre-computed rolling
// average or baseline would silently corrupt it.
export function isDailyTrendChart(spec: Record<string, unknown>): boolean {
  if (!spec || 'layer' in spec || 'facet' in spec || 'hconcat' in spec || 'vconcat' in spec) return false;
  const enc = getEncoding(spec);
  if (!enc) return false;
  const x = enc.x as TemporalEncoding | undefined;
  const y = enc.y as QuantEncoding | undefined;
  if (!x || x.type !== 'temporal' || !x.field) return false;
  if (!y || y.type !== 'quantitative' || !y.field) return false;
  const values = (spec as any).data?.values;
  return Array.isArray(values) && values.length > 0;
}

function bucketStart(dateStr: string, granularity: Granularity): string {
  const d = new Date(dateStr);
  if (granularity === 'month') {
    return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1)).toISOString().slice(0, 10);
  }
  // week: bucket to the Monday of that ISO week
  const day = d.getUTCDay(); // 0 = Sunday
  const diffToMonday = (day + 6) % 7;
  const monday = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() - diffToMonday));
  return monday.toISOString().slice(0, 10);
}

const TIME_UNIT: Record<Granularity, string> = {
  day: 'yearmonthdate',
  week: 'yearweek',
  month: 'yearmonth',
};

// The window (in buckets) the rolling average smooths over, per granularity:
// a 7-day, a 4-week, and a 4-month ("quarterly") rolling average.
const ROLLING_WINDOW: Record<Granularity, number> = { day: 7, week: 4, month: 4 };

// The rolling average only earns its place once there's more than about two
// windows' worth of buckets - otherwise it's mostly retracing the raw line
// (or, in month mode with only a handful of points, adds nothing at all).
const MIN_BUCKETS_FOR_ROLLING: Record<Granularity, number> = { day: 14, week: 8, month: 8 };

// Sums the y field within each bucket, grouped by the color/series field too
// when the chart has one (multi-series lines keep their series apart).
function bucketRows(
  values: Array<Record<string, unknown>>,
  xField: string,
  yField: string,
  seriesField: string | undefined,
  granularity: Granularity,
): Array<Record<string, unknown>> {
  const grouped = new Map<string, Record<string, unknown>>();
  for (const row of values) {
    const rawDate = row[xField];
    if (typeof rawDate !== 'string' && !(rawDate instanceof Date)) continue;
    const bucket = granularity === 'day' ? String(rawDate).slice(0, 10) : bucketStart(String(rawDate), granularity);
    const seriesKey = seriesField ? String(row[seriesField]) : '';
    const key = `${bucket} ${seriesKey}`;
    const yVal = Number(row[yField]) || 0;
    const existing = grouped.get(key);
    if (existing) {
      existing[yField] = (Number(existing[yField]) || 0) + yVal;
    } else {
      grouped.set(key, { ...row, [xField]: bucket, [yField]: yVal });
    }
  }
  return Array.from(grouped.values()).sort((a, b) => String(a[xField]).localeCompare(String(b[xField])));
}

// Adds a trailing rolling-average column, windowed per series so one
// series's values never bleed into another's average.
function withRollingAverage(
  rows: Array<Record<string, unknown>>,
  xField: string,
  yField: string,
  seriesField: string | undefined,
  window: number,
  rollingField: string,
): Array<Record<string, unknown>> {
  const bySeries = new Map<string, Array<Record<string, unknown>>>();
  for (const row of rows) {
    const key = seriesField ? String(row[seriesField]) : '';
    if (!bySeries.has(key)) bySeries.set(key, []);
    bySeries.get(key)!.push(row);
  }
  const out: Array<Record<string, unknown>> = [];
  for (const seriesRows of bySeries.values()) {
    for (let i = 0; i < seriesRows.length; i++) {
      const windowRows = seriesRows.slice(Math.max(0, i - window + 1), i + 1);
      const avg = windowRows.reduce((sum, r) => sum + (Number(r[yField]) || 0), 0) / windowRows.length;
      out.push({ ...seriesRows[i], [rollingField]: Math.round(avg * 100) / 100 });
    }
  }
  return out.sort((a, b) => String(a[xField]).localeCompare(String(b[xField])));
}

function markType(mark: unknown): string {
  if (typeof mark === 'string') return mark;
  if (mark && typeof mark === 'object' && typeof (mark as any).type === 'string') return (mark as any).type;
  return 'line';
}

// Builds the granularity-aware trend view: the re-bucketed raw series, plus
// (once there's enough data) a rolling-average line and a period-average
// baseline rule - the same reading aid livi.charts.trend.counted_event
// gives its own layered charts, applied here to any flat trend chart.
export function buildTrendSpec(spec: Record<string, unknown>, granularity: Granularity): Record<string, unknown> {
  const enc = getEncoding(spec)!;
  const x = enc.x as TemporalEncoding;
  const y = enc.y as QuantEncoding;
  const seriesField = (enc.color as { field?: string } | undefined)?.field;
  const values = (spec as any).data.values as Array<Record<string, unknown>>;

  const bucketed = bucketRows(values, x.field, y.field, seriesField, granularity);
  const window = ROLLING_WINDOW[granularity];
  const isNormalizedShare = (y as { stack?: unknown }).stack === 'normalize';
  const showRolling = !isNormalizedShare && bucketed.length >= MIN_BUCKETS_FOR_ROLLING[granularity];

  const xEnc = { ...x, timeUnit: TIME_UNIT[granularity] };
  const colorEnc = seriesField ? { color: enc.color } : {};

  const layers: Record<string, unknown>[] = [
    {
      data: { values: bucketed },
      mark: showRolling
        ? { type: 'area', opacity: 0.45, color: '#7c9cff', interpolate: 'monotone', line: { color: '#7c9cff', strokeWidth: 1.5 }, point: { color: '#7c9cff', size: 25 } }
        : { type: markType((spec as any).mark), color: '#7c9cff', point: { color: '#7c9cff', size: 30 } },
      encoding: { x: xEnc, y: enc.y, ...colorEnc },
    },
  ];

  if (showRolling) {
    const rollingField = `${y.field}_rolling_avg`;
    const rollingRows = withRollingAverage(bucketed, x.field, y.field, seriesField, window, rollingField);
    const windowLabel = granularity === 'day' ? `${window}-day` : granularity === 'week' ? `${window}-week` : `${window}-month`;
    layers.push({
      data: { values: rollingRows },
      mark: { type: 'line', strokeWidth: 2.5, color: '#ffb454', interpolate: 'monotone' },
      encoding: {
        x: xEnc,
        y: { field: rollingField, type: 'quantitative', title: `${windowLabel} rolling avg` },
        ...colorEnc,
      },
    });
  }

  // Period-average baseline, aggregated straight off the bucketed data so
  // it always matches what's actually on screen for the current toggle.
  // Skipped for a normalized share chart for the same reason as the
  // rolling average above - a mean of raw counts doesn't belong on a 0-1 axis.
  if (!isNormalizedShare) {
    layers.push({
      data: { values: bucketed },
      mark: { type: 'rule', strokeDash: [6, 4], color: '#ff5c7c', strokeWidth: 1.5 },
      encoding: { y: { field: y.field, type: 'quantitative', aggregate: 'mean' } },
    });
  }

  const rest = { ...spec };
  delete (rest as any).encoding;
  delete (rest as any).mark;
  delete (rest as any).data;

  return { ...rest, layer: layers, resolve: { scale: { y: 'shared' } } };
}

// Formats a bucket's date the same way the x-axis reads it (e.g. "Aug 01,
// 2026"), so the summary stats below the chart never disagree with the
// axis's own labels.
export function formatAxisDate(dateStr: string): string {
  const d = new Date(dateStr);
  if (Number.isNaN(d.getTime())) return dateStr;
  return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit', year: 'numeric', timeZone: 'UTC' });
}

export interface TrendStats {
  total: number;
  avgPerPeriod: number;
  peak: { value: number; date: string };
  low: { value: number; date: string };
  trendPct: number | null;
  firstDate: string;
  lastDate: string;
}

// Summary stats for the currently selected granularity - recomputed on
// every toggle change since "total"/"avg per period"/peak all shift with
// the bucket size. Only defined for single-series trend charts; a
// multi-series chart has no single "total" that reads honestly.
export function computeTrendStats(spec: Record<string, unknown>, granularity: Granularity): TrendStats | null {
  const enc = getEncoding(spec);
  if (!enc) return null;
  const x = enc.x as TemporalEncoding;
  const y = enc.y as QuantEncoding;
  const seriesField = (enc.color as { field?: string } | undefined)?.field;
  if (seriesField) return null;
  const values = (spec as any).data?.values as Array<Record<string, unknown>> | undefined;
  if (!Array.isArray(values) || values.length === 0) return null;

  const rows = bucketRows(values, x.field, y.field, undefined, granularity);
  if (rows.length === 0) return null;

  let total = 0;
  let peak = rows[0];
  let low = rows[0];
  for (const row of rows) {
    const v = Number(row[y.field]) || 0;
    total += v;
    if (v > (Number(peak[y.field]) || 0)) peak = row;
    if (v < (Number(low[y.field]) || 0)) low = row;
  }
  const first = Number(rows[0][y.field]) || 0;
  const last = Number(rows[rows.length - 1][y.field]) || 0;
  const trendPct = first === 0 ? null : Math.round(((last - first) / first) * 100);

  return {
    total: Math.round(total * 100) / 100,
    avgPerPeriod: Math.round((total / rows.length) * 100) / 100,
    peak: { value: Number(peak[y.field]) || 0, date: String(peak[x.field]) },
    low: { value: Number(low[y.field]) || 0, date: String(low[x.field]) },
    trendPct,
    firstDate: String(rows[0][x.field]),
    lastDate: String(rows[rows.length - 1][x.field]),
  };
}

interface CategoryEncodingParts {
  catField: string;
  valField: string;
}

function getCategoryEncoding(spec: Record<string, unknown>): CategoryEncodingParts | null {
  const enc = getEncoding(spec);
  if (!enc) return null;
  const x = enc.x as { field?: string; type?: string } | undefined;
  const y = enc.y as { field?: string; type?: string } | undefined;
  if (x?.type === 'quantitative' && x.field && (y?.type === 'nominal' || y?.type === 'ordinal') && y.field) {
    return { catField: y.field, valField: x.field };
  }
  if (y?.type === 'quantitative' && y.field && (x?.type === 'nominal' || x?.type === 'ordinal') && x.field) {
    return { catField: x.field, valField: y.field };
  }
  return null;
}

// Only the simple single-mark bar/dot shape (one category axis, one
// quantitative axis, no facet/layer) gets a stats summary - a stacked or
// grouped chart's per-category total isn't a single unambiguous number. A
// color channel is still fine as long as it just tints each category by
// itself (the distribution-band case, color.field === the category field)
// rather than encoding an actual second grouping dimension.
export function isCategoricalChart(spec: Record<string, unknown>): boolean {
  if (!spec || 'layer' in spec || 'facet' in spec || 'hconcat' in spec || 'vconcat' in spec) return false;
  const enc = getEncoding(spec);
  const parts = getCategoryEncoding(spec);
  if (!parts) return false;
  const colorField = (enc?.color as { field?: string } | undefined)?.field;
  if (colorField && colorField !== parts.catField) return false;
  const values = (spec as any).data?.values;
  return Array.isArray(values) && values.length > 0;
}

// Detects a distribution/adoption-band bar chart - same heuristic the
// backend's injectDistributionBandColor uses (see
// internal/mcpagent/interpretation_sanitization.go): the category field's
// name looks like a band/level/tier.
export function isBandChart(spec: Record<string, unknown>): boolean {
  if (!isCategoricalChart(spec)) return false;
  const parts = getCategoryEncoding(spec)!;
  const field = parts.catField.toLowerCase();
  return field.includes('band') || field.includes('level') || field.includes('tier');
}

export interface BandStats {
  totalActive: number;
  largest: { label: string; value: number };
  largestSharePct: number;
}

// KPI tiles for a band chart, in the spirit of
// scripts/adoption_chart/generate_breadth.py's "Engineers active / Median
// reviews per engineer / Top contributor's share" row - but only what's
// honestly derivable from the chart's own already-bucketed data. The
// per-engineer raw counts behind each band aren't available client-side
// (the SQL already aggregated them into bands before the chart spec was
// built), so this reports total active + which band is largest and its
// share, rather than fabricating a median or a top-contributor figure this
// chart's data can't actually support.
export function computeBandStats(spec: Record<string, unknown>): BandStats | null {
  const stats = computeCategoryStats(spec);
  if (!stats) return null;
  const values = (spec as any).data.values as Array<Record<string, unknown>>;
  const parts = getCategoryEncoding(spec)!;
  let totalActive = 0;
  for (const row of values) {
    const label = String(row[parts.catField]);
    if (/^\s*0\b/.test(label) || /\bnone\b/i.test(label)) continue;
    totalActive += Number(row[parts.valField]) || 0;
  }
  if (totalActive === 0) return null;
  return {
    totalActive,
    largest: stats.highest,
    largestSharePct: Math.round((stats.highest.value / totalActive) * 100),
  };
}

export interface CategoryStat {
  label: string;
  value: number;
}

export interface CategoryStats {
  highest: CategoryStat;
  lowest: CategoryStat;
  top3: CategoryStat[];
  bottom3: CategoryStat[];
}

export function computeCategoryStats(spec: Record<string, unknown>): CategoryStats | null {
  const parts = getCategoryEncoding(spec);
  if (!parts) return null;
  const values = (spec as any).data?.values as Array<Record<string, unknown>> | undefined;
  if (!Array.isArray(values) || values.length === 0) return null;

  const grouped = new Map<string, number>();
  for (const row of values) {
    const label = String(row[parts.catField]);
    const val = Number(row[parts.valField]) || 0;
    grouped.set(label, (grouped.get(label) ?? 0) + val);
  }
  const sorted = Array.from(grouped.entries())
    .map(([label, value]) => ({ label, value }))
    .sort((a, b) => b.value - a.value);
  if (sorted.length === 0) return null;

  return {
    highest: sorted[0],
    lowest: sorted[sorted.length - 1],
    top3: sorted.slice(0, 3),
    bottom3: sorted.slice(-3).reverse(),
  };
}
