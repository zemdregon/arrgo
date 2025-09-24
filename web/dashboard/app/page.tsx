"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { DataPanel } from "../components/DataPanel";
import { StatusPill } from "../components/StatusPill";
import { useApiData } from "../hooks/useApiData";
import {
  fetchEntities,
  fetchJobs,
  fetchWorkerPools,
  fetchWorkers,
  JobSummary,
  LibraryEntity,
  triggerJob,
  updateWorkerPool,
  WorkerPoolConfig,
  WorkerSummary
} from "../lib/api";
import styles from "./page.module.css";

type Tone = "neutral" | "success" | "warning" | "danger";

function formatTimestamp(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(date);
}

function jobTone(state: string): Tone {
  switch (state?.toLowerCase()) {
    case "completed":
      return "success";
    case "running":
      return "warning";
    case "queued":
      return "neutral";
    default:
      return "danger";
  }
}

function workerTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "healthy":
      return "success";
    case "stale":
      return "warning";
    default:
      return "danger";
  }
}

function shortenIdentifier(value: string, size = 12): string {
  if (!value) {
    return "—";
  }
  if (value.length <= size) {
    return value;
  }
  return `${value.slice(0, size)}…`;
}

type BreakdownEntry = [string, number];

type LibraryStats = {
  total: number;
  breakdown: BreakdownEntry[];
};

type JobStats = {
  total: number;
  counts: BreakdownEntry[];
};

type WorkerStats = {
  total: number;
  healthy: number;
  stale: number;
  pools: BreakdownEntry[];
};

type QueueAnalytics = {
  total: number;
  byType: BreakdownEntry[];
  byState: BreakdownEntry[];
  averageAttempts: number;
  latestSubmittedAt?: string;
};

type SelectOption = {
  value: string;
  label: string;
};

type EntityOption = {
  value: string;
  label: string;
  entityType: string;
};

const JOB_TYPE_OPTIONS: SelectOption[] = [
  { value: "scan", label: "Scan" },
  { value: "import", label: "Import" }
];

function normalize(value?: string, fallback = "unknown"): string {
  const trimmed = value?.trim();
  if (!trimmed) {
    return fallback;
  }
  return trimmed.toLowerCase();
}

