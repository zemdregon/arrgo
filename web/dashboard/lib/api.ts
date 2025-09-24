import { HttpClient } from "./httpClient";
import {
  finishTrace,
  startClientTrace,
  TraceAttributes,
} from "./observability/tracing";

const FALLBACK_BASE_URL = "http://localhost:8080";

export type TokenProvider = () => string | undefined;

const defaultTokenProvider: TokenProvider = () =>
  process.env.NEXT_PUBLIC_ARRGO_API_TOKEN?.trim() || undefined;

let resolveApiToken: TokenProvider = defaultTokenProvider;

export function setApiTokenProvider(provider?: TokenProvider) {
  resolveApiToken = provider ?? defaultTokenProvider;
}

function resolveBaseUrl(baseUrl?: string): string {
  const value = baseUrl ?? process.env.NEXT_PUBLIC_ARRGO_API_URL ?? FALLBACK_BASE_URL;
  return value.replace(/\/$/, "");
}

export interface LibraryEntity {
  id: string;
  entityType: string;
  title: string;
  synopsis: string;
  pendingImports: number;
  qualityUpgrades: number;
  failures: number;
}

export interface JobSummary {
  id: string;
  jobType: string;
  targetID: string;
  state: string;
  submittedAt?: string;
  payloadURI?: string;
  idempotencyKey?: string;
  attempt: number;
  lastAttemptAt?: string;
  completedAt?: string;
}

export interface WorkerSummary {
  id: string;
  pool: string;
  capabilities: string[];
  endpoint: string;
  version: string;
  status: string;
  registeredAt?: string;
  lastHeartbeatAt?: string;
  activeJobs: number;
}

export type QueueHealthStatus = "healthy" | "warning" | "critical";

export interface QueueStreamLagMetric {
  stream: string;
  consumerGroup: string;
  lagSeconds?: number;
  lagMessages?: number;
  oldestPendingAgeSeconds?: number;
  lastProducedAt?: string;
  lastConsumedAt?: string;
  status: QueueHealthStatus;
}

export interface QueueRetryMetric {
  jobType: string;
  retryCount: number;
  recentFailureRate?: number;
  maxAttempts?: number;
  oldestRetryAgeSeconds?: number;
  lastRetryAt?: string;
  status: QueueHealthStatus;
}

export interface QueueSlaIndicator {
  queue: string;
  targetSeconds?: number;
  p95Seconds?: number;
  p99Seconds?: number;
  successRate?: number;
  breachesToday?: number;
  status: QueueHealthStatus;
  updatedAt?: string;
}

export interface QueueAnalyticsOverview {
  updatedAt?: string;
  observedWindowSeconds?: number;
  streamLag: QueueStreamLagMetric[];
  retries: QueueRetryMetric[];
  sla: QueueSlaIndicator[];
}

export interface ManualSearchRelease {
  id: string;
  searchID: string;
  entityID: string;
  title: string;
  indexer: string;
  protocol: string;
  quality: string;
  publishDate?: string;
  sizeBytes: number;
  seeders: number;
  grabs: number;
  score: number;
  status: string;
  rejectionReason?: string;
  downloadURL: string;
  grabbedAt?: string;
}

export interface ManualSearchResult {
  id: string;
  entityID: string;
  query: string;
  executedAt?: string;
  results: ManualSearchRelease[];
}

export interface ReadingList {
  id: string;
  name: string;
  source: string;
  curator?: string;
  entryCount: number;
  completedCount: number;
  lastSyncedAt?: string;
  tags: string[];
}

export interface MetadataSourceSummary {
  provider: string;
  identifier?: string;
  edition?: string;
  language?: string;
  publishedAt?: string;
  runtimeMinutes?: number;
  quality?: string;
  format?: string;
}

export interface MetadataComparison {
  id: string;
  title: string;
  author?: string;
  ebook?: MetadataSourceSummary | null;
  audiobook?: MetadataSourceSummary | null;
  confidence: number;
  mismatchedFields: string[];
  lastEvaluatedAt?: string;
}

