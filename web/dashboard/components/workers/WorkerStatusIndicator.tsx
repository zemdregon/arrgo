"use client";

import { useMemo } from "react";
import { useEnvironment } from "../environment/EnvironmentContext";
import { useRestQuery } from "../../hooks/useRestQuery";
import type { WorkerSummary } from "../../lib/api";
import styles from "./WorkerStatusIndicator.module.css";

type OverallStatus = "unknown" | "healthy" | "warning" | "danger";

type WorkerSummaryCounts = {
  total: number;
  healthy: number;
  stale: number;
  offline: number;
};

function summarizeWorkers(data?: WorkerSummary[]): WorkerSummaryCounts {
  const workers = data ?? [];
  let healthy = 0;
  let stale = 0;
  let offline = 0;
  for (const worker of workers) {
    const status = worker.status?.trim().toLowerCase();
    if (status === "healthy") {
      healthy += 1;
    } else if (status === "stale") {
      stale += 1;
    } else {
      offline += 1;
    }
  }
  return {
    total: workers.length,
    healthy,
    stale,
    offline,
  };
}

function computeOverallStatus(
  summary: WorkerSummaryCounts,
  loading: boolean,
  error?: string,
): OverallStatus {
  if (error) {
    return "danger";
  }
  if (loading && summary.total === 0) {
    return "unknown";
  }
  if (summary.total === 0) {
    return "warning";
  }
  if (summary.offline > 0) {
    return "danger";
  }
  if (summary.stale > 0) {
    return "warning";
  }
  return "healthy";
}

function formatSummary(
  summary: WorkerSummaryCounts,
  loading: boolean,
  error?: string,
): string {
  if (error) {
    return `Error · ${error}`;
  }
  if (loading && summary.total === 0) {
    return "Loading…";
  }
  if (summary.total === 0) {
    return "No workers connected";
  }
  const parts = [`${summary.healthy} healthy`];
  if (summary.stale > 0) {
    parts.push(`${summary.stale} stale`);
  }
  if (summary.offline > 0) {
    parts.push(`${summary.offline} offline`);
  }
  return parts.join(" · ");
}

export function WorkerStatusIndicator() {
  const { current } = useEnvironment();
  const apiBaseUrl = current.apiUrl;

  const workersQuery = useRestQuery(
    ["workers", "indicator", apiBaseUrl],
    (client) => client.fetchWorkers(),
    { refetchInterval: 30000 },
  );

  const { data, isPending, isFetching, error, refetch } = workersQuery;
  const loading = isPending && !data;
  const errorMessage = error?.message;
  const summary = useMemo(() => summarizeWorkers(data), [data]);
  const overallStatus = computeOverallStatus(summary, loading, errorMessage);
  const label = formatSummary(summary, loading, errorMessage);

  return (
    <button
      type="button"
      className={styles.root}
      data-status={overallStatus}
      onClick={() => {
        void refetch();
      }}
      disabled={loading && !data}
      aria-busy={isFetching}
      title="Refresh worker status"
    >
      <span className={styles.statusDot} aria-hidden />
      <span className={styles.copy}>
        <span className={styles.label}>Workers</span>
        <span className={styles.value}>{label}</span>
      </span>
    </button>
  );
}
