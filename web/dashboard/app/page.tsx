"use client";

import { useMemo } from "react";
import { DataPanel } from "../components/DataPanel";
import { StatusPill } from "../components/StatusPill";
import { useApiData } from "../hooks/useApiData";
import {
  fetchEntities,
  fetchJobs,
  fetchWorkers,
  JobSummary,
  LibraryEntity,
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

  const libraryStats = useMemo(() => buildLibraryStats(library.data), [
    library.data
  ]);
  const jobStats = useMemo(() => buildJobStats(jobs.data), [jobs.data]);
  const workerStats = useMemo(() => buildWorkerStats(workers.data), [
    workers.data
  ]);

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
          footer={
            libraryStats.total
              ? `Tracking ${libraryStats.total} entities`
              : undefined
          }
        >
          {libraryStats.total ? (
            <>
              <div className={styles.metricsRow}>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{libraryStats.total}</span>
                  <span className={styles.metricLabel}>Total entities</span>
                </div>
                {libraryStats.breakdown.map(([type, count]) => (
                  <div key={type} className={styles.metricCard}>
                    <span className={styles.metricValue}>{count}</span>
                    <span className={styles.metricLabel}>{type}</span>
                  </div>
                ))}
              </div>
              <ul className={styles.entityList}>
                {(library.data ?? []).slice(0, 5).map((entity) => (
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
          footer={
            jobStats.total
              ? `Showing ${Math.min(jobStats.total, 8)} of ${jobStats.total} jobs`
              : undefined
          }
        >
          {jobStats.total ? (
            <>
              <div className={styles.metricsRow}>
                {jobStats.counts.map(([state, count]) => (
                  <div key={state} className={styles.metricCard}>
                    <span className={styles.metricValue}>{count}</span>
                    <span className={styles.metricLabel}>{state}</span>
                  </div>
                ))}
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
                    {(jobs.data ?? []).slice(0, 8).map((job) => (
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
          footer={
            workerStats.total
              ? `Workers across ${workerStats.pools.length} pool${
                  workerStats.pools.length === 1 ? "" : "s"
                }`
              : undefined
          }
        >
          {workerStats.total ? (
            <>
              <div className={styles.metricsRow}>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{workerStats.total}</span>
                  <span className={styles.metricLabel}>Total</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{workerStats.healthy}</span>
                  <span className={styles.metricLabel}>Healthy</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{workerStats.stale}</span>
                  <span className={styles.metricLabel}>Stale</span>
                </div>
              </div>
              {workerStats.pools.length ? (
                <div className={styles.badgeGroup}>
                  {workerStats.pools.map(([pool, count]) => (
                    <span
                      key={pool}
                      className={`${styles.poolBadge} ${styles.monospace}`}
                      title={`${count} worker${count === 1 ? "" : "s"}`}
                    >
                      {pool} · {count}
                    </span>
                  ))}
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
                    {(workers.data ?? []).slice(0, 8).map((worker) => (
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
              No workers are currently registered. Bring a worker online or send a
              heartbeat to populate this view.
            </p>
          )}
        </DataPanel>
      </div>
    </main>
  );
}