export interface DownloadHistoryEntry {
  id: string;
  entityID?: string;
  title: string;
  format: string;
  status: string;
  sizeBytes?: number;
  sourceList?: string;
  client?: string;
  downloadedAt?: string;
  completedAt?: string;
  failureReason?: string;
}

export interface TriggerJobInput {
  jobType: string;
  targetID: string;
  payloadURI?: string;
  idempotencyKey?: string;
}

export interface WorkerPoolConfig {
  pool: string;
  description?: string;
  stream: string;
  consumerGroup: string;
  heartbeatIntervalSeconds: number;
  staleAfterSeconds: number;
  maxConcurrentJobs?: number;
}

export interface UpdateWorkerPoolInput {
  description?: string;
  stream?: string;
  consumerGroup?: string;
  heartbeatIntervalSeconds?: number;
  staleAfterSeconds?: number;
  maxConcurrentJobs?: number;
}

export interface PrincipalRole {
  name: string;
  description?: string;
  permissions: string[];
}

export interface Principal {
  key: {
    id: string;
    label: string;
    role: string;
    version: number;
    active: boolean;
    createdAt?: string;
    lastRotatedAt?: string;
    lastUsedAt?: string;
    revokedAt?: string;
  };
  role: PrincipalRole;
  permissions: string[];
}

interface RawEntity {
  ID: string;
  EntityType: string;
  Title: string;
  Synopsis: string;
  PendingImports?: number;
  QualityUpgrades?: number;
  Failures?: number;
}

interface RawJob {
  ID: string;
  JobType: string;
  TargetID: string;
  State: string;
  SubmittedAt?: string;
  PayloadURI?: string;
  IdempotencyKey?: string;
  Attempt: number;
  LastAttemptAt?: string;
  CompletedAt?: string;
}

interface RawWorker {
  id: string;
  pool: string;
  capabilities: string[];
  endpoint: string;
  version: string;
  status: string;
  registered_at?: string;
  last_heartbeat_at?: string;
  active_jobs: number;
}

interface RawWorkerPool {
  pool: string;
  description?: string;
  stream: string;
  consumer_group: string;
  heartbeat_interval_seconds: number;
  stale_after_seconds: number;
  max_concurrent_jobs?: number;
}

interface RawQueueStreamLagMetric {
  stream: string;
  consumer_group: string;
  lag_seconds?: number;
  lag_messages?: number;
  oldest_pending_age_seconds?: number;
  last_produced_at?: string;
  last_consumed_at?: string;
  status?: string;
}

interface RawQueueRetryMetric {
  job_type: string;
  retry_count?: number;
  recent_failure_rate?: number;
  max_attempts?: number;
  oldest_retry_age_seconds?: number;
  last_retry_at?: string;
  status?: string;
}

interface RawQueueSlaIndicator {
  queue: string;
  target_seconds?: number;
  p95_seconds?: number;
  p99_seconds?: number;
  success_rate?: number;
  breaches_today?: number;
  status?: string;
  updated_at?: string;
}

interface RawQueueAnalyticsOverview {
  updated_at?: string;
  observed_window_seconds?: number;
  stream_lag?: RawQueueStreamLagMetric[];
  retry_metrics?: RawQueueRetryMetric[];
  sla?: RawQueueSlaIndicator[];
}

interface RawQueueAnalyticsResponse {
  analytics: RawQueueAnalyticsOverview;
}

interface RawManualSearchRelease {
  ID: string;
  SearchID: string;
  EntityID: string;
  Title: string;
  Indexer: string;
  Protocol: string;
  Quality: string;
  PublishDate?: string;
  SizeBytes: number;
  Seeders: number;
  Grabs: number;
  Score: number;
  Status: string;
  RejectionReason?: string;
  DownloadURL: string;
  GrabbedAt?: string;
}

interface RawManualSearchResult {
  ID: string;
  EntityID: string;
  Query: string;
  ExecutedAt?: string;
  Results: RawManualSearchRelease[];
}

