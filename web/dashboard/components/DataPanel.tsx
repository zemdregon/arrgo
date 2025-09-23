import styles from "./DataPanel.module.css";

interface DataPanelProps {
  title: string;
  description?: string;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
  children?: React.ReactNode;
  footer?: React.ReactNode;
}

export function DataPanel({
  title,
  description,
  loading = false,
  error,
  onRetry,
  children,
  footer
}: DataPanelProps) {
  return (
    <section className={styles.panel} aria-busy={loading}>
      <header className={styles.header}>
        <div>
          <h2 className={styles.title}>{title}</h2>
          {description ? (
            <p className={styles.description}>{description}</p>
          ) : null}
        </div>
        {onRetry ? (
          <button
            type="button"
            className={styles.refreshButton}
            onClick={onRetry}
            disabled={loading}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        ) : null}
      </header>
      {error ? (
        <div className={styles.error} role="alert">
          <span>{error}</span>
          {onRetry ? (
            <button
              type="button"
              className={styles.inlineRetry}
              onClick={onRetry}
              disabled={loading}
            >
              Try again
            </button>
          ) : null}
        </div>
      ) : loading ? (
        <div className={styles.loading}>Loading…</div>
      ) : (
        <div className={styles.content}>{children}</div>
      )}
      {footer ? <footer className={styles.footer}>{footer}</footer> : null}
    </section>
  );
}
