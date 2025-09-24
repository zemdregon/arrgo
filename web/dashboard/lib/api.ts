const FALLBACK_BASE_URL = "http://localhost:8080";

export interface LibraryEntity {
  id: string;
  entityType: string;
  title: string;
  synopsis: string;
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

interface RawEntity {
  ID: string;
  EntityType: string;
  Title: string;
  Synopsis: string;
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

function getBaseUrl(): string {
  const base = process.env.NEXT_PUBLIC_ARRGO_API_URL ?? FALLBACK_BASE_URL;
  return base.replace(/\/$/, "");
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

async function requestJSON<T>(path: string, init: RequestInit): Promise<T> {
  const url = `${getBaseUrl()}${path}`;
  const headers = new Headers(init.headers ?? {});
  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      headers,
      cache: "no-store",
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : "unknown error";
    throw new Error(`Request to ${url} failed: ${message}`);
  }

  if (!response.ok) {
    let message = `Request to ${url} failed with status ${response.status}`;
    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      try {
        const payload = (await response.json()) as { error?: string };
        const errorMessage = payload?.error?.trim();
        if (errorMessage) {
          message = errorMessage;
        }
      } catch (parseError) {
        const errorMessage =
          parseError instanceof Error ? parseError.message : String(parseError);
        message = `${message}: ${errorMessage}`;
      }
    } else {
      const body = await response.text();
      if (body) {
        message = `${message}: ${body}`;
      }
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

async function fetchJSON<T>(path: string): Promise<T> {
  return requestJSON<T>(path, { method: "GET" });
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json" });
  return requestJSON<T>(path, {
    method: "POST",
    headers,
    body: JSON.stringify(body ?? {}),
  });
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json" });
  return requestJSON<T>(path, {
    method: "PUT",
    headers,
    body: JSON.stringify(body ?? {}),
  });
}

export async function fetchEntities(): Promise<LibraryEntity[]> {
  const payload = await fetchJSON<{ entities: RawEntity[] }>(
    "/api/v1/library/entities",
  );
  return payload.entities.map((entity) => ({
    id: entity.ID,
    entityType: entity.EntityType,
    title: entity.Title,
    synopsis: entity.Synopsis,
  }));
}

export async function fetchJobs(): Promise<JobSummary[]> {
  const payload = await fetchJSON<{ jobs: RawJob[] }>("/api/v1/jobs");
  return payload.jobs.map((job) => ({
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
  }));
}

export async function fetchWorkers(): Promise<WorkerSummary[]> {
  const payload = await fetchJSON<{ workers: RawWorker[] }>("/api/v1/workers");
  return payload.workers.map((worker) => ({
    id: worker.id,
    pool: worker.pool,
    capabilities: worker.capabilities ?? [],
    endpoint: worker.endpoint,
    version: worker.version,
    status: worker.status,
    registeredAt: normalizeTimestamp(worker.registered_at),
    lastHeartbeatAt: normalizeTimestamp(worker.last_heartbeat_at),
    activeJobs: worker.active_jobs,
  }));
}

export async function triggerJob(input: TriggerJobInput): Promise<JobSummary> {
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

  const response = await postJSON<{ job: RawJob }>("/api/v1/jobs", payload);
  const job = response.job;
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

export async function fetchWorkerPools(): Promise<WorkerPoolConfig[]> {
  const payload = await fetchJSON<{ pools: RawWorkerPool[] }>(
    "/api/v1/worker-pools",
  );
  return payload.pools.map((pool) => ({
    pool: pool.pool,
    description: pool.description?.trim() || undefined,
    stream: pool.stream,
    consumerGroup: pool.consumer_group,
    heartbeatIntervalSeconds: pool.heartbeat_interval_seconds ?? 0,
    staleAfterSeconds: pool.stale_after_seconds ?? 0,
    maxConcurrentJobs: pool.max_concurrent_jobs ?? undefined,
  }));
}

export async function updateWorkerPool(
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

  const response = await putJSON<{ pool: RawWorkerPool }>(
    `/api/v1/worker-pools/${encodeURIComponent(pool)}`,
    body,
  );
  const cfg = response.pool;
  return {
    pool: cfg.pool,
    description: cfg.description?.trim() || undefined,
    stream: cfg.stream,
    consumerGroup: cfg.consumer_group,
    heartbeatIntervalSeconds: cfg.heartbeat_interval_seconds ?? 0,
    staleAfterSeconds: cfg.stale_after_seconds ?? 0,
    maxConcurrentJobs: cfg.max_concurrent_jobs ?? undefined,
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

export async function fetchManualSearch(
  entityID: string,
): Promise<ManualSearchResult | null> {
  const payload = await fetchJSON<{ search: RawManualSearchResult | null }>(
    `/api/v1/indexer/search?entity_id=${encodeURIComponent(entityID)}`,
  );
  return payload.search ? normalizeManualSearchResult(payload.search) : null;
}

export async function runManualSearch(
  entityID: string,
  query: string,
): Promise<ManualSearchResult> {
  const response = await postJSON<{ search: RawManualSearchResult }>(
    "/api/v1/indexer/search",
    { entity_id: entityID, query },
  );
  return normalizeManualSearchResult(response.search);
}

export async function grabManualRelease(
  entityID: string,
  releaseID: string,
): Promise<ManualSearchRelease> {
  const response = await postJSON<{ release: RawManualSearchRelease }>(
    "/api/v1/indexer/releases/grab",
    { entity_id: entityID, release_id: releaseID },
  );
  return normalizeManualSearchRelease(response.release);
}
