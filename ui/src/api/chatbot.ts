import apiClient from './apiClient';

export interface ChatMessage {
  role: 'user' | 'assistant';
  text: string;
}

// Who/what a chart or export is scoped to. organization is always the real
// org name; repository/person list the specific names the question narrows
// to, empty when the question names none.
export interface ChartContext {
  organization: string;
  repository?: string[];
  person?: string[];
}

// Precomputed KPI chips for a chart, mirroring internal/chatstats.AllStats -
// computed once on the backend at chart-build time (see
// internal/chatstats/chatstats.go) instead of recomputed client-side on
// every day/week/month toggle click. `day`/`week`/`month` are populated only
// for the two trend-shaped kinds; `stats` is populated for everything else.
// Undefined entirely when the chart's shape has no precomputed KPIs (e.g. a
// layered/faceted spec).
export interface TrendStats {
  total: number;
  avgPerPeriod: number;
  peak: { value: number; date: string };
  low: { value: number; date: string };
  trendPct: number | null;
  firstDate: string;
  lastDate: string;
}

export interface MultiSeriesTrendStats {
  total: number;
  seriesCount: number;
  topSeries: { label: string; value: number };
  firstDate: string;
  lastDate: string;
}

export interface CategoryStat {
  label: string;
  value: number;
}

export interface BandStats {
  totalActive: number;
  largest: CategoryStat;
  largestSharePct: number;
}

export interface CategoryStats {
  highest: CategoryStat;
  lowest: CategoryStat;
  top3: CategoryStat[];
  bottom3: CategoryStat[];
}

export interface HeatmapStats {
  total: number;
  activeDays: number;
  avgOnActiveDays: number;
  busiest: { date: string; value: number };
}

export interface SlopeStats {
  entityCount: number;
  gained: number;
  lost: number;
  flat: number;
  biggestGain: { label: string; delta: number };
  biggestLoss: { label: string; delta: number };
}

// Fallback for any chart shape none of the other kinds recognize (pie/arc,
// scatter, ...) - a crude per-row Total/Count/Highest/Lowest off whatever
// quantitative field exists, so no chart is ever left with zero numbers.
export interface GenericStat {
  label: string;
  value: number;
}

export interface GenericStats {
  total: number;
  count: number;
  highest: GenericStat;
  lowest: GenericStat;
}

export type ChartStats =
  | { kind: 'trend'; day?: TrendStats; week?: TrendStats; month?: TrendStats }
  | { kind: 'multi_series_trend'; day?: MultiSeriesTrendStats; week?: MultiSeriesTrendStats; month?: MultiSeriesTrendStats }
  | { kind: 'band'; stats: BandStats }
  | { kind: 'heatmap'; stats: HeatmapStats }
  | { kind: 'slope'; stats: SlopeStats }
  | { kind: 'category'; stats: CategoryStats }
  | { kind: 'generic'; stats: GenericStats };

// A chart report the backend hands back as a raw Vega-Lite spec rather than a
// rendered image, so the frontend can render it interactively (tooltips,
// hover, legend filtering) instead of a flat PNG. `id` is only present once
// the chart has been persisted (loaded via getConversation) - a chart fresh
// off a live turn doesn't have one yet.
export interface ChatChart {
  id?: number;
  title?: string;
  description?: string;
  query?: string;
  time_range?: string;
  granularity?: string;
  context?: ChartContext;
  spec: Record<string, unknown>;
  stats?: ChartStats;
}

// A downloadable export (CSV) produced alongside an answer. Unlike charts,
// these are served from an authenticated endpoint scoped to the caller's org,
// so they must be fetched with the usual auth headers rather than by <a href>.
export interface ChatFile {
  url: string;
  filename: string;
  title?: string;
  description?: string;
  query?: string;
  time_range?: string;
  granularity?: string;
  context?: ChartContext;
  rows?: number;
}

export interface SuggestedQuestionCategory {
  category: string;
  questions: string[];
}

export interface ChatResponse {
  response: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  suggested_questions?: SuggestedQuestionCategory[];
  debug_artifacts?: unknown;
  sessionId?: string;
  conversationId: number;
}

// The backend now owns conversation history: it loads prior turns by
// conversationId and persists this one, so the client only ever sends the
// new message plus which conversation it belongs to (omitted to start a new
// one).
export async function sendChatMessage(
  message: string,
  conversationId?: number,
): Promise<ChatResponse> {
  return apiClient.post<ChatResponse>('/api/v1/chat/send', {
    message,
    conversationId: conversationId ?? undefined,
  });
}
