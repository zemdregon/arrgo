import type {
  JobSummary,
  LibraryEntity,
  WorkerSummary,
} from "../../../lib/api";

export type SelectOption = {
  value: string;
  label: string;
};

export type EntityOption = {
  value: string;
  label: string;
  entityType: string;
};

export type BreakdownEntry = [string, number];

export type LibraryStats = {
  total: number;
  breakdown: BreakdownEntry[];
};

export type JobStats = {
  total: number;
  counts: BreakdownEntry[];
};

export type WorkerStats = {
  total: number;
  healthy: number;
  stale: number;
  pools: BreakdownEntry[];
};

export type QueueAnalytics = {
  total: number;
  byType: BreakdownEntry[];
  byState: BreakdownEntry[];
  averageAttempts: number;
  latestSubmittedAt?: string;
};

export function normalize(value?: string, fallback = "unknown"): string {
  const trimmed = value?.trim();
  if (!trimmed) {
    return fallback;
  }
  return trimmed.toLowerCase();
}

export function buildOptions(
  values: Array<string | undefined>,
  fallbackLabel = "Unknown",
  fallbackValue = "unknown",
): SelectOption[] {
  const map = new Map<string, string>();
  for (const raw of values) {
    const trimmed = raw?.trim();
    if (!trimmed) {
      if (!map.has(fallbackValue)) {
        map.set(fallbackValue, fallbackLabel);
      }
      continue;
    }
    const normalized = trimmed.toLowerCase();
    if (!map.has(normalized)) {
      map.set(normalized, trimmed);
    }
  }
  return Array.from(map.entries())
    .sort((a, b) => a[1].localeCompare(b[1]))
    .map(([value, label]) => ({ value, label }));
}

export function optionLabel(
  options: SelectOption[],
  value: string,
  fallback: string,
): string {
  const match = options.find((option) => option.value === value);
  return match ? match.label : fallback;
}

export function createEntityOptions(
  entities?: LibraryEntity[],
): EntityOption[] {
  return (entities ?? [])
    .map<EntityOption>((entity) => ({
      value: entity.id,
      label: entity.title || entity.id,
      entityType: entity.entityType || "Unknown",
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

export function buildLibraryStats(data?: LibraryEntity[]): LibraryStats {
  const entities = data ?? [];
  const counts = new Map<string, number>();
  for (const entity of entities) {
    const key = entity.entityType || "unknown";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return {
    total: entities.length,
    breakdown: Array.from(counts.entries()).sort((a, b) =>
      a[0].localeCompare(b[0]),
    ),
  };
}

export function buildJobStats(data?: JobSummary[]): JobStats {
  const jobs = data ?? [];
  const counts = new Map<string, number>();
  for (const job of jobs) {
    const key = job.state || "unknown";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return { total: jobs.length, counts: Array.from(counts.entries()) };
}

export function buildWorkerStats(data?: WorkerSummary[]): WorkerStats {
  const workers = data ?? [];
  let healthy = 0;
  let stale = 0;
  const pools = new Map<string, number>();
  for (const worker of workers) {
    if (worker.status.toLowerCase() === "healthy") {
      healthy += 1;
    } else if (worker.status.toLowerCase() === "stale") {
      stale += 1;
    }
    pools.set(worker.pool, (pools.get(worker.pool) ?? 0) + 1);
  }
  return {
    total: workers.length,
    healthy,
    stale,
    pools: Array.from(pools.entries()).sort((a, b) =>
      a[0].localeCompare(b[0]),
    ),
  };
}

export function buildQueueAnalytics(data?: JobSummary[]): QueueAnalytics {
  const jobs = data ?? [];
  const typeCounts = new Map<string, number>();
  const stateCounts = new Map<string, number>();
  let totalAttempts = 0;
  let latestSubmission: string | undefined;

  for (const job of jobs) {
    const typeKey = job.jobType || "unknown";
    typeCounts.set(typeKey, (typeCounts.get(typeKey) ?? 0) + 1);

    const stateKey = job.state || "unknown";
    stateCounts.set(stateKey, (stateCounts.get(stateKey) ?? 0) + 1);

    totalAttempts += job.attempt ?? 0;

    if (job.submittedAt) {
      if (!latestSubmission || job.submittedAt > latestSubmission) {
        latestSubmission = job.submittedAt;
      }
    }
  }

  return {
    total: jobs.length,
    byType: Array.from(typeCounts.entries()).sort((a, b) =>
      a[0].localeCompare(b[0]),
    ),
    byState: Array.from(stateCounts.entries()).sort((a, b) =>
      a[0].localeCompare(b[0]),
    ),
    averageAttempts: jobs.length ? totalAttempts / jobs.length : 0,
    latestSubmittedAt: latestSubmission,
  };
}

export function formatShowingSummary(
  visibleCount: number,
  limit: number,
  noun: string,
  totalCount: number,
): string {
  return (
    "Showing " +
    Math.min(visibleCount, limit) +
    " of " +
    visibleCount +
    " " +
    noun +
    " (" +
    totalCount +
    " total)"
  );
}
