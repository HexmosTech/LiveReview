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

interface AnalyzedEncoding {
  temporal?: { channel: string; field: string; enc: TemporalEncoding };
  categorical?: { channel: string; field: string; enc: QuantEncoding };
  quantitative: Array<{ channel: string; field: string; enc: QuantEncoding }>;
  color?: { field?: string };
}

// Scans every encoding channel (x, y, theta, ...) by declared `type` instead
// of assuming the field we want is always on x or y. The model is allowed to
// swap axes on request (livi.interpreting.chart-rules law 10) and some chart
// types put their category/value on non-x/y channels (e.g. pie's `theta`) -
// a fixed x/y assumption silently drops the toggle/stats for any of those.
function analyzeEncoding(enc: Record<string, any>): AnalyzedEncoding {
  const result: AnalyzedEncoding = { quantitative: [] };
  for (const [channel, e] of Object.entries(enc)) {
    if (!e || typeof e !== 'object' || typeof (e as any).field !== 'string') continue;
    const field = (e as any).field as string;
    const type = (e as any).type;
    if (channel === 'color') continue; // color is read separately below, as a series/grouping key
    if (type === 'temporal' && !result.temporal) {
      result.temporal = { channel, field, enc: e as TemporalEncoding };
    } else if ((type === 'nominal' || type === 'ordinal') && !result.categorical) {
      result.categorical = { channel, field, enc: e as QuantEncoding };
    } else if (type === 'quantitative') {
      result.quantitative.push({ channel, field, enc: e as QuantEncoding });
    }
  }
  const color = enc.color;
  if (color && typeof color === 'object' && typeof (color as any).field === 'string') {
    result.color = { field: (color as any).field };
  }
  return result;
}

function getTrendFields(
  spec: Record<string, unknown>,
): { x: TemporalEncoding; y: QuantEncoding; seriesField?: string } | null {
  const enc = getEncoding(spec);
  if (!enc) return null;
  const found = analyzeEncoding(enc);
  if (!found.temporal || found.quantitative.length === 0) return null;
  return { x: found.temporal.enc, y: found.quantitative[0].enc, seriesField: found.color?.field };
}

