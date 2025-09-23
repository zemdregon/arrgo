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

async function fetchJSON<T>(path: string): Promise<T> {
  const url = `${getBaseUrl()}${path}`;
  let response: Response;
  try {
    response = await fetch(url, {
      headers: {
        Accept: "application/json"
      },
      cache: "no-store"
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : "unknown error";
    throw new Error(`Request to ${url} failed: ${message}`);
  }

  if (!response.ok) {
    const body = await response.text();
    const suffix = body ? `: ${body}` : "";
    throw new Error(`Request to ${url} failed with status ${response.status}${suffix}`);
  }

  return (await response.json()) as T;
}

export async function fetchEntities(): Promise<LibraryEntity[]> {
  const payload = await fetchJSON<{ entities: RawEntity[] }>(
    "/api/v1/library/entities"
  );
  return payload.entities.map((entity) => ({
    id: entity.ID,
    entityType: entity.EntityType,
    title: entity.Title,
    synopsis: entity.Synopsis
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
    completedAt: normalizeTimestamp(job.CompletedAt)
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
    activeJobs: worker.active_jobs
  }));
}
