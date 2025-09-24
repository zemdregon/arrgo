"use client";

import {
  createContext,
  PropsWithChildren,
  useContext,
  useEffect,
  useMemo,
} from "react";
import { useEnvironment } from "../environment/EnvironmentContext";
import {
  ClientLogger,
  ClientLoggerOptions,
  createClientLogger,
} from "../../lib/diagnostics/logger";
import { ToastProvider } from "../feedback/ToastProvider";
import { ToastViewport } from "../feedback/ToastViewport";

type ClientLoggerContextValue = ClientLogger;

const ClientLoggerContext = createContext<ClientLoggerContextValue | undefined>(
  undefined,
);

function buildLoggerOptions({
  environmentId,
  environmentLabel,
  apiUrl,
}: {
  environmentId: string;
  environmentLabel: string;
  apiUrl: string;
}): ClientLoggerOptions {
  return {
    scope: "dashboard",
    context: {
      environmentId,
      environmentLabel,
      environmentApiUrl: apiUrl,
    },
  };
}

export function DiagnosticsProvider({ children }: PropsWithChildren) {
  const { current } = useEnvironment();

  const logger = useMemo(() => {
    return createClientLogger(
      buildLoggerOptions({
        environmentId: current.id,
        environmentLabel: current.label,
        apiUrl: current.apiUrl,
      }),
    );
  }, [current.apiUrl, current.id, current.label]);

  useEffect(() => {
    logger.info("Environment context initialized", {
      environmentId: current.id,
      environmentLabel: current.label,
      environmentApiUrl: current.apiUrl,
    });
  }, [current.apiUrl, current.id, current.label, logger]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const handleError = (event: ErrorEvent) => {
      logger.error(
        "Unhandled error event",
        {
          filename: event.filename,
          line: event.lineno,
          column: event.colno,
        },
        event.error ?? event.message,
      );
    };

    const handleRejection = (event: PromiseRejectionEvent) => {
      logger.error(
        "Unhandled promise rejection",
        {
          reasonType: typeof event.reason,
        },
        event.reason,
      );
    };

    window.addEventListener("error", handleError);
    window.addEventListener("unhandledrejection", handleRejection);

    return () => {
      window.removeEventListener("error", handleError);
      window.removeEventListener("unhandledrejection", handleRejection);
    };
  }, [logger]);

  return (
    <ClientLoggerContext.Provider value={logger}>
      <ToastProvider>
        {children}
        <ToastViewport />
      </ToastProvider>
    </ClientLoggerContext.Provider>
  );
}

export function useClientLogger(
  scope?: string,
  context?: Record<string, unknown>,
): ClientLogger {
  const logger = useContext(ClientLoggerContext);
  if (!logger) {
    throw new Error("useClientLogger must be used within a DiagnosticsProvider");
  }
  if (!scope && !context) {
    return logger;
  }
  return logger.child(scope, context);
}
