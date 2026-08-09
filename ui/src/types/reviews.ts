// TypeScript interfaces for Reviews API, matching backend Go structs

export interface Review {
  id: number;
  repository: string;
  branch?: string;
  commitHash?: string;
  prMrUrl?: string;
  connectorId?: number;
  status: ReviewStatus;
  triggerType: string;
  userEmail?: string;
  provider?: string;
  mrTitle?: string;
  friendlyName?: string;
  aiSummaryTitle?: string;
  authorName?: string;
  authorUsername?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  metadata?: Record<string, any>;
  orgId: number;
}

export type ReviewStatus = 'created' | 'in_progress' | 'completed' | 'failed';

export interface ReviewsListResponse {
  reviews: Review[];
  total: number;
  page: number;
  perPage: number;
  totalPages: number;
  hasNext: boolean;
  hasPrevious: boolean;
}

export interface ReviewEvent {
  id: number;
  reviewId: number;
  orgId: number;
  time: string;
  type: ReviewEventType;
  level?: ReviewEventLevel;
  batchId?: string;
  data: ReviewEventData;
}

export type ReviewEventType = 'status' | 'log' | 'batch' | 'artifact' | 'completion';
export type ReviewEventLevel = 'info' | 'warn' | 'error' | 'debug';

export interface ReviewEventData {
  // For "status" events
  status?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;

  // For "log" events
  message?: string;

  // For "batch" events
  tokenEstimate?: number;
  fileCount?: number;

  // For "artifact" events
  kind?: string;
  sizeBytes?: number;
  previewHead?: string;
  previewTail?: string;
  url?: string;

  // For "completion" events
  resultSummary?: string;
  commentCount?: number;
  errorSummary?: string;
}

export interface ReviewEventsResponse {
  events: ReviewEvent[];
  meta: {
    reviewId: number;
    count: number;
    limit: number;
    since?: string;
    eventType?: string;
  };
}

export interface ReviewSummary {
  reviewId: number;
  currentStatus: string;
  lastActivity: string;
  eventCounts: Record<string, number>;
  batchCount: number;
}

export interface ReviewAccountingOperation {
  operationType: string;
  triggerSource: string;
  operationId: string;
  idempotencyKey: string;
  billableLoc: number;
  accountedAt: string;
  provider?: string;
  model?: string;
  pricingVersion?: string;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
  metadata?: string;
}

export interface ReviewAccountingStage {
  stage: string;
  provider?: string;
  model?: string;
  pricingVersion?: string;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
}

export interface ReviewAccounting {
  reviewId: number;
  totalBillableLoc: number;
  accountedOperations: number;
  tokenTrackedOperations: number;
  lastAccountedAt?: string;
  totalInputTokens?: number;
  totalOutputTokens?: number;
  totalCostUsd?: number;
  helperEnabled?: boolean;
  helperMode?: string;
  stageBreakdown?: ReviewAccountingStage[];
  latestOperation?: ReviewAccountingOperation;
}

export interface ReviewCommit {
  ref: string;
  refType: 'commit' | 'range';
  createdAt: string;
}

export interface ReviewCommitsResponse {
  reviewId: number;
  commits: ReviewCommit[];
}

export type ReviewsSort = 'review' | 'branch' | 'repository' | 'source' | 'status' | 'author' | 'last_activity';

export interface ReviewsFilters {
  /** Comma-separated for multi-select (backend: multiValueFilterClause). */
  status?: string;
  /** Comma-separated short provider keys (backend: providerFilterClause prefix-matches compound values). */
  provider?: string;
  search?: string;
  sort?: ReviewsSort;
  order?: 'asc' | 'desc';
  page?: number;
  perPage?: number;
}

export interface CreateReviewRequest {
  url: string;
}

export interface CreateReviewResponse {
  message: string;
  url: string;
  reviewId: string;
  ai_execution_mode?: string;
  ai_execution_source?: string;
}

// API Error interface
export interface APIError {
  error: string;
  message?: string;
  status?: number;
}

// ===== Diff/findings viewer (GET /api/v1/diff-review/:review_id) =====
// Mirrors internal/api/diff_review.go's GetDiffReviewStatus response shape
// (marshalHunks/filterCommentsForFile), which reuses the same
// {files: [{file_path, hunks, comments}]} contract the git-lrc CLI already
// consumes via internal/reviewmodel.DiffReviewResponse.

export interface DiffReviewHunk {
  old_start_line: number;
  old_line_count: number;
  new_start_line: number;
  new_line_count: number;
  content: string;
  // Client-side only — attached by lib/blastRadius.ts's attachBlastData() by
  // joining against the blast-radius artifact; never part of the
  // GetDiffReviewStatus response itself. Mirrors git-lrc's
  // reviewmodel.DiffReviewHunk.BlastRadius / app.js's hunk.BlastDetail.
  BlastRadius?: number;
  BlastDetail?: BlastRadiusHunkReport;
}

