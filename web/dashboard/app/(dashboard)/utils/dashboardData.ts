import type {
  JobSummary,
  LibraryEntity,
  WorkerPoolConfig,
  WorkerSummary,
} from "../../lib/api";

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

export type WorkerHeartbeatEvent = {
  workerId: string;
  status: string;
  version?: string;
  lastHeartbeatAt?: string;
};

export type WorkerPoolInsight = {
  pool: string;
  description?: string;
  workerCount: number;
  healthyCount: number;
  staleCount: number;
  unknownCount: number;
  status: "healthy" | "warning" | "critical";
  capabilities: string[];
  lastHeartbeatAt?: string;
  heartbeatHistory: WorkerHeartbeatEvent[];
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

function derivePoolStatus(
  workerCount: number,
  healthyCount: number,
  staleCount: number,
  unknownCount: number,
): "healthy" | "warning" | "critical" {
  if (workerCount === 0) {
    return "critical";
  }
  if (healthyCount === 0) {
    return "critical";
  }
  const staleRatio = workerCount ? staleCount / workerCount : 0;
  if (staleRatio >= 0.5) {
    return "critical";
  }
  if (staleCount > 0 || unknownCount > 0) {
    return "warning";
  }
  return "healthy";
}

export function buildWorkerPoolInsights(
  pools?: WorkerPoolConfig[],
  workers?: WorkerSummary[],
): WorkerPoolInsight[] {
  const poolConfigs = new Map<string, WorkerPoolConfig>();
  for (const pool of pools ?? []) {
    poolConfigs.set(pool.pool, pool);
  }

  const workersByPool = new Map<string, WorkerSummary[]>();
  for (const worker of workers ?? []) {
    const poolName = worker.pool || "unassigned";
    const list = workersByPool.get(poolName) ?? [];
    list.push(worker);
    workersByPool.set(poolName, list);
  }

  const poolNames = new Set<string>();
  for (const pool of poolConfigs.keys()) {
    poolNames.add(pool);
  }
  for (const pool of workersByPool.keys()) {
    poolNames.add(pool);
  }

  return Array.from(poolNames)
    .sort((a, b) => a.localeCompare(b))
    .map<WorkerPoolInsight>((poolName) => {
      const config = poolConfigs.get(poolName);
      const poolWorkers = workersByPool.get(poolName) ?? [];
      let healthyCount = 0;
      let staleCount = 0;
      let unknownCount = 0;
      const capabilitySet = new Set<string>();
      let latestHeartbeat: string | undefined;

      for (const worker of poolWorkers) {
        const normalizedStatus = worker.status?.toLowerCase() || "unknown";
        if (normalizedStatus === "healthy") {
          healthyCount += 1;
        } else if (normalizedStatus === "stale") {
          staleCount += 1;
        } else {
          unknownCount += 1;
        }
        for (const capability of worker.capabilities ?? []) {
          const trimmed = capability.trim();
          if (trimmed) {
            capabilitySet.add(trimmed);
          }
        }
        if (
          worker.lastHeartbeatAt &&
          (!latestHeartbeat || worker.lastHeartbeatAt > latestHeartbeat)
        ) {
          latestHeartbeat = worker.lastHeartbeatAt;
        }
      }

      const heartbeatHistory = poolWorkers
        .map<WorkerHeartbeatEvent>((worker) => ({
          workerId: worker.id,
          status: worker.status,
          version: worker.version,
          lastHeartbeatAt: worker.lastHeartbeatAt,
        }))
        .sort((a, b) => {
          const left = a.lastHeartbeatAt ?? "";
          const right = b.lastHeartbeatAt ?? "";
          if (left === right) {
            return a.workerId.localeCompare(b.workerId);
          }
          return right.localeCompare(left);
        });

      const workerCount = poolWorkers.length;
      const status = derivePoolStatus(
        workerCount,
        healthyCount,
        staleCount,
        unknownCount,
      );

      return {
        pool: poolName,
        description: config?.description,
        workerCount,
        healthyCount,
        staleCount,
        unknownCount,
        status,
        capabilities: Array.from(capabilitySet).sort((a, b) =>
          a.localeCompare(b),
        ),
        lastHeartbeatAt: latestHeartbeat,
        heartbeatHistory,
      };
    });
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
