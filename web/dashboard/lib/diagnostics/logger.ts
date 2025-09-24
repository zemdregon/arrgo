/* eslint-disable no-console -- Structured logging relies on console sinks for local diagnostics */
export type LogLevel = "debug" | "info" | "warn" | "error";

export type LogContext = Record<string, unknown>;

export interface ClientLogEntry {
  timestamp: string;
  level: LogLevel;
  message: string;
  scope?: string;
  context?: LogContext;
  error?: {
    name?: string;
    message?: string;
    stack?: string;
    cause?: unknown;
  };
}

export type LogSink = (entry: ClientLogEntry) => void;

export interface ClientLogger {
  log(level: LogLevel, message: string, context?: LogContext, error?: unknown): void;
  debug(message: string, context?: LogContext): void;
  info(message: string, context?: LogContext): void;
  warn(message: string, context?: LogContext, error?: unknown): void;
  error(message: string, context?: LogContext, error?: unknown): void;
  child(scope?: string, context?: LogContext): ClientLogger;
}

type LoggerConfig = {
  scope?: string;
  context: LogContext;
  sinks: LogSink[];
};

function normalizeError(error: unknown): ClientLogEntry["error"] | undefined {
  if (!error) {
    return undefined;
  }
  if (error instanceof Error) {
    const payload: ClientLogEntry["error"] = {
      name: error.name,
      message: error.message,
    };
    if (error.stack) {
      payload.stack = error.stack;
    }
    if ("cause" in error) {
      payload.cause = (error as Error & { cause?: unknown }).cause;
    }
    return payload;
  }
  if (typeof error === "string") {
    return { message: error };
  }
  if (typeof error === "object") {
    try {
      return JSON.parse(JSON.stringify(error)) as ClientLogEntry["error"];
    } catch (serializationError) {
      return { message: String(error) };
    }
  }
  return { message: String(error) };
}

const consoleSink: LogSink = (entry) => {
  if (typeof console === "undefined") {
    return;
  }
  const method: keyof Console =
    entry.level === "debug"
      ? "debug"
      : entry.level === "info"
        ? "info"
        : entry.level === "warn"
          ? "warn"
          : "error";
  const prefixParts = [
    entry.timestamp,
    `[${entry.level.toUpperCase()}]`,
    entry.scope ? `[${entry.scope}]` : undefined,
    entry.message,
  ].filter(Boolean);
  const prefix = prefixParts.join(" ");
  const payload = {
    context: entry.context,
    error: entry.error,
  };
  if (entry.context || entry.error) {
    (console[method] ?? console.log).call(console, prefix, payload);
  } else {
    (console[method] ?? console.log).call(console, prefix);
  }
};

class StructuredLogger implements ClientLogger {
  private readonly scope?: string;

  private readonly context: LogContext;

  private readonly sinks: LogSink[];

  constructor(config: LoggerConfig) {
    this.scope = config.scope;
    this.context = { ...config.context };
    this.sinks = config.sinks;
  }

  log(level: LogLevel, message: string, context?: LogContext, error?: unknown) {
    const mergedContext = { ...this.context, ...(context ?? {}) };
    const entry: ClientLogEntry = {
      timestamp: new Date().toISOString(),
      level,
      message,
      scope: this.scope,
      context: Object.keys(mergedContext).length ? mergedContext : undefined,
      error: normalizeError(error),
    };

    for (const sink of this.sinks) {
      try {
        sink(entry);
      } catch (sinkError) {
        if (typeof console !== "undefined") {
          console.error("client log sink failure", sinkError);
        }
      }
    }
  }

  debug(message: string, context?: LogContext) {
    this.log("debug", message, context);
  }

  info(message: string, context?: LogContext) {
    this.log("info", message, context);
  }

  warn(message: string, context?: LogContext, error?: unknown) {
    this.log("warn", message, context, error);
  }

  error(message: string, context?: LogContext, error?: unknown) {
    this.log("error", message, context, error);
  }

  child(scope?: string, context?: LogContext): ClientLogger {
    const scopedName = scope
      ? this.scope
        ? `${this.scope}:${scope}`
        : scope
      : this.scope;
    const mergedContext = context
      ? { ...this.context, ...context }
      : { ...this.context };
    return new StructuredLogger({
      scope: scopedName,
      context: mergedContext,
      sinks: this.sinks,
    });
  }
}

export type ClientLoggerOptions = {
  scope?: string;
  context?: LogContext;
  sinks?: LogSink[];
};

export function createClientLogger(options: ClientLoggerOptions = {}): ClientLogger {
  const sinks = options.sinks?.length ? options.sinks : [consoleSink];
  return new StructuredLogger({
    scope: options.scope,
    context: options.context ? { ...options.context } : {},
    sinks,
  });
}