interface RawReadingList {
  id: string;
  name: string;
  source: string;
  curator?: string;
  entry_count?: number;
  completed_count?: number;
  last_synced_at?: string;
  tags?: Array<string | null | undefined>;
}

interface RawMetadataSourceSummary {
  provider: string;
  identifier?: string;
  edition?: string;
  language?: string;
  published_at?: string;
  runtime_minutes?: number;
  quality?: string;
  format?: string;
}

interface RawMetadataComparison {
  id: string;
  title: string;
  author?: string;
  ebook?: RawMetadataSourceSummary | null;
  audiobook?: RawMetadataSourceSummary | null;
  confidence?: number;
  mismatched_fields?: Array<string | null | undefined>;
  last_evaluated_at?: string;
}

interface RawDownloadHistoryEntry {
  id: string;
  entity_id?: string;
  title: string;
  format: string;
  status: string;
  size_bytes?: number;
  source_list?: string;
  client?: string;
  downloaded_at?: string;
  completed_at?: string;
  failure_reason?: string;
}

interface RawPrincipal {
  key: {
    id: string;
    label: string;
    role: string;
    version: number;
    active: boolean;
    created_at?: string;
    last_rotated_at?: string;
    last_used_at?: string;
    revoked_at?: string;
  };
  role: {
    name: string;
    description?: string;
    permissions?: string[];
  };
  permissions?: string[];
}

function normalizeTimestamp(value?: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const trimmed = value.trim();
  if (!trimmed || trimmed.startsWith("0001-01-01")) {
    return undefined;
  }
  return trimmed;
}

function mapLibraryEntity(entity: RawEntity): LibraryEntity {
  return {
    id: entity.ID,
    entityType: entity.EntityType,
    title: entity.Title,
    synopsis: entity.Synopsis,
    pendingImports: entity.PendingImports ?? 0,
    qualityUpgrades: entity.QualityUpgrades ?? 0,
    failures: entity.Failures ?? 0,
  };
}

function mapJobSummary(job: RawJob): JobSummary {
  return {
    id: job.ID,
    jobType: job.JobType,
    targetID: job.TargetID,
    state: job.State,
    submittedAt: normalizeTimestamp(job.SubmittedAt),
    payloadURI: job.PayloadURI?.trim() || undefined,
    idempotencyKey: job.IdempotencyKey?.trim() || undefined,
    attempt: job.Attempt ?? 0,
    lastAttemptAt: normalizeTimestamp(job.LastAttemptAt),
    completedAt: normalizeTimestamp(job.CompletedAt),
  };
}

function mapWorkerSummary(worker: RawWorker): WorkerSummary {
  return {
    id: worker.id,
    pool: worker.pool,
    capabilities: worker.capabilities ?? [],
    endpoint: worker.endpoint,
    version: worker.version,
    status: worker.status,
    registeredAt: normalizeTimestamp(worker.registered_at),
    lastHeartbeatAt: normalizeTimestamp(worker.last_heartbeat_at),
    activeJobs: worker.active_jobs,
  };
}

function mapWorkerPool(pool: RawWorkerPool): WorkerPoolConfig {
  return {
    pool: pool.pool,
    description: pool.description?.trim() || undefined,
    stream: pool.stream,
    consumerGroup: pool.consumer_group,
    heartbeatIntervalSeconds: pool.heartbeat_interval_seconds ?? 0,
    staleAfterSeconds: pool.stale_after_seconds ?? 0,
    maxConcurrentJobs: pool.max_concurrent_jobs ?? undefined,
  };
}

function normalizeManualSearchRelease(
  release: RawManualSearchRelease,
): ManualSearchRelease {
  return {
    id: release.ID,
    searchID: release.SearchID,
    entityID: release.EntityID,
    title: release.Title,
    indexer: release.Indexer,
    protocol: release.Protocol,
    quality: release.Quality,
    publishDate: normalizeTimestamp(release.PublishDate),
    sizeBytes: release.SizeBytes ?? 0,
    seeders: release.Seeders ?? 0,
    grabs: release.Grabs ?? 0,
    score: release.Score ?? 0,
    status: release.Status,
    rejectionReason: release.RejectionReason?.trim() || undefined,
    downloadURL: release.DownloadURL,
    grabbedAt: normalizeTimestamp(release.GrabbedAt),
  };
}

