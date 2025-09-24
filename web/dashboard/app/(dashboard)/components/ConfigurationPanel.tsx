"use client";

import { useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import type {
  ConfigurationSummary,
  EnvironmentVariableSummary,
  SecretPointer,
} from "../../../lib/api";
import styles from "../page.module.css";

interface ConfigurationPanelProps {
  summary?: ConfigurationSummary;
  loading: boolean;
  error?: string;
  onRetry: () => void;
}

function maskValue(value: string, secret?: boolean, revealed?: boolean) {
  if (!secret || revealed) {
    return value || "—";
  }
  return "••••••";
}

export function ConfigurationPanel({
  summary,
  loading,
  error,
  onRetry,
}: ConfigurationPanelProps) {
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const secrets = summary?.secrets ?? [];
  const documentationUrl = summary?.documentationUrl;
  const lastUpdated = summary?.lastUpdatedAt;

  const sortedEnvironment = useMemo(() => {
    const list = summary?.environment ?? [];
    return [...list].sort((a, b) => a.key.localeCompare(b.key));
  }, [summary?.environment]);

  const handleReveal = (key: string) => {
    setRevealed((previous) => ({ ...previous, [key]: !previous[key] }));
  };

  const handleCopy = async (variable: EnvironmentVariableSummary) => {
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        await navigator.clipboard.writeText(variable.value ?? "");
        setCopiedKey(variable.key);
        setTimeout(() => setCopiedKey(null), 2000);
      }
    } catch (copyError) {
      // eslint-disable-next-line no-console -- provide basic feedback when copy fails
      console.warn("Failed to copy value", copyError);
    }
  };

  return (
    <DataPanel
      title="Configuration"
      description="Pointers to secret stores and environment variables powering this deployment."
      loading={loading}
      error={error}
      onRetry={onRetry}
      footer={
        lastUpdated ? (
          <span className={styles.smallText}>
            Configuration updated {new Date(lastUpdated).toLocaleString()}
          </span>
        ) : null
      }
    >
      {loading ? null : (
        <div className={styles.configurationGrid}>
          <div className={styles.configurationColumn}>
            <h3 className={styles.observabilityTitle}>Secrets</h3>
            {secrets.length ? (
              <ul className={styles.secretList}>
                {secrets.map((secret: SecretPointer) => (
                  <li key={secret.name} className={styles.secretItem}>
                    <div className={styles.secretHeader}>
                      <span className={styles.secretName}>{secret.name}</span>
                      <span className={styles.secretProvider}>{secret.provider}</span>
                    </div>
                    <div className={styles.secretMeta}>
                      <span className={styles.monospace}>{secret.path}</span>
                      {secret.status ? (
                        <span className={styles.smallText}>{secret.status}</span>
                      ) : null}
                    </div>
                    {secret.lastRotatedAt ? (
                      <span className={styles.smallText}>
                        Last rotated {new Date(secret.lastRotatedAt).toLocaleString()}
                      </span>
                    ) : null}
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.emptyState}>No secret pointers configured.</p>
            )}
            {documentationUrl ? (
              <a
                className={styles.linkButton}
                href={documentationUrl}
                target="_blank"
                rel="noreferrer"
              >
                View secrets management guide
              </a>
            ) : null}
          </div>
          <div className={styles.configurationColumn}>
            <h3 className={styles.observabilityTitle}>Environment</h3>
            {sortedEnvironment.length ? (
              <ul className={styles.environmentList}>
                {sortedEnvironment.map((variable: EnvironmentVariableSummary) => {
                  const isSecret = variable.secret ?? false;
                  const isRevealed = revealed[variable.key];
                  const displayValue = maskValue(variable.value ?? "", isSecret, isRevealed);
                  return (
                    <li key={variable.key} className={styles.environmentItem}>
                      <div className={styles.environmentHeader}>
                        <span className={styles.environmentKey}>{variable.key}</span>
                        <span className={styles.environmentScope}>{variable.scope}</span>
                      </div>
                      {variable.description ? (
                        <p className={styles.environmentDescription}>{variable.description}</p>
                      ) : null}
                      <div className={styles.environmentValueRow}>
                        <code className={styles.environmentValue}>{displayValue}</code>
                        <div className={styles.environmentActions}>
                          {isSecret ? (
                            <button
                              type="button"
                              className={styles.secondaryButton}
                              onClick={() => handleReveal(variable.key)}
                            >
                              {isRevealed ? "Hide" : "Reveal"}
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className={styles.secondaryButton}
                            onClick={() => handleCopy(variable)}
                          >
                            {copiedKey === variable.key ? "Copied" : "Copy"}
                          </button>
                        </div>
                      </div>
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className={styles.emptyState}>No environment variables defined.</p>
            )}
          </div>
        </div>
      )}
    </DataPanel>
  );
}
