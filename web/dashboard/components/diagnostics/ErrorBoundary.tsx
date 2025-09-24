"use client";

import {
  Component,
  ErrorInfo,
  PropsWithChildren,
  ReactNode,
  useCallback,
} from "react";
import { useClientLogger } from "./DiagnosticsProvider";
import { useToast } from "../feedback/ToastProvider";

type ErrorBoundaryFallbackProps = {
  error: Error;
  reset: () => void;
};

type ErrorBoundaryProps = PropsWithChildren<{
  fallback?: ReactNode | ((props: ErrorBoundaryFallbackProps) => ReactNode);
  onReset?: () => void;
}>;

type ErrorBoundaryState = {
  hasError: boolean;
  error?: Error;
};

class ErrorBoundaryInner extends Component<
  PropsWithChildren<{
    fallback?: ErrorBoundaryProps["fallback"];
    onError?: (error: Error, info: ErrorInfo) => void;
    onReset?: () => void;
  }>,
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = {
    hasError: false,
    error: undefined,
  };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.props.onError?.(error, info);
  }

  private resetBoundary = () => {
    this.setState({ hasError: false, error: undefined });
    this.props.onReset?.();
  };

  render(): ReactNode {
    const { fallback, children } = this.props;
    const { hasError, error } = this.state;

    if (!hasError) {
      return children;
    }

    const resolvedError = error ?? new Error("Unknown error");
    if (typeof fallback === "function") {
      return fallback({ error: resolvedError, reset: this.resetBoundary });
    }
    if (fallback) {
      return fallback;
    }

    return <DefaultErrorFallback error={resolvedError} onRetry={this.resetBoundary} />;
  }
}

function DefaultErrorFallback({
  error,
  onRetry,
}: {
  error: Error;
  onRetry: () => void;
}) {
  const handleReload = () => {
    if (typeof window !== "undefined") {
      window.location.reload();
    }
  };

  return (
    <div
      role="alert"
      style={{
        margin: "var(--space-xl) auto",
        maxWidth: "560px",
        background: "var(--surface-overlay)",
        color: "var(--color-text-primary)",
        borderRadius: "var(--radius-xl)",
        border: "1px solid var(--color-border-strong)",
        boxShadow: "var(--shadow-md)",
        padding: "var(--space-xl)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-md)",
      }}
    >
      <div>
        <h2
          style={{
            margin: 0,
            fontSize: "var(--font-size-xl)",
            fontWeight: 600,
          }}
        >
          Something went wrong
        </h2>
        <p
          style={{
            margin: 0,
            color: "var(--color-text-muted)",
            fontSize: "var(--font-size-sm)",
          }}
        >
          We encountered a rendering error. You can try again or reload the page
          if the issue persists.
        </p>
      </div>
      <code
        style={{
          display: "block",
          padding: "var(--space-sm)",
          background: "var(--surface-panel)",
          borderRadius: "var(--radius-md)",
          fontFamily: "var(--font-mono)",
          fontSize: "var(--font-size-sm)",
          overflowX: "auto",
        }}
      >
        {error.message}
      </code>
      <div style={{ display: "flex", gap: "var(--space-sm)" }}>
        <button
          type="button"
          onClick={onRetry}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            borderRadius: "var(--radius-lg)",
            border: "1px solid var(--color-border-subtle)",
            background: "var(--surface-panel-strong)",
            color: "var(--color-text-primary)",
            cursor: "pointer",
            fontWeight: 600,
            transition: "transform var(--transition-base)",
          }}
        >
          Try again
        </button>
        <button
          type="button"
          onClick={handleReload}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            borderRadius: "var(--radius-lg)",
            border: "1px solid var(--color-border-subtle)",
            background: "transparent",
            color: "var(--color-text-primary)",
            cursor: "pointer",
          }}
        >
          Reload page
        </button>
      </div>
    </div>
  );
}

export function ErrorBoundary({
  children,
  fallback,
  onReset,
}: ErrorBoundaryProps) {
  const logger = useClientLogger("error-boundary");
  const { push } = useToast();

  const handleError = useCallback(
    (error: Error, info: ErrorInfo) => {
      logger.error(
        "UI error boundary captured an exception",
        {
          componentStack: info.componentStack,
        },
        error,
      );
      push({
        variant: "error",
        title: "Unexpected error",
        description: "We've recorded the issue. Try again or reload the page.",
        duration: 8000,
      });
    },
    [logger, push],
  );

  const handleReset = useCallback(() => {
    logger.info("Error boundary reset triggered");
    onReset?.();
  }, [logger, onReset]);

  return (
    <ErrorBoundaryInner
      fallback={fallback}
      onError={handleError}
      onReset={handleReset}
    >
      {children}
    </ErrorBoundaryInner>
  );
}
