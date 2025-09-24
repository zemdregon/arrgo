"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import type {
  RestApiClient,
  WorkerPoolConfig,
  WorkerSummary,
} from "../../../lib/api";
import styles from "../page.module.css";

interface WorkerPoolPanelProps {
  pools?: WorkerPoolConfig[];
  workers?: WorkerSummary[];
  loading: boolean;
  error?: string;
  onRetry: () => void;
  restClient: RestApiClient;
  canManageWorkers: boolean;
  onPoolsRefetch: () => void;
}

export function WorkerPoolPanel({
  pools,
  workers,
  loading,
  error,
  onRetry,
  restClient,
  canManageWorkers,
  onPoolsRefetch,
}: WorkerPoolPanelProps) {
  const [poolSelection, setPoolSelection] = useState("");
  const [poolForm, setPoolForm] = useState({
    description: "",
    stream: "",
    consumerGroup: "",
    heartbeatIntervalSeconds: "",
    staleAfterSeconds: "",
    maxConcurrentJobs: "",
  });
  const [poolSubmitting, setPoolSubmitting] = useState(false);
  const [poolFeedback, setPoolFeedback] = useState<
    | {
        kind: "success" | "error";
        message: string;
      }
    | null
  >(null);

  const poolOptions = useMemo(
    () =>
      (pools ?? []).map((pool) => ({
        value: pool.pool,
        label: pool.description
          ? pool.pool + " · " + pool.description
          : pool.pool,
      })),
    [pools],
  );

  const selectedPoolConfig = useMemo(() => {
    if (!pools) {
      return null;
    }
    return pools.find((cfg) => cfg.pool === poolSelection) ?? null;
  }, [pools, poolSelection]);

  const selectedPoolWorkerCount = useMemo(() => {
    if (!workers) {
      return 0;
    }
    return workers.filter((worker) => worker.pool === poolSelection).length;
  }, [workers, poolSelection]);

  useEffect(() => {
    if (!pools || pools.length === 0) {
      if (poolSelection !== "") {
        setPoolSelection("");
      }
      return;
    }
    if (poolSelection && pools.some((cfg) => cfg.pool === poolSelection)) {
      return;
    }
    setPoolSelection(pools[0].pool);
  }, [pools, poolSelection]);

  useEffect(() => {
    if (!selectedPoolConfig) {
      setPoolForm({
        description: "",
        stream: "",
        consumerGroup: "",
        heartbeatIntervalSeconds: "",
        staleAfterSeconds: "",
        maxConcurrentJobs: "",
      });
      return;
    }
    setPoolForm({
      description: selectedPoolConfig.description ?? "",
      stream: selectedPoolConfig.stream,
      consumerGroup: selectedPoolConfig.consumerGroup,
      heartbeatIntervalSeconds: String(selectedPoolConfig.heartbeatIntervalSeconds || 0),
      staleAfterSeconds: String(selectedPoolConfig.staleAfterSeconds || 0),
      maxConcurrentJobs:
        selectedPoolConfig.maxConcurrentJobs !== undefined
          ? String(selectedPoolConfig.maxConcurrentJobs)
          : "",
    });
    setPoolFeedback(null);
  }, [selectedPoolConfig]);

  const parseOptionalNumber = (value: string): number | undefined => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    const parsed = Number(trimmed);
    if (!Number.isFinite(parsed)) {
      return undefined;
    }
    return parsed;
  };

  const handlePoolSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canManageWorkers) {
      setPoolFeedback({
        kind: "error",
        message: "You do not have permission to update worker pools.",
      });
      return;
    }
    if (!poolSelection) {
      setPoolFeedback({
        kind: "error",
        message: "Select a worker pool to update before saving changes.",
      });
      return;
    }
    if (!poolForm.stream.trim() || !poolForm.consumerGroup.trim()) {
      setPoolFeedback({
        kind: "error",
        message: "Stream and consumer group values are required.",
      });
      return;
    }

    const heartbeatInterval = parseOptionalNumber(poolForm.heartbeatIntervalSeconds);
    const staleAfter = parseOptionalNumber(poolForm.staleAfterSeconds);
    const maxConcurrentJobs = parseOptionalNumber(poolForm.maxConcurrentJobs);

    setPoolSubmitting(true);
    setPoolFeedback(null);
    try {
      await restClient.updateWorkerPool(poolSelection, {
        description: poolForm.description,
        stream: poolForm.stream,
        consumerGroup: poolForm.consumerGroup,
        heartbeatIntervalSeconds: heartbeatInterval,
        staleAfterSeconds: staleAfter,
        maxConcurrentJobs,
      });
      setPoolFeedback({
        kind: "success",
        message: "Updated " + poolSelection + " pool configuration.",
      });
      onPoolsRefetch();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setPoolFeedback({ kind: "error", message });
    } finally {
      setPoolSubmitting(false);
    }
  };

  const poolSubmitDisabled = poolSubmitting || !canManageWorkers;

  return (
    <DataPanel
      title="Worker pool configuration"
      description="Tune queue assignments and cadence for master/worker pools."
      loading={loading}
      error={error}
      onRetry={onRetry}
      actions={
        pools && pools.length ? (
          <div className={styles.panelControls}>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-select">
                Pool
              </label>
              <select
                id="pool-select"
                className={styles.controlSelect}
                value={poolSelection}
                onChange={(event) => {
                  setPoolSelection(event.target.value);
                  setPoolFeedback(null);
                }}
              >
                {poolOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
          </div>
        ) : null
      }
    >
      {loading ? null : pools && pools.length ? (
        <form className={styles.poolForm} onSubmit={handlePoolSubmit}>
          <div className={styles.metricsRow}>
            <div className={styles.metricCard}>
              <span className={styles.metricValue}>{selectedPoolWorkerCount}</span>
              <span className={styles.metricLabel}>Workers in pool</span>
            </div>
            <div className={styles.metricCard}>
              <span className={styles.metricValue}>
                {selectedPoolConfig
                  ? String(selectedPoolConfig.heartbeatIntervalSeconds) + "s"
                  : "—"}
              </span>
              <span className={styles.metricLabel}>Heartbeat interval</span>
            </div>
            <div className={styles.metricCard}>
              <span className={styles.metricValue}>
                {selectedPoolConfig
                  ? String(selectedPoolConfig.staleAfterSeconds) + "s"
                  : "—"}
              </span>
              <span className={styles.metricLabel}>Stale threshold</span>
            </div>
            {selectedPoolConfig?.maxConcurrentJobs !== undefined ? (
              <div className={styles.metricCard}>
                <span className={styles.metricValue}>
                  {selectedPoolConfig.maxConcurrentJobs}
                </span>
                <span className={styles.metricLabel}>Max concurrent jobs</span>
              </div>
            ) : null}
          </div>
          <div className={styles.poolFormGrid}>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-description">
                Description
              </label>
              <input
                id="pool-description"
                className={styles.controlInput}
                value={poolForm.description}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    description: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
                placeholder="Optional summary"
              />
            </div>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-stream">
                Stream
              </label>
              <input
                id="pool-stream"
                className={styles.controlInput}
                value={poolForm.stream}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    stream: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
                required
              />
            </div>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-consumer-group">
                Consumer group
              </label>
              <input
                id="pool-consumer-group"
                className={styles.controlInput}
                value={poolForm.consumerGroup}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    consumerGroup: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
                required
              />
            </div>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-heartbeat-interval">
                Heartbeat interval (s)
              </label>
              <input
                id="pool-heartbeat-interval"
                type="number"
                min={0}
                className={styles.controlInput}
                value={poolForm.heartbeatIntervalSeconds}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    heartbeatIntervalSeconds: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
              />
            </div>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-stale-after">
                Stale after (s)
              </label>
              <input
                id="pool-stale-after"
                type="number"
                min={0}
                className={styles.controlInput}
                value={poolForm.staleAfterSeconds}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    staleAfterSeconds: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
              />
            </div>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-max-jobs">
                Max concurrent jobs
              </label>
              <input
                id="pool-max-jobs"
                type="number"
                min={0}
                className={styles.controlInput}
                value={poolForm.maxConcurrentJobs}
                onChange={(event) => {
                  setPoolForm((previous) => ({
                    ...previous,
                    maxConcurrentJobs: event.target.value,
                  }));
                  setPoolFeedback(null);
                }}
                placeholder="Optional"
              />
            </div>
          </div>
          {poolFeedback ? (
            <div
              className={[
                styles.formMessage,
                poolFeedback.kind === "success"
                  ? styles.formMessageSuccess
                  : styles.formMessageError,
              ].join(" ")}
            >
              <span>{poolFeedback.message}</span>
              <button
                type="button"
                className={styles.messageDismiss}
                onClick={() => setPoolFeedback(null)}
              >
                Dismiss
              </button>
            </div>
          ) : null}
          <button
            type="submit"
            className={styles.primaryButton}
            disabled={poolSubmitDisabled}
          >
            Save configuration
          </button>
          {!canManageWorkers ? (
            <p className={styles.permissionNotice}>
              Requires <code>worker:manage</code> permission to edit worker pools.
            </p>
          ) : null}
        </form>
      ) : (
        <p className={styles.emptyState}>
          Define a worker pool to begin tuning queue assignments.
        </p>
      )}
    </DataPanel>
  );
}