function normalizeManualSearchResult(
  search: RawManualSearchResult,
): ManualSearchResult {
  return {
    id: search.ID,
    entityID: search.EntityID,
    query: search.Query?.trim() || "",
    executedAt: normalizeTimestamp(search.ExecutedAt),
    results: (search.Results ?? []).map(normalizeManualSearchRelease),
  };
}

function normalizeStringArray(
  values?: Array<string | null | undefined>,
): string[] {
  return (values ?? [])
    .map((value) => value?.trim())
    .filter((value): value is string => Boolean(value));
}

function normalizeReadingList(list: RawReadingList): ReadingList {
  return {
    id: list.id,
    name: list.name,
    source: list.source,
    curator: list.curator?.trim() || undefined,
    entryCount: list.entry_count ?? 0,
    completedCount: list.completed_count ?? 0,
    lastSyncedAt: normalizeTimestamp(list.last_synced_at),
    tags: normalizeStringArray(list.tags),
  };
}

function normalizeMetadataSource(
  source?: RawMetadataSourceSummary | null,
): MetadataSourceSummary | null {
  if (!source) {
    return null;
  }
  return {
    provider: source.provider,
    identifier: source.identifier?.trim() || undefined,
    edition: source.edition?.trim() || undefined,
    language: source.language?.trim() || undefined,
    publishedAt: normalizeTimestamp(source.published_at),
    runtimeMinutes: source.runtime_minutes ?? undefined,
    quality: source.quality?.trim() || undefined,
    format: source.format?.trim() || undefined,
  };
}

function normalizeMetadataComparison(
  comparison: RawMetadataComparison,
): MetadataComparison {
  const confidence = comparison.confidence ?? 0;
  const normalizedConfidence = Number.isFinite(confidence)
    ? Math.min(Math.max(confidence, 0), 1)
    : 0;
  return {
    id: comparison.id,
    title: comparison.title,
    author: comparison.author?.trim() || undefined,
    ebook: normalizeMetadataSource(comparison.ebook),
    audiobook: normalizeMetadataSource(comparison.audiobook),
    confidence: normalizedConfidence,
    mismatchedFields: normalizeStringArray(comparison.mismatched_fields),
    lastEvaluatedAt: normalizeTimestamp(comparison.last_evaluated_at),
  };
}

function normalizeDownloadHistoryEntry(
  entry: RawDownloadHistoryEntry,
): DownloadHistoryEntry {
  return {
    id: entry.id,
    entityID: entry.entity_id?.trim() || undefined,
    title: entry.title,
    format: entry.format,
    status: entry.status,
    sizeBytes: entry.size_bytes ?? undefined,
    sourceList: entry.source_list?.trim() || undefined,
    client: entry.client?.trim() || undefined,
    downloadedAt: normalizeTimestamp(entry.downloaded_at),
    completedAt: normalizeTimestamp(entry.completed_at),
    failureReason: entry.failure_reason?.trim() || undefined,
  };
}

function normalizePrincipal(payload: RawPrincipal): Principal {
  const key = payload.key ?? {
    id: "",
    label: "",
    role: "",
    version: 0,
    active: false,
  };
  const role = payload.role ?? { name: "", permissions: [] };
  return {
    key: {
      id: key.id,
      label: key.label,
      role: key.role,
      version: key.version ?? 0,
      active: Boolean(key.active),
      createdAt: normalizeTimestamp(key.created_at),
      lastRotatedAt: normalizeTimestamp(key.last_rotated_at),
      lastUsedAt: normalizeTimestamp(key.last_used_at),
      revokedAt: normalizeTimestamp(key.revoked_at),
    },
    role: {
      name: role.name,
      description: role.description?.trim() || undefined,
      permissions: (role.permissions ?? [])
        .map((permission) => permission.trim())
        .filter(Boolean),
    },
    permissions: (payload.permissions ?? [])
      .map((permission) => permission.trim())
      .filter(Boolean),
  };
}

