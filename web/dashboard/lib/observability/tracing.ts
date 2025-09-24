export type TraceAttributeValue = string | number | boolean;

export type TraceAttributes = Record<string, TraceAttributeValue>;

export type TraceStatus = "success" | "error";

export interface TraceEvent {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  name: string;
  startTime: number;
  durationMs: number;
  status: TraceStatus;
  attributes: TraceAttributes;
  error?: string;
}

export interface TraceContext {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  name: string;
  startTime: number;
  attributes: TraceAttributes;
}

export type TraceListener = (event: TraceEvent) => void;

const listeners = new Set<TraceListener>();

function getNow(): number {
  if (typeof performance !== "undefined" && typeof performance.now === "function") {
    return performance.now();
  }
  return Date.now();
}

function randomBytes(length: number): Uint8Array {
  if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
    return crypto.getRandomValues(new Uint8Array(length));
  }
  const array = new Uint8Array(length);
  for (let index = 0; index < length; index += 1) {
    array[index] = Math.floor(Math.random() * 256);
  }
  return array;
}

function randomHex(byteLength: number): string {
  return Array.from(randomBytes(byteLength), (value) => value.toString(16).padStart(2, "0")).join("");
}

function randomTraceId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    const value = crypto.randomUUID().replace(/-/g, "");
    if (value.length >= 32) {
      return value.slice(0, 32);
    }
  }
  return randomHex(16);
}

function randomSpanId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    const value = crypto.randomUUID().replace(/-/g, "");
    if (value.length >= 16) {
      return value.slice(0, 16);
    }
  }
  return randomHex(8);
}

function toErrorMessage(error: unknown): string {
  if (!error) {
    return "";
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return String(error);
}

function publish(event: TraceEvent) {
  for (const listener of listeners) {
    try {
      listener(event);
    } catch (err) {
      // eslint-disable-next-line no-console -- tracing listeners should never break the pipeline
      console.error("trace listener error", err);
    }
  }

  if (typeof window !== "undefined" && typeof window.dispatchEvent === "function") {
    window.dispatchEvent(new CustomEvent("arrgo:trace", { detail: event }));
  }

  if (process.env.NODE_ENV !== "production") {
    const label = `[trace] ${event.name}`;
    if (event.status === "error") {
      // eslint-disable-next-line no-console -- surfaced for local debugging
      console.warn(label, event);
    } else {
      // eslint-disable-next-line no-console -- surfaced for local debugging
      console.debug(label, event);
    }
  }
}

export function onTrace(listener: TraceListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function startClientTrace(
  name: string,
  attributes: TraceAttributes = {},
  parentSpanId?: string,
): TraceContext {
  return {
    traceId: randomTraceId(),
    spanId: randomSpanId(),
    parentSpanId,
    name,
    startTime: getNow(),
    attributes: { ...attributes },
  };
}

export function finishTrace(
  context: TraceContext,
  status: TraceStatus,
  extraAttributes: TraceAttributes = {},
  error?: unknown,
): void {
  const endTime = getNow();
  const duration = Math.max(0, endTime - context.startTime);
  const attributes: TraceAttributes = { ...context.attributes, ...extraAttributes };
  const errorMessage = error ? toErrorMessage(error) : undefined;
  if (errorMessage && !("error" in attributes) && !("errorMessage" in attributes)) {
    attributes.errorMessage = errorMessage;
  }
  publish({
    traceId: context.traceId,
    spanId: context.spanId,
    parentSpanId: context.parentSpanId,
    name: context.name,
    startTime: context.startTime,
    durationMs: duration,
    status,
    attributes,
    error: errorMessage,
  });
}

export function buildTraceHeaders(context: TraceContext): Record<string, string> {
  return {
    traceparent: `00-${context.traceId}-${context.spanId}-01`,
    tracestate: "arrgo=dashboard",
  };
}

export async function runWithTrace<T>(
  name: string,
  attributes: TraceAttributes,
  executor: (context: TraceContext) => Promise<T>,
): Promise<T> {
  const context = startClientTrace(name, attributes);
  try {
    const result = await executor(context);
    finishTrace(context, "success");
    return result;
  } catch (error) {
    finishTrace(context, "error", {}, error);
    throw error;
  }
}
