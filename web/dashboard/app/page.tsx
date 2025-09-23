"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { DataPanel } from "../components/DataPanel";
import { StatusPill } from "../components/StatusPill";
import { useApiData } from "../hooks/useApiData";
import {
  fetchEntities,
  fetchJobs,
  fetchWorkers,
  JobSummary,
  LibraryEntity,
  triggerJob,
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

export default function DashboardPage() {
  const library = useApiData(fetchEntities);
  const jobs = useApiData(fetchJobs);
  const workers = useApiData(fetchWorkers);

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

  const libraryStats = useMemo(() => buildLibraryStats(library.data), [
    library.data
  ]);
  const jobStats = useMemo(() => buildJobStats(jobs.data), [jobs.data]);
  const workerStats = useMemo(() => buildWorkerStats(workers.data), [
    workers.data
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

  const libraryFooter = libraryStats.total
    ? `Showing ${Math.min(visibleLibrary.length, 5)} of ${visibleLibrary.length} ${
        libraryTypeFilter === "all" ? "entities" : "matches"
      } (${libraryStats.total} total)`
    : undefined;
  const jobFooter = jobStats.total
    ? `Showing ${Math.min(visibleJobs.length, 8)} of ${visibleJobs.length} ${
        jobStateFilter === "all" ? "jobs" : "matches"
      } (${jobStats.total} total)`
    : undefined;
  const workerFooter = workerStats.total
    ? `Showing ${Math.min(visibleWorkers.length, 8)} of ${visibleWorkers.length} ${
        workerPoolFilter === "all" && workerStatusFilter === "all"
          ? "workers"
          : "matches"
      } (${workerStats.total} total)`
    : undefined;

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
      ? `${jobTargetOption.label} · ${jobTargetOption.entityType}`
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
        message: `Queued ${jobTypeLabel} job for ${jobTargetDescription}.`
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
                    className={`${styles.panelControls} ${styles.inlineForm}`}
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
                            {`${option.label} · ${option.entityType}`}
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
                    className={`${styles.formMessage} ${
                      jobFeedback.kind === "success"
                        ? styles.formMessageSuccess
                        : styles.formMessageError
                    }`}
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
                    className={`${styles.metricCard} ${
                      libraryTypeFilter !== "all" ? styles.metricCardEmphasis : ""
                    }`}
                  >
                    <span className={styles.metricValue}>{visibleLibrary.length}</span>
                    <span className={styles.metricLabel}>
                      {libraryTypeFilter === "all"
                        ? "Total entities"
                        : `Filtered · ${selectedLibraryLabel}`}
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
                        className={`${styles.metricCard} ${
                          isActive ? styles.metricCardActive : ""
                        }`}
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
                    className={`${styles.metricCard} ${
                      jobStateFilter !== "all" ? styles.metricCardEmphasis : ""
                    }`}
                  >
                    <span className={styles.metricValue}>{visibleJobs.length}</span>
                    <span className={styles.metricLabel}>
                      {jobStateFilter === "all"
                        ? "Total jobs"
                        : `Filtered · ${selectedJobLabel}`}
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
                        className={`${styles.metricCard} ${
                          isActive ? styles.metricCardActive : ""
                        }`}
                      >
                        <span className={styles.metricValue}>{count}</span>
                        <span className={styles.metricLabel}>{state}</span>
                      </div>
                    );
                  })}
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
                      </tr>
                    </thead>
                    <tbody>
                      {visibleJobs.slice(0, 8).map((job) => (
                        <tr key={job.id}>
                          <td>
                            <div className={styles.jobDetails}>
                              <span className={`${styles.monospace}`}>
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
                    className={`${styles.metricCard} ${
                      workerFiltersActive ? styles.metricCardEmphasis : ""
                    }`}
                  >
                    <span className={styles.metricValue}>{visibleWorkers.length}</span>
                    <span className={styles.metricLabel}>
                      {workerFiltersActive
                        ? `Filtered · ${workerFilterDescription}`
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
                    className={`${styles.metricCard} ${
                      workerStatusFilter === "healthy"
                        ? styles.metricCardActive
                        : ""
                    }`}
                  >
                    <span className={styles.metricValue}>{workerStats.healthy}</span>
                    <span className={styles.metricLabel}>Healthy</span>
                  </div>
                  <div
                    className={`${styles.metricCard} ${
                      workerStatusFilter === "stale"
                        ? styles.metricCardActive
                        : ""
                    }`}
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
                          className={`${styles.poolBadge} ${styles.monospace} ${
                            isActive ? styles.poolBadgeActive : ""
                          }`}
                          title={`${count} worker${count === 1 ? "" : "s"}`}
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
                                className={`${styles.monospace}`}
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
      </div>
    </main>
  );
}