function normalizeQueueStatus(status?: string): QueueHealthStatus {
  switch (status?.toLowerCase()) {
    case "healthy":
    case "ok":
    case "normal":
      return "healthy";
    case "warning":
    case "degraded":
    case "elevated":
      return "warning";
    case "critical":
    case "danger":
    case "failed":
      return "critical";
    default:
      return "warning";
  }
}

function sanitizePositiveNumber(value?: number): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value <= 0) {
    return 0;
  }
  return value;
}

function sanitizeRate(value?: number): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  if (value <= 0) {
    return 0;
  }
  if (value >= 1) {
    return 1;
  }
  return value;
}

function normalizeQueueStreamLag(
  metric: RawQueueStreamLagMetric,
): QueueStreamLagMetric {
  return {
    stream: metric.stream,
    consumerGroup: metric.consumer_group,
    lagSeconds: sanitizePositiveNumber(metric.lag_seconds),
    lagMessages: sanitizePositiveNumber(metric.lag_messages),
    oldestPendingAgeSeconds: sanitizePositiveNumber(
      metric.oldest_pending_age_seconds,
    ),
    lastProducedAt: normalizeTimestamp(metric.last_produced_at),
    lastConsumedAt: normalizeTimestamp(metric.last_consumed_at),
    status: normalizeQueueStatus(metric.status),
  };
}

function normalizeQueueRetryMetric(metric: RawQueueRetryMetric): QueueRetryMetric {
  return {
    jobType: metric.job_type,
    retryCount: sanitizePositiveNumber(metric.retry_count) ?? 0,
    recentFailureRate: sanitizeRate(metric.recent_failure_rate),
    maxAttempts: sanitizePositiveNumber(metric.max_attempts),
    oldestRetryAgeSeconds: sanitizePositiveNumber(
      metric.oldest_retry_age_seconds,
    ),
    lastRetryAt: normalizeTimestamp(metric.last_retry_at),
    status: normalizeQueueStatus(metric.status),
  };
}

function normalizeQueueSlaIndicator(
  indicator: RawQueueSlaIndicator,
): QueueSlaIndicator {
  return {
    queue: indicator.queue,
    targetSeconds: sanitizePositiveNumber(indicator.target_seconds),
    p95Seconds: sanitizePositiveNumber(indicator.p95_seconds),
    p99Seconds: sanitizePositiveNumber(indicator.p99_seconds),
    successRate: sanitizeRate(indicator.success_rate),
    breachesToday: sanitizePositiveNumber(indicator.breaches_today),
    status: normalizeQueueStatus(indicator.status),
    updatedAt: normalizeTimestamp(indicator.updated_at),
  };
}

function normalizeQueueAnalytics(
  analytics?: RawQueueAnalyticsOverview,
): QueueAnalyticsOverview {
  const safeAnalytics = analytics ?? {};
  return {
    updatedAt: normalizeTimestamp(safeAnalytics.updated_at),
    observedWindowSeconds: sanitizePositiveNumber(
      safeAnalytics.observed_window_seconds,
    ),
    streamLag: (safeAnalytics.stream_lag ?? []).map(normalizeQueueStreamLag),
    retries: (safeAnalytics.retry_metrics ?? []).map(normalizeQueueRetryMetric),
    sla: (safeAnalytics.sla ?? []).map(normalizeQueueSlaIndicator),
  };
}

export interface RestApiClientOptions {
  baseUrl?: string;
  tokenProvider?: TokenProvider;
}

export class RestApiClient {
  private readonly options: RestApiClientOptions;

  private readonly baseUrl: string;

  private readonly http: HttpClient;

  constructor(options: RestApiClientOptions = {}) {
    this.options = { ...options };
    this.baseUrl = resolveBaseUrl(options.baseUrl);
    const tokenProvider = options.tokenProvider ?? (() => resolveApiToken());
    this.http = new HttpClient({
      baseUrl: this.baseUrl,
      tokenProvider,
      traceAttributes: { adapter: "rest" },
    });
  }

