"use client";

import { FormEvent, useCallback, useMemo, useState } from "react";
import { useEnvironment } from "../environment/EnvironmentContext";
import { useAuth } from "./AuthContext";
import styles from "./ApiKeySelector.module.css";

type ApiKeySelectorProps = {
  className?: string;
  variant?: "compact" | "inline";
};

function classNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}

function formatTimestamp(value?: string): string {
  if (!value) {
    return "";
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

function relativeTime(value?: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  const diff = Date.now() - date.getTime();
  if (diff < 0) {
    return undefined;
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
  if (days < 14) {
    return `${days}d ago`;
  }
  return formatTimestamp(value);
}

export function ApiKeySelector({
  className,
  variant = "compact",
}: ApiKeySelectorProps) {
  const { current } = useEnvironment();
  const {
    keys,
    activeKey,
    status,
    principal,
    error,
    addKey,
    selectKey,
    removeKey,
    clearError,
    refresh,
  } = useAuth();

  const [open, setOpen] = useState<boolean>(variant === "inline");
  const [label, setLabel] = useState("");
  const [token, setToken] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const environmentLabel = current.label || current.id;

  const handleToggle = useCallback(() => {
    setOpen((previous) => !previous);
  }, []);

  const handleSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      setFormError(null);
      if (!token.trim()) {
        setFormError("API token cannot be empty");
        return;
      }
      const result = addKey({ label, token });
      if (!result.ok) {
        setFormError(result.error ?? "Unable to store API key");
        return;
      }
      setLabel("");
      setToken("");
      if (variant === "compact") {
        setOpen(false);
      }
    },
    [addKey, label, token, variant],
  );

  const handleSelect = useCallback(
    (id: string) => {
      selectKey(id);
      if (variant === "compact") {
        setOpen(false);
      }
    },
    [selectKey, variant],
  );

  const handleRemove = useCallback(
    (id: string) => {
      removeKey(id);
      if (variant === "compact" && keys.length <= 1) {
        setOpen(false);
      }
    },
    [keys.length, removeKey, variant],
  );

  const handleRefresh = useCallback(() => {
    clearError();
    refresh();
  }, [clearError, refresh]);

  const roleLabel = principal?.role.name || activeKey?.role;

  const statusVariant = useMemo(() => {
    switch (status) {
      case "authenticated":
        return "success";
      case "error":
        return "danger";
      case "loading":
        return "warning";
      default:
        return "neutral";
    }
  }, [status]);

  const statusLabel = useMemo(() => {
    switch (status) {
      case "authenticated":
        return roleLabel ? `${roleLabel} access` : "Authenticated";
      case "loading":
        return "Authenticating…";
      case "error":
        return "Authentication failed";
      default:
        return "No API key";
    }
  }, [roleLabel, status]);

  const showPanel = variant === "inline" || open;

  const containerClass = classNames(
    styles.container,
    variant === "inline" && styles.inline,
    className,
  );

  return (
    <div className={containerClass}>
      {variant === "compact" && (
        <button
          type="button"
          className={classNames(styles.toggleButton, styles[`tone-${statusVariant}`])}
          onClick={handleToggle}
        >
          <span className={styles.toggleLabel}>
            {activeKey ? activeKey.label : "Connect API key"}
          </span>
          <span className={styles.statusBadge}>{statusLabel}</span>
        </button>
      )}

      {showPanel && (
        <div
          className={classNames(
            styles.panel,
            variant === "compact" && styles.floating,
          )}
        >
          <div className={styles.panelHeader}>
            <div>
              <span className={styles.panelTitle}>API keys</span>
              <span className={styles.panelSubtitle}>Environment: {environmentLabel}</span>
            </div>
            {variant === "compact" && (
              <button
                type="button"
                className={styles.closeButton}
                onClick={handleToggle}
                aria-label="Close API key manager"
              >
                ×
              </button>
            )}
          </div>

          {error && (
            <div className={styles.errorBanner}>
              <div>{error}</div>
              <button type="button" onClick={handleRefresh} className={styles.retryButton}>
                Retry
              </button>
            </div>
          )}

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Saved keys</h3>
            {keys.length === 0 ? (
              <p className={styles.emptyState}>No API keys saved for this environment.</p>
            ) : (
              <ul className={styles.keyList}>
                {keys.map((key) => {
                  const isActive = activeKey?.id === key.id;
                  return (
                    <li
                      key={key.id}
                      className={classNames(
                        styles.keyItem,
                        isActive && styles.activeKey,
                      )}
                    >
                      <div className={styles.keyMeta}>
                        <div className={styles.keyTitleRow}>
                          <span className={styles.keyLabel}>{key.label}</span>
                          {isActive && <span className={styles.activeBadge}>Active</span>}
                        </div>
                        <div className={styles.keyDetails}>
                          <span className={styles.keyIdLabel}>ID:</span>
                          <span className={styles.keyIdValue}>{key.tokenId ?? key.id}</span>
                        </div>
                        {key.role && (
                          <div className={styles.keyDetails}>
                            <span className={styles.keyIdLabel}>Role:</span>
                            <span className={styles.keyIdValue}>{key.role}</span>
                          </div>
                        )}
                        {key.lastValidatedAt && (
                          <div className={styles.keyDetails}>
                            <span className={styles.keyIdLabel}>Validated:</span>
                            <span className={styles.keyIdValue}>
                              {relativeTime(key.lastValidatedAt) ??
                                formatTimestamp(key.lastValidatedAt)}
                            </span>
                          </div>
                        )}
                      </div>
                      <div className={styles.keyActions}>
                        <button
                          type="button"
                          className={styles.selectButton}
                          onClick={() => handleSelect(key.id)}
                          disabled={isActive || status === "loading"}
                        >
                          Use
                        </button>
                        <button
                          type="button"
                          className={styles.removeButton}
                          onClick={() => handleRemove(key.id)}
                        >
                          Remove
                        </button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Add API key</h3>
            <form className={styles.form} onSubmit={handleSubmit}>
              <label className={styles.field}>
                <span className={styles.fieldLabel}>Label</span>
                <input
                  type="text"
                  value={label}
                  onChange={(event) => setLabel(event.target.value)}
                  placeholder="Dashboard"
                  className={styles.input}
                />
              </label>
              <label className={styles.field}>
                <span className={styles.fieldLabel}>Token</span>
                <input
                  type="text"
                  value={token}
                  onChange={(event) => setToken(event.target.value)}
                  className={styles.input}
                  placeholder="key-id.secret"
                  required
                />
              </label>
              {formError && <p className={styles.formError}>{formError}</p>}
              <button type="submit" className={styles.submitButton}>
                Save API key
              </button>
            </form>
          </div>

          {principal?.permissions && principal.permissions.length > 0 && (
            <div className={styles.section}>
              <h3 className={styles.sectionTitle}>Granted permissions</h3>
              <ul className={styles.permissionList}>
                {principal.permissions.map((permission) => (
                  <li key={permission} className={styles.permissionBadge}>
                    {permission}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
