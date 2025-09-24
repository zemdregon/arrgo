"use client";

import { useCallback, useMemo } from "react";
import { PageHeader } from "../../../components/layout/PageHeader";
import { useEnvironment } from "../../../components/environment/EnvironmentContext";
import { useAuth } from "../../../components/auth/AuthContext";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import { useRestQuery } from "../../../hooks/useRestQuery";
import type { QueueAnalyticsOverview } from "../../../lib/api";
import {
  formatRelativeTime,
  formatTimestamp,
} from "../utils/dashboardFormatting";
import {
  buildQueueSummary,
  formatCount,
  formatDuration,
  formatPercentage,
  formatStatusLabel,
  queueStatusTone,
} from "./utils/queueAnalytics";
import styles from "./page.module.css";

export default function QueueAnalyticsPage() {
  const { current } = useEnvironment();
  const { status: authStatus, hasPermission } = useAuth();
  const apiBaseUrl = current.apiUrl;

  const canReadQueue =
    authStatus === "authenticated" && hasPermission("worker:read");

  const queueQuery = useRestQuery<QueueAnalyticsOverview>(
    ["queue", "analytics", apiBaseUrl],
    (client) => client.fetchQueueAnalytics(),
    {
      enabled: canReadQueue,
      refetchInterval: 30000,
      refetchOnWindowFocus: false,
    },
  );

  const { data, isPending, error, refetch, isFetching } = queueQuery;

  const summary = useMemo(() => buildQueueSummary(data), [data]);

  const streamLag = useMemo(() => {
    return [...(data?.streamLag ?? [])].sort((a, b) => {
      return (b.lagSeconds ?? 0) - (a.lagSeconds ?? 0);
    });
  }, [data]);

  const retryMetrics = useMemo(() => {
    return [...(data?.retries ?? [])].sort((a, b) => {
      return (b.retryCount ?? 0) - (a.retryCount ?? 0);
    });
  }, [data]);

  const slaIndicators = data?.sla ?? [];

  const handleRefresh = useCallback(() => {
    void refetch();
  }, [refetch]);

  const streamFooter = useMemo(() => {
    if (!data) {
      return undefined;
    }
    const notes: string[] = [];
    if (data.observedWindowSeconds !== undefined) {
      notes.push(`Window: last ${formatDuration(data.observedWindowSeconds)}`);
    }
    if (data.updatedAt) {
      notes.push(`Updated ${formatTimestamp(data.updatedAt)}`);
    }
    return notes.length ? <span>{notes.join(" · ")}</span> : undefined;
  }, [data]);

  const lastUpdatedLabel = data?.updatedAt
    ? `Last updated ${formatRelativeTime(data.updatedAt)} · ${formatTimestamp(
        data.updatedAt,
      )}`
    : "Awaiting analytics snapshot";

  return (
    <div className={styles.page}>
      <PageHeader
        eyebrow="Queues"
        title="Queue analytics"
        description="Track stream lag, retry behaviour, and SLA compliance across arrgo orchestration pipelines."
        actions={
          <div className={styles.headerActions}>
            <span className={styles.lastUpdated}>{lastUpdatedLabel}</span>
            <button
              type="button"
              className={styles.refreshButton}
              onClick={handleRefresh}
              disabled={!canReadQueue || isFetching}
            >
              {isFetching ? "Refreshing…" : "Refresh data"}
            </button>
          </div>
        }
      />
      <div className={styles.panels}>
        <DataPanel
          title="Stream lag"
          description="Understand where consumer groups are falling behind their streams."
          loading={isPending}
          error={error?.message}
          onRetry={handleRefresh}
          footer={streamFooter}
        >
          {streamLag.length ? (
            <>
              <div className={styles.metricsRow}>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {formatCount(summary.streamCount)}
                  </span>
                  <span className={styles.metricLabel}>Streams observed</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {formatCount(summary.impactedStreamCount)}
                  </span>
                  <span className={styles.metricLabel}>Impacted streams</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {formatDuration(summary.maxLagSeconds)}
                  </span>
                  <span className={styles.metricLabel}>Peak lag</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {formatCount(summary.totalLagMessages)}
                  </span>
                  <span className={styles.metricLabel}>Messages behind</span>
                </div>
              </div>
              <div className={styles.tableWrapper}>
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th scope="col">Stream</th>
                      <th scope="col">Consumer group</th>
                      <th scope="col">Backlog</th>
                      <th scope="col">Lag</th>
                      <th scope="col">Oldest pending</th>
                      <th scope="col">Last produced</th>
                      <th scope="col">Last consumed</th>
                      <th scope="col">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {streamLag.map((metric) => (
                      <tr
                        key={`${metric.stream}-${metric.consumerGroup}`}
                      >
                        <td className={styles.monospace}>{metric.stream}</td>
                        <td className={styles.monospace}>
                          {metric.consumerGroup}
                        </td>
                        <td>{formatCount(metric.lagMessages)}</td>
                        <td>{formatDuration(metric.lagSeconds)}</td>
                        <td>{formatDuration(metric.oldestPendingAgeSeconds)}</td>
                        <td>
                          {metric.lastProducedAt ? (
                            <>
                              {formatRelativeTime(metric.lastProducedAt)}
                              <div className={styles.smallText}>
                                {formatTimestamp(metric.lastProducedAt)}
                              </div>
                            </>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td>
                          {metric.lastConsumedAt ? (
                            <>
                              {formatRelativeTime(metric.lastConsumedAt)}
                              <div className={styles.smallText}>
                                {formatTimestamp(metric.lastConsumedAt)}
                              </div>
                            </>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td>
                          <StatusPill
                            label={formatStatusLabel(metric.status)}
                            tone={queueStatusTone(metric.status)}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : (
            <p className={styles.emptyState}>
              No stream lag metrics are available yet. Once workers consume
              active streams, lag trends will appear here.
            </p>
          )}
        </DataPanel>
        <div className={styles.secondaryGrid}>
          <DataPanel
            title="Retry behaviour"
            description="Identify job types that are retrying frequently or showing elevated failure rates."
            loading={isPending}
            error={error?.message}
            onRetry={handleRefresh}
          >
            {retryMetrics.length ? (
              <>
                <div className={styles.metricsRow}>
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {formatCount(summary.totalRetries)}
                    </span>
                    <span className={styles.metricLabel}>Active retries</span>
                  </div>
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {formatCount(summary.impactedJobTypes)}
                    </span>
                    <span className={styles.metricLabel}>Job types impacted</span>
                  </div>
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {formatStatusLabel(summary.worstStatus)}
                    </span>
                    <span className={styles.metricLabel}>Cluster posture</span>
                  </div>
                </div>
                <div className={styles.tableWrapper}>
                  <table className={styles.table}>
                    <thead>
                      <tr>
                        <th scope="col">Job type</th>
                        <th scope="col">Retries</th>
                        <th scope="col">Failure rate</th>
                        <th scope="col">Oldest retry</th>
                        <th scope="col">Last retry</th>
                        <th scope="col">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {retryMetrics.map((metric) => (
                        <tr key={metric.jobType}>
                          <td className={styles.monospace}>{metric.jobType}</td>
                          <td>{formatCount(metric.retryCount)}</td>
                          <td>{formatPercentage(metric.recentFailureRate)}</td>
                          <td>
                            {formatDuration(metric.oldestRetryAgeSeconds)}
                          </td>
                          <td>
                            {metric.lastRetryAt ? (
                              <>
                                {formatRelativeTime(metric.lastRetryAt)}
                                <div className={styles.smallText}>
                                  {formatTimestamp(metric.lastRetryAt)}
                                </div>
                              </>
                            ) : (
                              "—"
                            )}
                          </td>
                          <td>
                            <StatusPill
                              label={formatStatusLabel(metric.status)}
                              tone={queueStatusTone(metric.status)}
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <p className={styles.emptyState}>
                No retries detected in the observation window. When jobs begin
                retrying, they will appear here with their failure rates.
              </p>
            )}
          </DataPanel>
          <DataPanel
            title="SLA indicators"
            description="Compare queue objectives against observed latency and reliability."
            loading={isPending}
            error={error?.message}
            onRetry={handleRefresh}
          >
            {slaIndicators.length ? (
              <>
                <div className={styles.metricsRow}>
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {formatCount(summary.slaBreaches)}
                    </span>
                    <span className={styles.metricLabel}>Breaches today</span>
                  </div>
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {formatCount(summary.slaAtRisk)}
                    </span>
                    <span className={styles.metricLabel}>Queues at risk</span>
                  </div>
                </div>
                <div className={styles.slaGrid}>
                  {slaIndicators.map((indicator) => (
                    <article key={indicator.queue} className={styles.slaCard}>
                      <div className={styles.slaHeader}>
                        <h3 className={styles.slaTitle}>{indicator.queue}</h3>
                        <StatusPill
                          label={formatStatusLabel(indicator.status)}
                          tone={queueStatusTone(indicator.status)}
                        />
                      </div>
                      <div className={styles.slaMetrics}>
                        <div className={styles.slaMetric}>
                          <span className={styles.slaMetricLabel}>Target P99</span>
                          <span className={styles.slaMetricValue}>
                            {formatDuration(indicator.targetSeconds)}
                          </span>
                        </div>
                        <div className={styles.slaMetric}>
                          <span className={styles.slaMetricLabel}>Observed P95</span>
                          <span className={styles.slaMetricValue}>
                            {formatDuration(indicator.p95Seconds)}
                          </span>
                        </div>
                        <div className={styles.slaMetric}>
                          <span className={styles.slaMetricLabel}>Observed P99</span>
                          <span className={styles.slaMetricValue}>
                            {formatDuration(indicator.p99Seconds)}
                          </span>
                        </div>
                        <div className={styles.slaMetric}>
                          <span className={styles.slaMetricLabel}>Success rate</span>
                          <span className={styles.slaMetricValue}>
                            {formatPercentage(indicator.successRate)}
                          </span>
                        </div>
                        <div className={styles.slaMetric}>
                          <span className={styles.slaMetricLabel}>Breaches today</span>
                          <span className={styles.slaMetricValue}>
                            {formatCount(indicator.breachesToday)}
                          </span>
                        </div>
                      </div>
                      {indicator.updatedAt ? (
                        <div className={styles.slaFooter}>
                          Updated {formatRelativeTime(indicator.updatedAt)} ·
                          {" "}
                          {formatTimestamp(indicator.updatedAt)}
                        </div>
                      ) : null}
                    </article>
                  ))}
                </div>
              </>
            ) : (
              <p className={styles.emptyState}>
                No SLA indicators are configured for this environment. Define
                queue objectives to track performance budgets.
              </p>
            )}
          </DataPanel>
        </div>
      </div>
    </div>
  );
}
