export type Tone = "neutral" | "success" | "warning" | "danger";

export function formatTimestamp(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function formatRelativeTime(value?: string): string {
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

export function formatBytes(value?: number): string {
  if (!value || value <= 0) {
    return "—";
  }
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const precision = size >= 100 || unitIndex === 0 ? 0 : 1;
  return `${size.toFixed(precision)} ${units[unitIndex]}`;
}

export function shortenIdentifier(value: string, size = 12): string {
  if (!value) {
    return "—";
  }
  if (value.length <= size) {
    return value;
  }
  return `${value.slice(0, size)}…`;
}

export function jobTone(state: string): Tone {
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

export function workerTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "healthy":
      return "success";
    case "stale":
      return "warning";
    default:
      return "danger";
  }
}

export function poolTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "healthy":
      return "success";
    case "warning":
      return "warning";
    default:
      return "danger";
  }
}

export function releaseTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "grabbed":
      return "success";
    case "rejected":
      return "danger";
    default:
      return "neutral";
  }
}

export function downloadTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "completed":
    case "imported":
      return "success";
    case "failed":
    case "rejected":
      return "danger";
    case "downloading":
    case "processing":
      return "warning";
    default:
      return "neutral";
  }
}

export function logTone(level: string): Tone {
  switch (level?.toLowerCase()) {
    case "error":
    case "fatal":
    case "panic":
      return "danger";
    case "warn":
    case "warning":
      return "warning";
    case "debug":
    case "trace":
      return "neutral";
    case "info":
    default:
      return "success";
  }
}

export function formatDurationMs(value?: number): string {
  if (!value || value <= 0) {
    return "0 ms";
  }
  if (value < 1000) {
    return `${Math.round(value)} ms`;
  }
  const seconds = value / 1000;
  if (seconds < 60) {
    return `${seconds.toFixed(seconds >= 10 ? 1 : 2)} s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds - minutes * 60;
  if (minutes < 60) {
    const secondsPart = remainingSeconds >= 10
      ? remainingSeconds.toFixed(0)
      : remainingSeconds.toFixed(1);
    return `${minutes}m ${secondsPart}s`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes - hours * 60;
  return `${hours}h ${remainingMinutes}m`;
}