  getBaseUrl(): string {
    return this.baseUrl;
  }

  withBaseUrl(baseUrl?: string): RestApiClient {
    if (!baseUrl) {
      return this;
    }
    const normalized = resolveBaseUrl(baseUrl);
    if (normalized === this.baseUrl) {
      return this;
    }
    return new RestApiClient({ ...this.options, baseUrl: normalized });
  }

  async fetchEntities(): Promise<LibraryEntity[]> {
    const payload = await this.http.get<{ entities: RawEntity[] }>(
      "/api/v1/library/entities",
      {
        traceName: "REST GET /api/v1/library/entities",
        traceAttributes: { resource: "library.entities" },
      },
    );
    return payload.entities.map(mapLibraryEntity);
  }

  async fetchJobs(): Promise<JobSummary[]> {
    const payload = await this.http.get<{ jobs: RawJob[] }>("/api/v1/jobs", {
      traceName: "REST GET /api/v1/jobs",
      traceAttributes: { resource: "library.jobs" },
    });
    return payload.jobs.map(mapJobSummary);
  }

  async fetchWorkers(): Promise<WorkerSummary[]> {
    const payload = await this.http.get<{ workers: RawWorker[] }>(
      "/api/v1/workers",
      {
        traceName: "REST GET /api/v1/workers",
        traceAttributes: { resource: "worker.status" },
      },
    );
    return payload.workers.map(mapWorkerSummary);
  }

  async fetchQueueAnalytics(): Promise<QueueAnalyticsOverview> {
    const payload = await this.http.get<RawQueueAnalyticsResponse>(
      "/api/v1/queue/analytics",
      {
        traceName: "REST GET /api/v1/queue/analytics",
        traceAttributes: { resource: "queue.analytics" },
      },
    );
    return normalizeQueueAnalytics(payload.analytics);
  }

  async triggerJob(input: TriggerJobInput): Promise<JobSummary> {
    const payload: Record<string, string> = {
      job_type: input.jobType,
      target_id: input.targetID,
    };
    const payloadURI = input.payloadURI?.trim();
    if (payloadURI) {
      payload.payload_uri = payloadURI;
    }
    const idempotencyKey = input.idempotencyKey?.trim();
    if (idempotencyKey) {
      payload.idempotency_key = idempotencyKey;
    }

    const response = await this.http.post<{ job: RawJob }>(
      "/api/v1/jobs",
      payload,
      {
        traceName: "REST POST /api/v1/jobs",
        traceAttributes: {
          resource: "library.jobs",
          jobType: input.jobType,
        },
      },
    );
    return mapJobSummary(response.job);
  }

  async fetchWorkerPools(): Promise<WorkerPoolConfig[]> {
    const payload = await this.http.get<{ pools: RawWorkerPool[] }>(
      "/api/v1/worker-pools",
      {
        traceName: "REST GET /api/v1/worker-pools",
        traceAttributes: { resource: "worker.pools" },
      },
    );
    return payload.pools.map(mapWorkerPool);
  }

  async updateWorkerPool(
    pool: string,
    input: UpdateWorkerPoolInput,
  ): Promise<WorkerPoolConfig> {
    const body: Record<string, unknown> = {};
    if (input.description !== undefined) {
      body.description = input.description;
    }
    if (input.stream !== undefined) {
      body.stream = input.stream;
    }
    if (input.consumerGroup !== undefined) {
      body.consumer_group = input.consumerGroup;
    }
    if (input.heartbeatIntervalSeconds !== undefined) {
      body.heartbeat_interval_seconds = input.heartbeatIntervalSeconds;
    }
    if (input.staleAfterSeconds !== undefined) {
      body.stale_after_seconds = input.staleAfterSeconds;
    }
    if (input.maxConcurrentJobs !== undefined) {
      body.max_concurrent_jobs = input.maxConcurrentJobs;
    }

    const response = await this.http.put<{ pool: RawWorkerPool }>(
      `/api/v1/worker-pools/${encodeURIComponent(pool)}`,
      body,
      {
        traceName: "REST PUT /api/v1/worker-pools",
        traceAttributes: { resource: "worker.pools", pool },
      },
    );
    return mapWorkerPool(response.pool);
  }

