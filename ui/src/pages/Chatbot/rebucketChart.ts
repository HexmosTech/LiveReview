// Client-side re-bucketing + trend smoothing for the Day/Week/Month toggle
// on flat trend-line charts. The backend emits daily-granularity data_sql
// for these (see the livi.charts.basics lawbook rule), so switching
// granularity here just re-aggregates the already-fetched rows instead of
// round-tripping to the server. This also builds the same
// raw + rolling-average + period-baseline layered view the counted-event
// law describes, so a flat chart gets the same reading aid regardless of
// which shape the model happened to pick.
//
// This file only builds chart *pixels* (the rebucketed points, rolling-avg
// layer, baseline rule). The KPI numbers shown alongside a chart (Total/Avg
// per period/Peak/Low/Trend, and the band/heatmap/slope/category
// equivalents) are precomputed backend-side - see
// internal/chatstats/chatstats.go and ChatChart.stats in api/chatbot.ts -
// and just get displayed, not recomputed here.
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
// a fixed x/y assumption silently drops the toggle for any of those.
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