// Only the simple single-mark shape (no facet/layer/concat) is supported -
// that covers the plain trend-line case this toggle targets. Anything else
// (the layered rolling-average charts some laws already build themselves)
// just doesn't get a toggle, since re-bucketing a pre-computed rolling
// average or baseline would silently corrupt it.
export function isDailyTrendChart(spec: Record<string, unknown>): boolean {
  if (!spec || 'layer' in spec || 'facet' in spec || 'hconcat' in spec || 'vconcat' in spec) return false;
  if (!getTrendFields(spec)) return false;
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
  const trend = getTrendFields(spec)!;
  const x = trend.x;
  const y = trend.y;
  const seriesField = trend.seriesField;
  const enc = getEncoding(spec)!;
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
      encoding: { x: xEnc, y, ...colorEnc },
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
  const trend = getTrendFields(spec);
  if (!trend) return null;
  const { x, y, seriesField } = trend;
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

export interface MultiSeriesTrendStats {
  total: number;
  seriesCount: number;
  topSeries: { label: string; value: number };
  firstDate: string;
  lastDate: string;
}

// Companion to computeTrendStats for the multi-series case (a color-encoded
// trend chart, e.g. "adoption by trigger type over time") - there's no
// single day's "peak" that means anything across series, but a per-series
// breakdown (total, which series dominates) is still an honest summary.
export function computeMultiSeriesTrendStats(
  spec: Record<string, unknown>,
  granularity: Granularity,
): MultiSeriesTrendStats | null {
  const trend = getTrendFields(spec);
  if (!trend || !trend.seriesField) return null;
  const { x, y, seriesField } = trend;
  const values = (spec as any).data?.values as Array<Record<string, unknown>> | undefined;
  if (!Array.isArray(values) || values.length === 0) return null;

  const rows = bucketRows(values, x.field, y.field, seriesField, granularity);
  if (rows.length === 0) return null;

  const bySeries = new Map<string, number>();
  for (const row of rows) {
    const key = String(row[seriesField]);
    bySeries.set(key, (bySeries.get(key) ?? 0) + (Number(row[y.field]) || 0));
  }
  const sorted = Array.from(bySeries.entries())
    .map(([label, value]) => ({ label, value }))
    .sort((a, b) => b.value - a.value);
  const total = sorted.reduce((sum, s) => sum + s.value, 0);
  if (total === 0) return null;

  return {
    total: Math.round(total * 100) / 100,
    seriesCount: sorted.length,
    topSeries: { label: sorted[0].label, value: Math.round(sorted[0].value * 100) / 100 },
    firstDate: String(rows[0][x.field]),
    lastDate: String(rows[rows.length - 1][x.field]),
  };
}

interface CategoryEncodingParts {
  catField: string;
  valField: string;
}

// Falls back to a layered spec's first layer (e.g. a `pareto` chart's raw
// `bar` layer, ahead of its cumulative-percent `line` layer) when there's no
// top-level `encoding` - that first layer carries the same per-category raw
// value a plain bar chart would, so it can drive the same stats chips even
// though the chart itself isn't eligible for the day/week/month toggle.
function getPrimaryEncoding(spec: Record<string, unknown>): Record<string, any> | null {
  const enc = getEncoding(spec);
  if (enc) return enc;
  const layer = (spec as any).layer;
  if (Array.isArray(layer) && layer.length > 0) {
    const first = layer[0];
    if (first && typeof first === 'object' && first.encoding && typeof first.encoding === 'object') {
      return first.encoding;
    }
  }
  return null;
}

function getCategoryEncoding(spec: Record<string, unknown>): CategoryEncodingParts | null {
  const enc = getPrimaryEncoding(spec);
  if (!enc) return null;
  const found = analyzeEncoding(enc);
  if (found.categorical && found.quantitative.length > 0) {
    return { catField: found.categorical.field, valField: found.quantitative[0].field };
  }
  // Histogram fallback: the livi.charts base spec types the bucket axis
  // "ordinal", but the model sometimes leaves it "quantitative" (a numeric
  // bucket start/label) instead - it's still a discrete bucket axis, just
  // mistyped, and a `bar` mark is the tell (a real two-quantitative-field
  // chart, e.g. a scatter plot, uses a point/circle mark instead).
  const mark = markType((spec as any).mark ?? (spec as any).layer?.[0]?.mark);
  if (mark === 'bar' && found.quantitative.length >= 2) {
    const xField = (enc.x as { field?: string } | undefined)?.field;
    const yField = (enc.y as { field?: string } | undefined)?.field;
    if (typeof xField === 'string' && typeof yField === 'string' && xField !== yField) {
      return { catField: xField, valField: yField };
    }
  }
  return null;
}

// The simple single-mark bar/dot shape (one category axis, one quantitative
// axis, no facet/concat) gets a stats summary - a stacked or grouped chart's
// per-category total isn't a single unambiguous number. A color channel is
// still fine as long as it just tints each category by itself (the
// distribution-band case, color.field === the category field) rather than
// encoding an actual second grouping dimension. A layered spec (e.g.
// `pareto`'s bar+cumulative-line) is also allowed through - see
// getPrimaryEncoding - since its first layer is itself a plain per-category
// bar; the second (line) layer is ignored for stats purposes.
export function isCategoricalChart(spec: Record<string, unknown>): boolean {
  if (!spec || 'facet' in spec || 'hconcat' in spec || 'vconcat' in spec) return false;
  const enc = getPrimaryEncoding(spec);
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

interface HeatmapEncodingParts {
  dateField: string;
  colorField: string;
}

// Detects a calendar-heatmap-shaped chart the same way the backend's
// sanitizeCalendarHeatmap does (see internal/mcpagent/interpretation_sanitization.go):
// both x and y are the same underlying date field, just bucketed by two
// different timeUnits (one "week", one plain "day"), with a quantitative
// color channel. Neither axis is a single re-bucketable temporal/quantitative
// pair, so this chart never gets the day/week/month toggle - it gets its own
// stats instead.
function getHeatmapEncoding(spec: Record<string, unknown>): HeatmapEncodingParts | null {
  if (!spec || 'layer' in spec || 'facet' in spec || 'hconcat' in spec || 'vconcat' in spec) return null;
  const enc = getEncoding(spec);
  if (!enc) return null;
  const x = enc.x as TemporalEncoding | undefined;
  const y = enc.y as TemporalEncoding | undefined;
  const color = enc.color as QuantEncoding | undefined;
  if (!x?.field || !y?.field || x.field !== y.field) return null;
  if (!color?.field || color.type !== 'quantitative') return null;
  const isWeek = (e?: TemporalEncoding) => typeof e?.timeUnit === 'string' && e.timeUnit.toLowerCase().includes('week');
  const isDay = (e?: TemporalEncoding) => e?.timeUnit === 'day';
  if (!((isWeek(x) && isDay(y)) || (isWeek(y) && isDay(x)))) return null;
  return { dateField: x.field, colorField: color.field };
}

export function isCalendarHeatmap(spec: Record<string, unknown>): boolean {
  if (!getHeatmapEncoding(spec)) return false;
  const values = (spec as any).data?.values;
  return Array.isArray(values) && values.length > 0;
}

export interface HeatmapStats {
  total: number;
  activeDays: number;
  avgOnActiveDays: number;
  busiest: { date: string; value: number };
}

export function computeHeatmapStats(spec: Record<string, unknown>): HeatmapStats | null {
  const parts = getHeatmapEncoding(spec);
  if (!parts) return null;
  const values = (spec as any).data?.values as Array<Record<string, unknown>> | undefined;
  if (!Array.isArray(values) || values.length === 0) return null;

  let total = 0;
  let activeDays = 0;
  let busiest = values[0];
  for (const row of values) {
    const v = Number(row[parts.colorField]) || 0;
    total += v;
    if (v > 0) activeDays++;
    if (v > (Number(busiest[parts.colorField]) || 0)) busiest = row;
  }
  if (total === 0) return null;

  return {
    total: Math.round(total * 100) / 100,
    activeDays,
    avgOnActiveDays: activeDays > 0 ? Math.round((total / activeDays) * 100) / 100 : 0,
    busiest: { date: String(busiest[parts.dateField]), value: Number(busiest[parts.colorField]) || 0 },
  };
}