  async fetchManualSearch(entityID: string): Promise<ManualSearchResult | null> {
    const payload = await this.http.get<{ search: RawManualSearchResult | null }>(
      `/api/v1/indexer/search?entity_id=${encodeURIComponent(entityID)}`,
      {
        traceName: "REST GET /api/v1/indexer/search",
        traceAttributes: { resource: "indexer.manual_search", entityID },
      },
    );
    return payload.search ? normalizeManualSearchResult(payload.search) : null;
  }

  async runManualSearch(
    entityID: string,
    query: string,
  ): Promise<ManualSearchResult> {
    const response = await this.http.post<{ search: RawManualSearchResult }>(
      "/api/v1/indexer/search",
      { entity_id: entityID, query },
      {
        traceName: "REST POST /api/v1/indexer/search",
        traceAttributes: {
          resource: "indexer.manual_search",
          entityID,
        },
      },
    );
    return normalizeManualSearchResult(response.search);
  }

  async grabManualRelease(
    entityID: string,
    releaseID: string,
  ): Promise<ManualSearchRelease> {
    const response = await this.http.post<{ release: RawManualSearchRelease }>(
      "/api/v1/indexer/releases/grab",
      { entity_id: entityID, release_id: releaseID },
      {
        traceName: "REST POST /api/v1/indexer/releases/grab",
        traceAttributes: {
          resource: "indexer.manual_search",
          entityID,
          releaseID,
        },
      },
    );
    return normalizeManualSearchRelease(response.release);
  }

  async fetchPrincipal(): Promise<Principal> {
    const payload = await this.http.get<{ principal: RawPrincipal }>(
      "/api/v1/auth/principal",
      {
        traceName: "REST GET /api/v1/auth/principal",
        traceAttributes: { resource: "auth.principal" },
      },
    );
    return normalizePrincipal(payload.principal);
  }

  async fetchReadingLists(): Promise<ReadingList[]> {
    const payload = await this.http.get<{ lists: RawReadingList[] }>(
      "/api/v1/library/reading-lists",
      {
        traceName: "REST GET /api/v1/library/reading-lists",
        traceAttributes: { resource: "library.reading_lists" },
      },
    );
    return (payload.lists ?? []).map(normalizeReadingList);
  }

  async fetchMetadataComparisons(): Promise<MetadataComparison[]> {
    const payload = await this.http.get<{ comparisons: RawMetadataComparison[] }>(
      "/api/v1/library/metadata/comparisons",
      {
        traceName: "REST GET /api/v1/library/metadata/comparisons",
        traceAttributes: { resource: "library.metadata_comparisons" },
      },
    );
    return (payload.comparisons ?? []).map(normalizeMetadataComparison);
  }

  async fetchDownloadHistory(): Promise<DownloadHistoryEntry[]> {
    const payload = await this.http.get<{ history: RawDownloadHistoryEntry[] }>(
      "/api/v1/library/download-history",
      {
        traceName: "REST GET /api/v1/library/download-history",
        traceAttributes: { resource: "library.download_history" },
      },
    );
    return (payload.history ?? []).map(normalizeDownloadHistoryEntry);
  }
}

const defaultRestClient = new RestApiClient();

export function createRestApiClient(options?: RestApiClientOptions): RestApiClient {
  return new RestApiClient(options);
}

export function fetchEntities(baseUrl?: string): Promise<LibraryEntity[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchEntities();
}

export function fetchJobs(baseUrl?: string): Promise<JobSummary[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchJobs();
}

export function fetchWorkers(baseUrl?: string): Promise<WorkerSummary[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchWorkers();
}

export function fetchQueueAnalytics(
  baseUrl?: string,
): Promise<QueueAnalyticsOverview> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchQueueAnalytics();
}

export function triggerJob(
  input: TriggerJobInput,
  baseUrl?: string,
): Promise<JobSummary> {
  return defaultRestClient.withBaseUrl(baseUrl).triggerJob(input);
}