function buildOptions(
  values: Array<string | undefined>,
  fallbackLabel = "Unknown",
  fallbackValue = "unknown"
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

function optionLabel(
  options: SelectOption[],
  value: string,
  fallback: string
): string {
  const match = options.find((option) => option.value === value);
  return match ? match.label : fallback;
}

function buildLibraryStats(data?: LibraryEntity[]): LibraryStats {
  const entities = data ?? [];
  const counts = new Map<string, number>();
  for (const entity of entities) {
    const key = entity.entityType || "unknown";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return {
    total: entities.length,
    breakdown: Array.from(counts.entries()).sort((a, b) =>
      a[0].localeCompare(b[0])
    )
  };
}

function buildJobStats(data?: JobSummary[]): JobStats {
  const jobs = data ?? [];
  const counts = new Map<string, number>();
  for (const job of jobs) {
    const key = job.state || "unknown";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return { total: jobs.length, counts: Array.from(counts.entries()) };
}

function buildWorkerStats(data?: WorkerSummary[]): WorkerStats {
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
    pools: Array.from(pools.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  };
}

function buildQueueAnalytics(data?: JobSummary[]): QueueAnalytics {
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
    byType: Array.from(typeCounts.entries()).sort((a, b) => a[0].localeCompare(b[0])),
    byState: Array.from(stateCounts.entries()).sort((a, b) => a[0].localeCompare(b[0])),
    averageAttempts: jobs.length ? totalAttempts / jobs.length : 0,
    latestSubmittedAt: latestSubmission
  };
}

function formatRelativeTime(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const diff = Date.now() - date.getTime();
  if (diff < 0) {
    return formatTimestamp(value);
  }
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) {
    return `${seconds}s ago`;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    return `${hours}h ago`;
  }
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function DashboardPage() {
  const library = useApiData(fetchEntities);
  const jobs = useApiData(fetchJobs);
  const workers = useApiData(fetchWorkers);
  const pools = useApiData(fetchWorkerPools);

  const [libraryTypeFilter, setLibraryTypeFilter] = useState("all");
  const [jobStateFilter, setJobStateFilter] = useState("all");
  const [workerPoolFilter, setWorkerPoolFilter] = useState("all");
  const [workerStatusFilter, setWorkerStatusFilter] = useState("all");
  const [jobTypeSelection, setJobTypeSelection] = useState(
    JOB_TYPE_OPTIONS[0]?.value ?? "scan"
  );
  const [jobTargetSelection, setJobTargetSelection] = useState("");
  const [jobPayloadURI, setJobPayloadURI] = useState("");
  const [jobIdempotencyKey, setJobIdempotencyKey] = useState("");
  const [jobSubmitting, setJobSubmitting] = useState(false);
  const [jobFeedback, setJobFeedback] = useState<
    { kind: "success" | "error"; message: string } | null
  >(null);
  const [selectedJob, setSelectedJob] = useState<JobSummary | null>(null);
  const [poolSelection, setPoolSelection] = useState("");
  const [poolForm, setPoolForm] = useState({
    description: "",
    stream: "",
    consumerGroup: "",
    heartbeatIntervalSeconds: "",
    staleAfterSeconds: "",
    maxConcurrentJobs: ""
  });
  const [poolSubmitting, setPoolSubmitting] = useState(false);
  const [poolFeedback, setPoolFeedback] = useState<
    { kind: "success" | "error"; message: string } | null
  >(null);

  const libraryStats = useMemo(() => buildLibraryStats(library.data), [
    library.data
  ]);
  const jobStats = useMemo(() => buildJobStats(jobs.data), [jobs.data]);
  const workerStats = useMemo(() => buildWorkerStats(workers.data), [
    workers.data
  ]);
  const queueAnalytics = useMemo(() => buildQueueAnalytics(jobs.data), [
    jobs.data
  ]);
  const libraryEntityOptions = useMemo(
    () =>
      (library.data ?? [])
        .map<EntityOption>((entity) => ({
          value: entity.id,
          label: entity.title || entity.id,
          entityType: entity.entityType || "Unknown"
        }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [library.data]
  );

  const libraryTypeOptions = useMemo(
    () =>
      buildOptions(
        (library.data ?? []).map((entity) => entity.entityType),
        "Unknown",
        "unknown"
      ),
    [library.data]
  );
  const jobStateOptions = useMemo(
    () =>
      buildOptions(
        (jobs.data ?? []).map((job) => job.state),
        "Unknown state",
        "unknown"
      ),
    [jobs.data]
  );
  const workerPoolOptions = useMemo(
    () =>
      buildOptions(
        (workers.data ?? []).map((worker) => worker.pool),
        "Unassigned",
        "unassigned"
      ),
    [workers.data]
  );
  const workerStatusOptions = useMemo(
    () =>
      buildOptions(
        (workers.data ?? []).map((worker) => worker.status),
        "Unknown status",
        "unknown"
      ),
    [workers.data]
  );
  const poolOptions = useMemo(
    () =>
      (pools.data ?? []).map<SelectOption>((pool) => ({
        value: pool.pool,
        label: pool.description
          ? pool.pool + " · " + pool.description
          : pool.pool
      })),
    [pools.data]
  );
  const selectedPoolConfig = useMemo<WorkerPoolConfig | null>(() => {
    if (!pools.data) {
      return null;
    }
    return pools.data.find((cfg) => cfg.pool === poolSelection) ?? null;
  }, [pools.data, poolSelection]);
  const selectedPoolWorkerCount = useMemo(() => {
    const match = workerStats.pools.find(([pool]) => pool === poolSelection);
    return match ? match[1] : 0;
  }, [workerStats.pools, poolSelection]);

  useEffect(() => {
    if (
      libraryTypeFilter !== "all" &&
      !libraryTypeOptions.some((option) => option.value === libraryTypeFilter)
    ) {
      setLibraryTypeFilter("all");
    }
  }, [libraryTypeFilter, libraryTypeOptions]);

  useEffect(() => {
    if (
      jobStateFilter !== "all" &&
      !jobStateOptions.some((option) => option.value === jobStateFilter)
    ) {
      setJobStateFilter("all");
    }
  }, [jobStateFilter, jobStateOptions]);

  useEffect(() => {
    if (
      workerPoolFilter !== "all" &&
      !workerPoolOptions.some((option) => option.value === workerPoolFilter)
    ) {
      setWorkerPoolFilter("all");
    }
  }, [workerPoolFilter, workerPoolOptions]);

  useEffect(() => {
    if (
      workerStatusFilter !== "all" &&
      !workerStatusOptions.some((option) => option.value === workerStatusFilter)
    ) {
      setWorkerStatusFilter("all");
    }
  }, [workerStatusFilter, workerStatusOptions]);

  useEffect(() => {
    if (!libraryEntityOptions.length) {
      setJobTargetSelection("");
      return;
    }
    if (
      jobTargetSelection &&
      libraryEntityOptions.some((option) => option.value === jobTargetSelection)
    ) {
      return;
    }
    setJobTargetSelection(libraryEntityOptions[0].value);
  }, [jobTargetSelection, libraryEntityOptions]);

  useEffect(() => {
    if (!pools.data || pools.data.length === 0) {
      if (poolSelection !== "") {
        setPoolSelection("");
      }
      return;
    }
    if (poolSelection && pools.data.some((cfg) => cfg.pool === poolSelection)) {
      return;
    }
    setPoolSelection(pools.data[0].pool);
  }, [pools.data, poolSelection]);

  useEffect(() => {
    if (!selectedPoolConfig) {
      setPoolForm({
        description: "",
        stream: "",
        consumerGroup: "",
        heartbeatIntervalSeconds: "",
        staleAfterSeconds: "",
        maxConcurrentJobs: ""
      });
      return;
    }
    setPoolForm({
      description: selectedPoolConfig.description ?? "",
      stream: selectedPoolConfig.stream,
      consumerGroup: selectedPoolConfig.consumerGroup,
      heartbeatIntervalSeconds: String(
        selectedPoolConfig.heartbeatIntervalSeconds || 0
      ),
      staleAfterSeconds: String(selectedPoolConfig.staleAfterSeconds || 0),
      maxConcurrentJobs:
        selectedPoolConfig.maxConcurrentJobs !== undefined
          ? String(selectedPoolConfig.maxConcurrentJobs)
          : ""
    });
    setPoolFeedback(null);
  }, [selectedPoolConfig]);

  useEffect(() => {
    if (selectedJob && !jobs.data?.some((job) => job.id === selectedJob.id)) {
      setSelectedJob(null);
    }
  }, [jobs.data, selectedJob]);

  const visibleLibrary = useMemo(() => {
    const entities = library.data ?? [];
    if (libraryTypeFilter === "all") {
      return entities;
    }
    return entities.filter(
      (entity) => normalize(entity.entityType) === libraryTypeFilter
    );
  }, [library.data, libraryTypeFilter]);

  const visibleJobs = useMemo(() => {
    const items = jobs.data ?? [];
    if (jobStateFilter === "all") {
      return items;
    }
    return items.filter((job) => normalize(job.state) === jobStateFilter);
  }, [jobs.data, jobStateFilter]);

  const visibleWorkers = useMemo(() => {
    const nodes = workers.data ?? [];
    return nodes.filter((worker) => {
      if (
        workerPoolFilter !== "all" &&
        normalize(worker.pool, "unassigned") !== workerPoolFilter
      ) {
        return false;
      }
      if (
        workerStatusFilter !== "all" &&
        normalize(worker.status) !== workerStatusFilter
      ) {
        return false;
      }
      return true;
    });
  }, [workers.data, workerPoolFilter, workerStatusFilter]);

  const jobSubmitDisabled =
    !libraryEntityOptions.length || !jobTargetSelection || jobSubmitting;

  const formatShowingSummary = (
    visibleCount: number,
    limit: number,
    noun: string,
    totalCount: number
  ): string => {
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
  };
  const libraryFooter = libraryStats.total
    ? formatShowingSummary(
        visibleLibrary.length,
        5,
        libraryTypeFilter === "all" ? "entities" : "matches",
        libraryStats.total
      )
    : undefined;
  const jobFooter = jobStats.total
    ? formatShowingSummary(
        visibleJobs.length,
        8,
        jobStateFilter === "all" ? "jobs" : "matches",
        jobStats.total
      )
    : undefined;
  const workerFooter = workerStats.total
    ? formatShowingSummary(
        visibleWorkers.length,
        8,
        workerPoolFilter === "all" && workerStatusFilter === "all"
          ? "workers"
          : "matches",
        workerStats.total
      )
    : undefined;

  const parseOptionalNumber = (value: string): number | undefined => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    const parsed = Number(trimmed);
    if (!Number.isFinite(parsed)) {
      return undefined;
    }
    return parsed;
  };

  const handleTriggerJob = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!jobTargetSelection) {
      setJobFeedback({
        kind: "error",
        message: "Select a target entity before queueing a job."
      });
      return;
    }

    const jobTypeValue = jobTypeSelection;
    const jobTargetValue = jobTargetSelection;
    const jobTargetOption = libraryEntityOptions.find(
      (option) => option.value === jobTargetValue
    );
    const jobTargetDescription = jobTargetOption
      ? jobTargetOption.label + " · " + jobTargetOption.entityType
      : jobTargetValue;
    const jobTypeLabel =
      JOB_TYPE_OPTIONS.find((option) => option.value === jobTypeValue)?.label ??
      jobTypeValue;

    setJobSubmitting(true);
    setJobFeedback(null);
    try {
      await triggerJob({
        jobType: jobTypeValue,
        targetID: jobTargetValue,
        payloadURI: jobPayloadURI,
        idempotencyKey: jobIdempotencyKey
      });
      setJobFeedback({
        kind: "success",
        message:
          "Queued " +
          jobTypeLabel +
          " job for " +
          jobTargetDescription +
          "."
      });
      setJobPayloadURI("");
      setJobIdempotencyKey("");
      jobs.refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setJobFeedback({ kind: "error", message });
    } finally {
      setJobSubmitting(false);
    }
  };

  const handlePoolSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!poolSelection) {
      setPoolFeedback({
        kind: "error",
        message: "Select a worker pool to update before saving changes."
      });
      return;
    }
    if (!poolForm.stream.trim() || !poolForm.consumerGroup.trim()) {
      setPoolFeedback({
        kind: "error",
        message: "Stream and consumer group values are required."
      });
      return;
    }

    const heartbeatInterval = parseOptionalNumber(
      poolForm.heartbeatIntervalSeconds
    );
    const staleAfter = parseOptionalNumber(poolForm.staleAfterSeconds);
    const maxConcurrentJobs = parseOptionalNumber(poolForm.maxConcurrentJobs);

    setPoolSubmitting(true);
    setPoolFeedback(null);
    try {
      await updateWorkerPool(poolSelection, {
        description: poolForm.description,
        stream: poolForm.stream,
        consumerGroup: poolForm.consumerGroup,
        heartbeatIntervalSeconds: heartbeatInterval,
        staleAfterSeconds: staleAfter,
        maxConcurrentJobs
      });
      setPoolFeedback({
        kind: "success",
        message: "Updated " + poolSelection + " pool configuration."
      });
      pools.refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setPoolFeedback({ kind: "error", message });
    } finally {
      setPoolSubmitting(false);
    }
  };

  const selectedLibraryLabel =
    libraryTypeFilter === "all"
      ? "All types"
      : optionLabel(libraryTypeOptions, libraryTypeFilter, "Selected type");
  const selectedJobLabel =
    jobStateFilter === "all"
      ? "All states"
      : optionLabel(jobStateOptions, jobStateFilter, "Selected state");
  const selectedPoolLabel =
    workerPoolFilter === "all"
      ? "All pools"
      : optionLabel(workerPoolOptions, workerPoolFilter, "Selected pool");
  const selectedStatusLabel =
    workerStatusFilter === "all"
      ? "All statuses"
      : optionLabel(workerStatusOptions, workerStatusFilter, "Selected status");
  const workerFiltersActive =
    workerPoolFilter !== "all" || workerStatusFilter !== "all";
  const workerFilterDescription =
    [
      workerStatusFilter !== "all" ? selectedStatusLabel : null,
      workerPoolFilter !== "all" ? selectedPoolLabel : null
    ]
      .filter(Boolean)
      .join(" · ") || "Selected filters";

  return (
    <main className={styles.main}>
      <header className={styles.hero}>
        <p className={styles.eyebrow}>arrgo</p>
        <h1 className={styles.title}>Cluster overview</h1>
        <p className={styles.subtitle}>
          Stay on top of library health, orchestration jobs, and worker availability
          across your arrgo deployment.
        </p>
      </header>
      <div className={styles.grid}>
        <DataPanel
          title="Library health"
          description="Snapshot of tracked media entities grouped by type."
          loading={library.loading}
          error={library.error}
          onRetry={library.refresh}
          actions={
            libraryStats.total ? (
              <>
                <div className={styles.panelControls}>
                  <div className={styles.controlGroup}>
                    <label
                      className={styles.controlLabel}
                      htmlFor="library-type-filter"
                    >
                      Entity type
                    </label>
                    <select
                      id="library-type-filter"
                      className={styles.controlSelect}
                      value={libraryTypeFilter}
                      onChange={(event) => setLibraryTypeFilter(event.target.value)}
                    >
                      <option value="all">All types</option>
                      {libraryTypeOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
                {libraryEntityOptions.length ? (
                  <form
                    className={[styles.panelControls, styles.inlineForm].join(" ")}
                    onSubmit={handleTriggerJob}
                  >
                    <div className={styles.controlGroup}>
                      <label className={styles.controlLabel} htmlFor="job-type-select">
                        Job type
                      </label>
                      <select
                        id="job-type-select"
                        className={styles.controlSelect}
                        value={jobTypeSelection}
                        onChange={(event) => {
                          setJobTypeSelection(event.target.value);
                          setJobFeedback(null);
                        }}
                      >
                        {JOB_TYPE_OPTIONS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className={styles.controlGroup}>
                      <label
                        className={styles.controlLabel}
                        htmlFor="job-target-select"
                      >
                        Target entity
                      </label>
                      <select
                        id="job-target-select"
                        className={styles.controlSelect}
                        value={jobTargetSelection}
                        onChange={(event) => {
                          setJobTargetSelection(event.target.value);
                          setJobFeedback(null);
                        }}
                      >
                        {libraryEntityOptions.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label} · {option.entityType}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className={styles.controlGroup}>
                      <label
                        className={styles.controlLabel}
                        htmlFor="job-payload-input"
                      >
                        Payload URI
                      </label>
                      <input
                        id="job-payload-input"
                        className={styles.controlInput}
                        type="text"
                        value={jobPayloadURI}
                        onChange={(event) => {
                          setJobPayloadURI(event.target.value);
                          setJobFeedback(null);
                        }}
                        placeholder="Optional"
                        autoComplete="off"
                      />
                    </div>
                    <div className={styles.controlGroup}>
                      <label
                        className={styles.controlLabel}
                        htmlFor="job-idempotency-input"
                      >
                        Idempotency key
                      </label>
                      <input
                        id="job-idempotency-input"
                        className={styles.controlInput}
                        type="text"
                        value={jobIdempotencyKey}
                        onChange={(event) => {
                          setJobIdempotencyKey(event.target.value);
                          setJobFeedback(null);
                        }}
                        placeholder="Optional"
                        autoComplete="off"
                      />
                    </div>
                    <button
                      type="submit"
                      className={styles.primaryButton}
                      disabled={jobSubmitDisabled}
                    >
                      {jobSubmitting ? "Queueing…" : "Queue job"}
                    </button>
                  </form>
                ) : null}
              </>
            ) : null
          }
          footer={libraryFooter}
        >
          {libraryStats.total ? (
            visibleLibrary.length ? (
              <>
                  {jobFeedback ? (
                    <div
                      className={[
                        styles.formMessage,
                        jobFeedback.kind === "success"
                          ? styles.formMessageSuccess
                          : styles.formMessageError
                      ].join(" ")}
                      role={jobFeedback.kind === "error" ? "alert" : "status"}
                      aria-live={
                        jobFeedback.kind === "error" ? "assertive" : "polite"
                      }
                    >
                    <span>{jobFeedback.message}</span>
                    <button
                      type="button"
                      className={styles.messageDismiss}
                      onClick={() => setJobFeedback(null)}
                    >
                      Dismiss
                    </button>
                  </div>
                ) : null}
                  <div className={styles.metricsRow}>
                    <div
                      className={[
                        styles.metricCard,
                        libraryTypeFilter !== "all" ? styles.metricCardEmphasis : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                    <span className={styles.metricValue}>{visibleLibrary.length}</span>
                      <span className={styles.metricLabel}>
                        {libraryTypeFilter === "all"
                          ? "Total entities"
                          : "Filtered · " + selectedLibraryLabel}
                      </span>
                  </div>
                  {libraryTypeFilter !== "all" ? (
                    <div className={styles.metricCard}>
                      <span className={styles.metricValue}>{libraryStats.total}</span>
                      <span className={styles.metricLabel}>Cluster total</span>
                    </div>
                  ) : null}
                  {libraryStats.breakdown.map(([type, count]) => {
                    const isActive =
                      libraryTypeFilter !== "all" &&
                      normalize(type) === libraryTypeFilter;
                    return (
                      <div
                        key={type}
                        className={[styles.metricCard, isActive ? styles.metricCardActive : ""]
                          .filter(Boolean)
                          .join(" ")}
                      >
                        <span className={styles.metricValue}>{count}</span>
                        <span className={styles.metricLabel}>{type}</span>
                      </div>
                    );
                  })}
                </div>
                <ul className={styles.entityList}>
                  {visibleLibrary.slice(0, 5).map((entity) => (
                    <li key={entity.id} className={styles.entityItem}>
                      <span className={styles.entityTitle}>
                        {entity.title || entity.id}
                      </span>
                      <span className={styles.entityType}>{entity.entityType}</span>
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <p className={styles.emptyState}>
                No entities match the selected filter yet. Adjust the filters to
                explore other media types.
              </p>
            )
          ) : (
            <p className={styles.emptyState}>
              No entities have been indexed yet. Once ingestion runs you&apos;ll see
              titles and counts here.
            </p>
          )}
        </DataPanel>
        <DataPanel
          title="Job queue"
          description="Latest orchestration jobs dispatched by the master node."
          loading={jobs.loading}
          error={jobs.error}
          onRetry={jobs.refresh}
          actions={
            jobStats.total ? (
              <div className={styles.panelControls}>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="job-state-filter">
                    Job state
                  </label>
                  <select
                    id="job-state-filter"
                    className={styles.controlSelect}
                    value={jobStateFilter}
                    onChange={(event) => setJobStateFilter(event.target.value)}
                  >
                    <option value="all">All states</option>
                    {jobStateOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            ) : null
          }
          footer={jobFooter}
        >
          {jobStats.total ? (
            visibleJobs.length ? (
              <>
                  <div className={styles.metricsRow}>
                    <div
                      className={[
                        styles.metricCard,
                        jobStateFilter !== "all" ? styles.metricCardEmphasis : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                    <span className={styles.metricValue}>{visibleJobs.length}</span>
                      <span className={styles.metricLabel}>
                        {jobStateFilter === "all"
                          ? "Total jobs"
                          : "Filtered · " + selectedJobLabel}
                      </span>
                  </div>
                  {jobStateFilter !== "all" ? (
                    <div className={styles.metricCard}>
                      <span className={styles.metricValue}>{jobStats.total}</span>
                      <span className={styles.metricLabel}>Cluster total</span>
                    </div>
                  ) : null}
                    {jobStats.counts.map(([state, count]) => {
                      const isActive =
                        jobStateFilter !== "all" &&
                        normalize(state) === jobStateFilter;
                      return (
                        <div
                          key={state}
                          className={[styles.metricCard, isActive ? styles.metricCardActive : ""]
                            .filter(Boolean)
                            .join(" ")}
                        >
                          <span className={styles.metricValue}>{count}</span>
                          <span className={styles.metricLabel}>{state}</span>
                        </div>
                      );
                    })}
                </div>
                <div className={styles.analyticsGrid}>
                  <div className={styles.analyticsCard}>
                    <h3 className={styles.analyticsTitle}>By job type</h3>
                    {queueAnalytics.byType.length ? (
                      <ul className={styles.analyticsList}>
                        {queueAnalytics.byType.map(([type, count]) => (
                          <li key={type} className={styles.analyticsListItem}>
                            <span className={styles.analyticsLabel}>{type}</span>
                            <span className={styles.analyticsValue}>{count}</span>
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className={styles.emptyState}>No jobs recorded yet.</p>
                    )}
                  </div>
                  <div className={styles.analyticsCard}>
                    <h3 className={styles.analyticsTitle}>Recent activity</h3>
                    <dl className={styles.analyticsDetails}>
                      <div className={styles.analyticsDetailRow}>
                        <dt>Average attempts</dt>
                        <dd>
                          {queueAnalytics.total
                            ? queueAnalytics.averageAttempts.toFixed(1)
                            : "—"}
                        </dd>
                      </div>
                      <div className={styles.analyticsDetailRow}>
                        <dt>Last submission</dt>
                        <dd>
                          {queueAnalytics.latestSubmittedAt
                            ? [
                                formatRelativeTime(queueAnalytics.latestSubmittedAt),
                                formatTimestamp(queueAnalytics.latestSubmittedAt)
                              ].join(" · ")
                            : "—"}
                        </dd>
                      </div>
                    </dl>
                  </div>
                </div>
                <div className={styles.tableWrapper}>
                  <table className={styles.table}>
                    <thead>
                      <tr>
                        <th scope="col">Job</th>
                        <th scope="col">Target</th>
                        <th scope="col">State</th>
                        <th scope="col">Submitted</th>
                        <th scope="col">Attempt</th>
                        <th scope="col">Details</th>
                      </tr>
                    </thead>
                    <tbody>
                      {visibleJobs.slice(0, 8).map((job) => (
                        <tr key={job.id}>
                          <td>
                            <div className={styles.jobDetails}>
                              <span className={styles.monospace}>
                                {job.jobType}
                              </span>
                              {job.payloadURI ? (
                                <span className={styles.smallText}>{job.payloadURI}</span>
                              ) : null}
                            </div>
                          </td>
                          <td className={styles.monospace}>{job.targetID}</td>
                          <td>
                            <StatusPill
                              label={job.state || "unknown"}
                              tone={jobTone(job.state)}
                            />
                          </td>
                          <td>{formatTimestamp(job.submittedAt)}</td>
                          <td>{job.attempt}</td>
                          <td>
                            <button
                              type="button"
                              className={styles.linkButton}
                              onClick={() => setSelectedJob(job)}
                            >
                              Inspect
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <p className={styles.emptyState}>
                No jobs match the selected state yet. Try a different filter to
                inspect other workloads.
              </p>
            )
          ) : (
            <p className={styles.emptyState}>
              The job queue is empty. Schedule a library scan or import to see
              activity here.
            </p>
          )}
        </DataPanel>
        <DataPanel
          title="Worker fleet"
          description="Active nodes reporting heartbeats to the master."
          loading={workers.loading}
          error={workers.error}
          onRetry={workers.refresh}
          actions={
            workerStats.total ? (
              <div className={styles.panelControls}>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="worker-pool-filter">
                    Worker pool
                  </label>
                  <select
                    id="worker-pool-filter"
                    className={styles.controlSelect}
                    value={workerPoolFilter}
                    onChange={(event) => setWorkerPoolFilter(event.target.value)}
                  >
                    <option value="all">All pools</option>
                    {workerPoolOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="worker-status-filter">
                    Status
                  </label>
                  <select
                    id="worker-status-filter"
                    className={styles.controlSelect}
                    value={workerStatusFilter}
                    onChange={(event) => setWorkerStatusFilter(event.target.value)}
                  >
                    <option value="all">All statuses</option>
                    {workerStatusOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            ) : null
          }
          footer={workerFooter}
        >
          {workerStats.total ? (
            visibleWorkers.length ? (
              <>
                  <div className={styles.metricsRow}>
                    <div
                      className={[
                        styles.metricCard,
                        workerFiltersActive ? styles.metricCardEmphasis : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                    <span className={styles.metricValue}>{visibleWorkers.length}</span>
                      <span className={styles.metricLabel}>
                        {workerFiltersActive
                          ? "Filtered · " + workerFilterDescription
                          : "Total workers"}
                      </span>
                  </div>
                  {workerFiltersActive ? (
                    <div className={styles.metricCard}>
                      <span className={styles.metricValue}>{workerStats.total}</span>
                      <span className={styles.metricLabel}>Cluster total</span>
                    </div>
                  ) : null}
                    <div
                      className={[
                        styles.metricCard,
                        workerStatusFilter === "healthy"
                          ? styles.metricCardActive
                          : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                    <span className={styles.metricValue}>{workerStats.healthy}</span>
                    <span className={styles.metricLabel}>Healthy</span>
                  </div>
                    <div
                      className={[
                        styles.metricCard,
                        workerStatusFilter === "stale"
                          ? styles.metricCardActive
                          : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                    <span className={styles.metricValue}>{workerStats.stale}</span>
                    <span className={styles.metricLabel}>Stale</span>
                  </div>
                </div>
                {workerStats.pools.length ? (
                  <div className={styles.badgeGroup}>
                    {workerStats.pools.map(([pool, count]) => {
                      const isActive =
                        workerPoolFilter !== "all" &&
                        normalize(pool, "unassigned") === workerPoolFilter;
                      return (
                          <span
                            key={pool}
                            className={[
                              styles.poolBadge,
                              styles.monospace,
                              isActive ? styles.poolBadgeActive : ""
                            ]
                              .filter(Boolean)
                              .join(" ")}
                            title={String(count) + (count === 1 ? " worker" : " workers")}
                          >
                          {pool} · {count}
                        </span>
                      );
                    })}
                  </div>
                ) : null}
                <div className={styles.tableWrapper}>
                  <table className={styles.table}>
                    <thead>
                      <tr>
                        <th scope="col">Worker</th>
                        <th scope="col">Pool</th>
                        <th scope="col">Status</th>
                        <th scope="col">Active jobs</th>
                        <th scope="col">Last heartbeat</th>
                      </tr>
                    </thead>
                    <tbody>
                      {visibleWorkers.slice(0, 8).map((worker) => (
                        <tr key={worker.id}>
                          <td>
                            <div className={styles.jobDetails}>
                                <span
                                  className={styles.monospace}
                                  title={worker.id}
                                >
                                {shortenIdentifier(worker.id)}
                              </span>
                              <span className={styles.smallText}>{worker.version}</span>
                            </div>
                          </td>
                          <td>{worker.pool}</td>
                          <td>
                            <StatusPill
                              label={worker.status || "unknown"}
                              tone={workerTone(worker.status)}
                            />
                          </td>
                          <td>{worker.activeJobs}</td>
                          <td>
                            <span className={styles.smallText}>
                              {formatTimestamp(worker.lastHeartbeatAt)}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <p className={styles.emptyState}>
                No workers match the current filters. Broaden the filters to view
                other pools or statuses.
              </p>
            )
          ) : (
            <p className={styles.emptyState}>
              No workers are currently registered. Bring a worker online or send a
              heartbeat to populate this view.
            </p>
          )}
        </DataPanel>
        <DataPanel
          title="Worker pool configuration"
          description="Tune queue assignments and cadence for master/worker pools."
          loading={pools.loading}
          error={pools.error}
          onRetry={pools.refresh}
          actions={
            pools.data && pools.data.length ? (
              <div className={styles.panelControls}>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="pool-select">
                    Pool
                  </label>
                  <select
                    id="pool-select"
                    className={styles.controlSelect}
                    value={poolSelection}
                    onChange={(event) => {
                      setPoolSelection(event.target.value);
                      setPoolFeedback(null);
                    }}
                  >
                    {poolOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            ) : null
          }
        >
          {pools.loading ? null : pools.data && pools.data.length ? (
            <form className={styles.poolForm} onSubmit={handlePoolSubmit}>
              <div className={styles.metricsRow}>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{selectedPoolWorkerCount}</span>
                  <span className={styles.metricLabel}>Workers in pool</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {selectedPoolConfig
                      ? String(selectedPoolConfig.heartbeatIntervalSeconds) + "s"
                      : "—"}
                  </span>
                  <span className={styles.metricLabel}>Heartbeat interval</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {selectedPoolConfig
                      ? String(selectedPoolConfig.staleAfterSeconds) + "s"
                      : "—"}
                  </span>
                  <span className={styles.metricLabel}>Stale threshold</span>
                </div>
                {selectedPoolConfig?.maxConcurrentJobs !== undefined ? (
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {selectedPoolConfig.maxConcurrentJobs}
                    </span>
                    <span className={styles.metricLabel}>Max concurrent jobs</span>
                  </div>
                ) : null}
              </div>
              <div className={styles.poolFormGrid}>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="pool-description">
                    Description
                  </label>
                  <input
                    id="pool-description"
                    className={styles.controlInput}
                    value={poolForm.description}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        description: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                    placeholder="Optional summary"
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="pool-stream">
                    Stream
                  </label>
                  <input
                    id="pool-stream"
                    className={styles.controlInput}
                    value={poolForm.stream}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        stream: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                    required
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="pool-consumer-group"
                  >
                    Consumer group
                  </label>
                  <input
                    id="pool-consumer-group"
                    className={styles.controlInput}
                    value={poolForm.consumerGroup}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        consumerGroup: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                    required
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="pool-heartbeat-interval"
                  >
                    Heartbeat interval (s)
                  </label>
                  <input
                    id="pool-heartbeat-interval"
                    type="number"
                    min={0}
                    className={styles.controlInput}
                    value={poolForm.heartbeatIntervalSeconds}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        heartbeatIntervalSeconds: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="pool-stale-after">
                    Stale after (s)
                  </label>
                  <input
                    id="pool-stale-after"
                    type="number"
                    min={0}
                    className={styles.controlInput}
                    value={poolForm.staleAfterSeconds}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        staleAfterSeconds: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label className={styles.controlLabel} htmlFor="pool-max-jobs">
                    Max concurrent jobs
                  </label>
                  <input
                    id="pool-max-jobs"
                    type="number"
                    min={0}
                    className={styles.controlInput}
                    value={poolForm.maxConcurrentJobs}
                    onChange={(event) => {
                      setPoolForm((previous) => ({
                        ...previous,
                        maxConcurrentJobs: event.target.value
                      }));
                      setPoolFeedback(null);
                    }}
                    placeholder="Optional"
                  />
                </div>
              </div>
              {poolFeedback ? (
                <div
                  className={[
                    styles.formMessage,
                    poolFeedback.kind === "success"
                      ? styles.formMessageSuccess
                      : styles.formMessageError
                  ].join(" ")}
                >
                  <span>{poolFeedback.message}</span>
                  <button
                    type="button"
                    className={styles.messageDismiss}
                    onClick={() => setPoolFeedback(null)}
                  >
                    Dismiss
                  </button>
                </div>
              ) : null}
              <button
                type="submit"
                className={styles.primaryButton}
                disabled={poolSubmitting}
              >
                Save configuration
              </button>
            </form>
          ) : (
            <p className={styles.emptyState}>
              Define a worker pool to begin tuning queue assignments.
            </p>
          )}
        </DataPanel>
      </div>
      {selectedJob ? (
        <div className={styles.drawerBackdrop} role="presentation">
          <div
            className={styles.drawer}
            role="dialog"
            aria-modal="true"
            aria-label={"Details for job " + selectedJob.id}
          >
            <div className={styles.drawerHeader}>
              <h2 className={styles.drawerTitle}>Job details</h2>
              <button
                type="button"
                className={styles.drawerClose}
                onClick={() => setSelectedJob(null)}
              >
                Close
              </button>
            </div>
            <dl className={styles.drawerDetails}>
              <div className={styles.drawerDetailRow}>
                <dt>Job ID</dt>
                <dd className={styles.monospace}>{selectedJob.id}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Job type</dt>
                <dd className={styles.monospace}>{selectedJob.jobType}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Target</dt>
                <dd className={styles.monospace}>{selectedJob.targetID}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Status</dt>
                <dd>
                  <StatusPill
                    label={selectedJob.state || "unknown"}
                    tone={jobTone(selectedJob.state)}
                  />
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Payload URI</dt>
                <dd className={styles.monospace}>
                  {selectedJob.payloadURI || "—"}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Idempotency key</dt>
                <dd className={styles.monospace}>
                  {selectedJob.idempotencyKey || "—"}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Attempt</dt>
                <dd>{selectedJob.attempt}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Submitted</dt>
                <dd>
                  {formatTimestamp(selectedJob.submittedAt)}
                  {selectedJob.submittedAt ? (
                    <span className={styles.drawerSubtle}>
                      {formatRelativeTime(selectedJob.submittedAt)}
                    </span>
                  ) : null}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Last attempt</dt>
                <dd>
                  {selectedJob.lastAttemptAt ? (
                    <>
                      {formatTimestamp(selectedJob.lastAttemptAt)}
                      <span className={styles.drawerSubtle}>
                        {formatRelativeTime(selectedJob.lastAttemptAt)}
                      </span>
                    </>
                  ) : (
                    "—"
                  )}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Completed</dt>
                <dd>
                  {selectedJob.completedAt ? (
                    <>
                      {formatTimestamp(selectedJob.completedAt)}
                      <span className={styles.drawerSubtle}>
                        {formatRelativeTime(selectedJob.completedAt)}
                      </span>
                    </>
                  ) : (
                    "—"
                  )}
                </dd>
              </div>
            </dl>
          </div>
        </div>
      ) : null}
    </main>
  );
}
