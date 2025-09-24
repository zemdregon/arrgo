import type {
  QueueAnalyticsOverview,
  QueueHealthStatus,
  QueueRetryMetric,
  QueueSlaIndicator,
  QueueStreamLagMetric,
} from "../../../lib/api";
import type { Tone } from "../../utils/dashboardFormatting";

export type QueueAnalyticsSummary = {
  streamCount: number;
  impactedStreamCount: number;
  maxLagSeconds: number;
  totalLagMessages: number;
  totalRetries: number;
  impactedJobTypes: number;
  slaBreaches: number;
  slaAtRisk: number;
  worstStatus: QueueHealthStatus;
};

const STATUS_SEVERITY: Record<QueueHealthStatus, number> = {
  healthy: 0,
  warning: 1,
  critical: 2,
};

export function buildQueueSummary(
  analytics?: QueueAnalyticsOverview,
): QueueAnalyticsSummary {
  const streamLag = analytics?.streamLag ?? [];
  const retries = analytics?.retries ?? [];
  const sla = analytics?.sla ?? [];

  const streamCount = streamLag.length;
  const impactedStreamCount = streamLag.filter(
    (metric) => metric.status !== "healthy",
  ).length;
  const maxLagSeconds = streamLag.reduce((max, metric) => {
    return Math.max(max, metric.lagSeconds ?? 0);
  }, 0);
  const totalLagMessages = streamLag.reduce((sum, metric) => {
    return sum + (metric.lagMessages ?? 0);
  }, 0);

  const totalRetries = retries.reduce((sum, metric) => {
    return sum + (metric.retryCount ?? 0);
  }, 0);
  const impactedJobTypes = retries.filter((metric) => {
    return metric.status !== "healthy" || (metric.retryCount ?? 0) > 0;
  }).length;

  const slaBreaches = sla.filter((indicator) => {
    return (
      indicator.status === "critical" || (indicator.breachesToday ?? 0) > 0
    );
  }).length;
  const slaAtRisk = sla.filter(
    (indicator) => indicator.status === "warning",
  ).length;

  const worstStatus = determineWorstStatus(streamLag, retries, sla);

  return {
    streamCount,
    impactedStreamCount,
    maxLagSeconds,
    totalLagMessages,
    totalRetries,
    impactedJobTypes,
    slaBreaches,
    slaAtRisk,
    worstStatus,
  };
}

function determineWorstStatus(
  streams: QueueStreamLagMetric[],
  retries: QueueRetryMetric[],
  sla: QueueSlaIndicator[],
): QueueHealthStatus {
  const statuses: QueueHealthStatus[] = ["healthy"];
  for (const stream of streams) {
    statuses.push(stream.status);
  }
  for (const retry of retries) {
    statuses.push(retry.status);
  }
  for (const indicator of sla) {
    statuses.push(indicator.status);
  }

  return statuses.reduce<QueueHealthStatus>((worst, current) => {
    return STATUS_SEVERITY[current] > STATUS_SEVERITY[worst] ? current : worst;
  }, "healthy");
}

export function queueStatusTone(status: QueueHealthStatus): Tone {
  switch (status) {
    case "healthy":
      return "success";
    case "warning":
      return "warning";
    case "critical":
    default:
      return "danger";
  }
}

export function formatDuration(seconds?: number): string {
  if (seconds === undefined || seconds === null || !Number.isFinite(seconds)) {
    return "—";
  }
  if (seconds < 0) {
    return "0s";
  }
  if (seconds > 0 && seconds < 1) {
    return "<1s";
  }

  const totalSeconds = Math.floor(seconds);
  const parts: string[] = [];

  const days = Math.floor(totalSeconds / 86400);
  if (days > 0) {
    parts.push(`${days}d`);
  }

  const hours = Math.floor((totalSeconds % 86400) / 3600);
  if (hours > 0 && parts.length < 2) {
    parts.push(`${hours}h`);
  }

  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (minutes > 0 && parts.length < 2) {
    parts.push(`${minutes}m`);
  }

  const remainingSeconds = totalSeconds % 60;
  if ((remainingSeconds > 0 && parts.length < 2) || parts.length === 0) {
    parts.push(`${remainingSeconds}s`);
  }

  return parts.slice(0, 2).join(" ");
}

export function formatPercentage(value?: number): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "—";
  }
  const formatter = new Intl.NumberFormat(undefined, {
    style: "percent",
    maximumFractionDigits: value > 0.2 ? 0 : 1,
    minimumFractionDigits: 0,
  });
  return formatter.format(Math.min(Math.max(value, 0), 1));
}

export function formatCount(value?: number): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return "0";
  }
  return new Intl.NumberFormat().format(Math.max(0, Math.round(value)));
}

export function formatStatusLabel(status: QueueHealthStatus): string {
  switch (status) {
    case "healthy":
      return "Healthy";
    case "warning":
      return "Warning";
    case "critical":
    default:
      return "Critical";
  }
}