export function fetchWorkerPools(baseUrl?: string): Promise<WorkerPoolConfig[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchWorkerPools();
}

export function updateWorkerPool(
  pool: string,
  input: UpdateWorkerPoolInput,
  baseUrl?: string,
): Promise<WorkerPoolConfig> {
  return defaultRestClient.withBaseUrl(baseUrl).updateWorkerPool(pool, input);
}

export function fetchManualSearch(
  entityID: string,
  baseUrl?: string,
): Promise<ManualSearchResult | null> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchManualSearch(entityID);
}

export function runManualSearch(
  entityID: string,
  query: string,
  baseUrl?: string,
): Promise<ManualSearchResult> {
  return defaultRestClient.withBaseUrl(baseUrl).runManualSearch(entityID, query);
}

export function grabManualRelease(
  entityID: string,
  releaseID: string,
  baseUrl?: string,
): Promise<ManualSearchRelease> {
  return defaultRestClient
    .withBaseUrl(baseUrl)
    .grabManualRelease(entityID, releaseID);
}

export function fetchPrincipal(baseUrl?: string): Promise<Principal> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchPrincipal();
}

export function fetchReadingLists(baseUrl?: string): Promise<ReadingList[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchReadingLists();
}

export function fetchMetadataComparisons(
  baseUrl?: string,
): Promise<MetadataComparison[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchMetadataComparisons();
}

export function fetchDownloadHistory(
  baseUrl?: string,
): Promise<DownloadHistoryEntry[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchDownloadHistory();
}

export interface GrpcLibraryClientOptions extends RestApiClientOptions {}

export class GrpcLibraryClient {
  private readonly options: GrpcLibraryClientOptions;

  private readonly restClient: RestApiClient;

  constructor(options: GrpcLibraryClientOptions = {}) {
    this.options = { ...options };
    this.restClient = new RestApiClient(options);
  }

  getBaseUrl(): string {
    return this.restClient.getBaseUrl();
  }

  withBaseUrl(baseUrl?: string): GrpcLibraryClient {
    if (!baseUrl) {
      return this;
    }
    const normalized = resolveBaseUrl(baseUrl);
    if (normalized === this.getBaseUrl()) {
      return this;
    }
    return new GrpcLibraryClient({ ...this.options, baseUrl: normalized });
  }

  private async tracedCall<T>(
    method: string,
    executor: () => Promise<T>,
    successAttributes?: (result: T) => TraceAttributes,
  ): Promise<T> {
    const trace = startClientTrace(`gRPC ${method}`, {
      adapter: "grpc",
      transport: "grpc",
      service: "arrgo.api.v1.LibraryService",
      method,
      baseUrl: this.getBaseUrl(),
    });
    try {
      const result = await executor();
      const extra = successAttributes ? successAttributes(result) : undefined;
      finishTrace(trace, "success", extra);
      return result;
    } catch (error) {
      finishTrace(trace, "error", undefined, error);
      throw error;
    }
  }

  listLibraryEntities(): Promise<LibraryEntity[]> {
    return this.tracedCall(
      "LibraryService/ListLibraryEntities",
      () => this.restClient.fetchEntities(),
      (entities) => ({ resultCount: entities.length }),
    );
  }

  listJobs(): Promise<JobSummary[]> {
    return this.tracedCall(
      "LibraryService/ListJobs",
      () => this.restClient.fetchJobs(),
      (jobs) => ({ resultCount: jobs.length }),
    );
  }

  triggerJob(input: TriggerJobInput): Promise<JobSummary> {
    return this.tracedCall(
      "LibraryService/TriggerJob",
      () => this.restClient.triggerJob(input),
      (job) => ({ jobType: job.jobType }),
    );
  }
}

const defaultGrpcClient = new GrpcLibraryClient();

export function createGrpcLibraryClient(
  options?: GrpcLibraryClientOptions,
): GrpcLibraryClient {
  return new GrpcLibraryClient(options);
}

export { defaultRestClient as restClient, defaultGrpcClient as grpcLibraryClient };
