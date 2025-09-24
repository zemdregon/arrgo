"use client";

import { useEffect, useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import type { WorkerSummary } from "../../../lib/api";
import {
  buildOptions,
  buildWorkerStats,
  formatShowingSummary,
  normalize,
  optionLabel,
} from "../utils/dashboardData";
import { formatTimestamp, shortenIdentifier, workerTone } from "../utils/dashboardFormatting";
import styles from "../page.module.css";

interface WorkersPanelProps {
  data?: WorkerSummary[];
  loading: boolean;
  error?: string;
  onRetry: () => void;
}

export function WorkersPanel({ data, loading, error, onRetry }: WorkersPanelProps) {
  const [workerPoolFilter, setWorkerPoolFilter] = useState("all");
  const [workerStatusFilter, setWorkerStatusFilter] = useState("all");

  const workerStats = useMemo(() => buildWorkerStats(data), [data]);
  const workerPoolOptions = useMemo(
    () =>
      buildOptions(
        (data ?? []).map((worker) => worker.pool),
        "Unassigned",
        "unassigned",
      ),
    [data],
  );
  const workerStatusOptions = useMemo(
    () =>
      buildOptions(
        (data ?? []).map((worker) => worker.status),
        "Unknown status",
        "unknown",
      ),
    [data],
  );

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

  const visibleWorkers = useMemo(() => {
    const nodes = data ?? [];
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
  }, [data, workerPoolFilter, workerStatusFilter]);

  const workerFooter = workerStats.total
    ? formatShowingSummary(
        visibleWorkers.length,
        8,
        workerPoolFilter === "all" && workerStatusFilter === "all"
          ? "workers"
          : "matches",
        workerStats.total,
      )
    : undefined;

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
      workerPoolFilter !== "all" ? selectedPoolLabel : null,
    ]
      .filter(Boolean)
      .join(" · ") || "Selected filters";

  return (
    <DataPanel
      title="Worker fleet"
      description="Active nodes reporting heartbeats to the master."
      loading={loading}
      error={error}
      onRetry={onRetry}
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
                  workerFiltersActive ? styles.metricCardEmphasis : "",
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
                  workerStatusFilter === "healthy" ? styles.metricCardActive : "",
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
                  workerStatusFilter === "stale" ? styles.metricCardActive : "",
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
                        isActive ? styles.poolBadgeActive : "",
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
                          <span className={styles.monospace} title={worker.id}>
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
  );
}