export type DiffReviewCommentSeverity = 'info' | 'warning' | 'critical';

// Field list matches filterCommentsForFile in internal/api/diff_review.go
// exactly — comments are always matched against a hunk's new-side line range
// (lineWithinHunks), so `line` is always a new_start_line-relative number.
export interface DiffReviewComment {
  line: number;
  content: string;
  severity?: DiffReviewCommentSeverity;
  confidence?: string;
  type?: string;
  category?: string;
  subcategory?: string;
}

export interface DiffReviewFile {
  file_path: string;
  hunks: DiffReviewHunk[];
  comments: DiffReviewComment[];
  // Client-side only — set by lib/blastRadius.ts's flattenFilesByRisk() when
  // dissolving file boundaries into one globally-ranked single-hunk stream
  // (the "Risk score: whole diff" sort mode). A real file has neither field;
  // a synthetic per-hunk entry has both, disambiguating it from every other
  // synthetic entry sharing the same file_path. Mirrors git-lrc's
  // FileBlock.ID/SourceHunkNumber (blast_radius_sort_state.mjs's
  // flattenFilesByRisk).
  syntheticId?: string;
  sourceHunkNumber?: number;
}

export interface DiffReviewQuizQuestion {
  type: string;
  question: string;
  options: string[];
  correctIndex: number;
  explanation?: string;
}

export interface DiffReviewStatusResponse {
  status: 'created' | 'in_progress' | 'completed' | 'failed';
  review_id: string;
  message?: string;
  friendly_name?: string;
  summary?: string;
  files?: DiffReviewFile[];
  excluded_files?: string[];
  ai_summary_title?: string;
  quiz?: DiffReviewQuizQuestion[];
}

// ===== Blast radius artifact (GET /api/v1/diff-review/:review_id/artifacts/blast-radius) =====
// Mirrors git-lrc's blastradius.Report struct (blastradius/blastradius.go) field-for-field
// (Go's default JSON marshaling keeps the exact PascalCase field names, no json tags) — this
// is the report git-lrc's CLI uploads verbatim via PutDiffReviewArtifact, so these types are
// the wire contract, not a LiveReview-side reinterpretation.

export interface BlastRadiusSignal {
  Name: string;
  Detail?: string;
  Points: number;
  Category: 'architecture' | 'graph' | 'duplication' | 'code-metrics' | 'diff-shape' | string;
  // Added client-side by allSignals() (ui/src/lib/blastRadius.ts) when
  // flattening a symbol's own signals into the hunk-level list — never part
  // of the server's JSON.
  _symbolName?: string;
}

export interface BlastRadiusCallerRef {
  QualifiedName: string;
  Depth: number;
  Weight: number;
  Path?: string[];
  PreRename?: boolean;
}

export interface BlastRadiusSymbolContribution {
  QualifiedName: string;
  Name: string;
  Label: string;
  Method: 'calls' | 'text-references' | string;
  Signals: BlastRadiusSignal[];
  BlastRadiusRaw: number;
  ReviewPriorityRaw: number;
  DirectCount: number;
  TransitiveCount: number;
  Callers?: BlastRadiusCallerRef[];
  RenamedFrom?: string;
  ImpactedPackages?: string[];
  MethodBlastRadius?: number;
  IsEntryPoint?: boolean;
  Complexity?: number;
  Cognitive?: number;
  LoopDepth?: number;
  OutDegree?: number;
  TestCount?: number;
}

export interface BlastRadiusWeights {
  BlastRadius: number;
  ReviewPriority: number;
}

export interface BlastRadiusHunkReport {
  FilePath: string;
  Header: string;
  NewStart: number;
  NewLines: number;
  Content?: string;
  Signals?: BlastRadiusSignal[];
  BlastRadiusRaw: number;
  BlastRadiusNorm: number;
  MaxBlastRadiusRaw: number;
  MaxBlastRadiusHunkFile?: string;
  MaxBlastRadiusHunkHeader?: string;
  ReviewPriorityRaw: number;
  ReviewPriorityNorm: number;
  MaxReviewPriorityRaw: number;
  MaxReviewPriorityHunkFile?: string;
  MaxReviewPriorityHunkHeader?: string;
  Combined: number;
  HygieneMultiplier: number;
  Weights: BlastRadiusWeights;
  Symbols?: BlastRadiusSymbolContribution[];
  ImpactedPackages?: string[];
  FileCouplingBonus?: number;
}

export interface BlastRadiusFileReport {
  Path: string;
  Hunks: BlastRadiusHunkReport[];
}

export interface BlastRadiusPackageImpact {
  Package: string;
  HunkCount: number;
  MaxBlastRadiusRaw: number;
}

export interface BlastRadiusReport {
  Project: string;
  GeneratedAt: string;
  Files: BlastRadiusFileReport[];
  ImpactedPackages?: BlastRadiusPackageImpact[];
}