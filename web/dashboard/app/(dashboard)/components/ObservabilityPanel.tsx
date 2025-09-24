"use client";

import { useEffect, useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import type {
  LogEntry,
  MetricSeries,
  ObservabilitySnapshot,
  TraceSample,
} from "../../../lib/api";
import { onTrace, type TraceEvent } from "../../../lib/observability/tracing";
import {
  formatDurationMs,
  formatRelativeTime,
  formatTimestamp,
  logTone,
} from "../utils/dashboardFormatting";
import styles from "../page.module.css";

interface ObservabilityPanelProps {
  snapshot?: ObservabilitySnapshot;
  loading: boolean;
  error?: string;
  onRetry: () => void;
}

interface SparklineProps {
  series: MetricSeries;
}

function Sparkline({ series }: SparklineProps) {
  const path = useMemo(() => {
    const points = series.points ?? [];
    if (points.length < 2) {
      return "";
    }
    const values = points.map((point) => point.value);
    const max = Math.max(...values);
    const min = Math.min(...values);
    const range = max - min || 1;
    const width = 160;
    const height = 60;
    const step = width / (points.length - 1);
    return points
      .map((point, index) => {
        const x = index * step;
        const normalized = (point.value - min) / range;
        const y = height - normalized * height;
        return `${x},${y}`;
      })
      .join(" ");
  }, [series.points]);

  const latest = series.points?.[series.points.length - 1];

  return (
    <div className={styles.sparklineCard}>
      <div className={styles.sparklineHeader}>
        <h4 className={styles.sparklineTitle}>{series.name}</h4>
        <span className={styles.sparklineValue}>
          {latest ? latest.value.toFixed(2) : "—"}
          {series.unit ? ` ${series.unit}` : ""}
        </span>
      </div>
      <svg
        className={styles.sparkline}
        viewBox="0 0 160 60"
        role="img"
        aria-label={`${series.name} trend`}
      >
        <polyline points={path} fill="none" stroke="currentColor" strokeWidth="2" />
      </svg>
    </div>
  );
}

function toTraceSample(event: TraceEvent): TraceSample {
  return {
    traceId: event.traceId,
    spanId: event.spanId,
    name: event.name,
    status: event.status,
    durationMs: event.durationMs,
    service: event.attributes.service as string | undefined,
    startedAt: new Date().toISOString(),
    attributes: Object.fromEntries(
      Object.entries(event.attributes).map(([key, value]) => [key, String(value)]),
    ),
  };
}

export function ObservabilityPanel({
  snapshot,
  loading,
  error,
  onRetry,
}: ObservabilityPanelProps) {
  const [liveTraces, setLiveTraces] = useState<TraceSample[]>(snapshot?.traces ?? []);
  const [liveLogs, setLiveLogs] = useState<LogEntry[]>(snapshot?.logs ?? []);

  useEffect(() => {
    setLiveTraces(snapshot?.traces ?? []);
  }, [snapshot?.traces]);

  useEffect(() => {
    setLiveLogs(snapshot?.logs ?? []);
  }, [snapshot?.logs]);

  useEffect(() => {
    return onTrace((event) => {
      setLiveTraces((previous) => {
        const next = [toTraceSample(event), ...previous];
        return next.slice(0, 12);
      });
    });
  }, []);

  const metrics = snapshot?.metrics ?? [];
  const updatedAt = snapshot?.updatedAt;

  return (
    <DataPanel
      title="Observability overview"
      description="Metrics, traces, and logs collected from the arrgo control plane."
      loading={loading}
      error={error}
      onRetry={onRetry}
      footer={
        updatedAt ? (
          <span className={styles.smallText}>
            Snapshot refreshed {formatRelativeTime(updatedAt)}
          </span>
        ) : null
      }
    >
      {loading ? null : (
        <div className={styles.observabilityGrid}>
          <div className={styles.metricColumn}>
            <h3 className={styles.observabilityTitle}>Metrics</h3>
            {metrics.length ? (
              <div className={styles.sparklineGrid}>
                {metrics.slice(0, 4).map((series) => (
                  <Sparkline key={series.name} series={series} />
                ))}
              </div>
            ) : (
              <p className={styles.emptyState}>No metrics available yet.</p>
            )}
          </div>
          <div className={styles.traceColumn}>
            <h3 className={styles.observabilityTitle}>Recent traces</h3>
            {liveTraces.length ? (
              <ul className={styles.traceList}>
                {liveTraces.slice(0, 8).map((trace) => (
                  <li key={`${trace.traceId}-${trace.spanId}`} className={styles.traceItem}>
                    <div className={styles.traceHeader}>
                      <span className={styles.monospace}>{trace.name}</span>
                      <span className={styles.smallText}>
                        {formatDurationMs(trace.durationMs)}
                      </span>
                    </div>
                    <div className={styles.traceMeta}>
                      <span className={styles.monospace}>{trace.traceId}</span>
                      <StatusPill label={trace.status} tone={logTone(trace.status)} />
                    </div>
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.emptyState}>No traces observed yet.</p>
            )}
          </div>
          <div className={styles.logColumn}>
            <h3 className={styles.observabilityTitle}>Log stream</h3>
            {liveLogs.length ? (
              <ul className={styles.logList}>
                {liveLogs.slice(0, 10).map((log, index) => {
                  const timestamp = log.timestamp ? formatTimestamp(log.timestamp) : "—";
                  return (
                    <li key={`${log.timestamp ?? "unknown"}-${index}`} className={styles.logListItem}>
                      <div className={styles.logListHeader}>
                        <StatusPill label={log.level} tone={logTone(log.level)} />
                        <span className={styles.smallText}>{timestamp}</span>
                      </div>
                      <p className={styles.logListMessage}>{log.message}</p>
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className={styles.emptyState}>No logs recorded for the selected window.</p>
            )}
          </div>
        </div>
      )}
    </DataPanel>
  );
}
