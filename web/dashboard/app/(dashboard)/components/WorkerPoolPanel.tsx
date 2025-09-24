"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import type {
  RestApiClient,
  WorkerPoolConfig,
  WorkerSummary,
} from "../../../lib/api";
import { buildWorkerPoolInsights } from "../utils/dashboardData";
import {
  formatRelativeTime,
  formatTimestamp,
  poolTone,
  shortenIdentifier,
  workerTone,
} from "../utils/dashboardFormatting";
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

  const poolInsights = useMemo(
    () => buildWorkerPoolInsights(pools, workers),
    [pools, workers],
  );

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

  const selectedPoolInsight = useMemo(() => {
    if (!poolInsights.length) {
      return null;
    }
    return (
      poolInsights.find((insight) => insight.pool === poolSelection) ?? null
    );
  }, [poolInsights, poolSelection]);

  const selectedPoolWorkerCount = selectedPoolInsight?.workerCount ?? 0;

  useEffect(() => {
    if (!poolInsights.length) {
      if (poolSelection !== "") {
        setPoolSelection("");
      }
      return;
    }
    if (
      poolSelection &&
      poolInsights.some((insight) => insight.pool === poolSelection)
    ) {
      return;
    }
    if (pools && pools.length) {
      setPoolSelection(pools[0].pool);
      return;
    }
    setPoolSelection(poolInsights[0].pool);
  }, [poolInsights, pools, poolSelection]);

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
    if (!selectedPoolConfig) {
      setPoolFeedback({
        kind: "error",
        message:
          "The selected pool is not managed by this cluster. Choose a managed pool before saving changes.",
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

  const poolSubmitDisabled =
    poolSubmitting || !canManageWorkers || !selectedPoolConfig;

  const managedSelectionValue = useMemo(() => {
    return poolOptions.some((option) => option.value === poolSelection)
      ? poolSelection
      : "";
  }, [poolOptions, poolSelection]);

  return (
    <DataPanel
      title="Worker pool configuration"
      description="Tune queue assignments and cadence for master/worker pools."
      loading={loading}
      error={error}
      onRetry={onRetry}
      actions={
        poolOptions.length ? (
          <div className={styles.panelControls}>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="pool-select">
                Pool
              </label>
              <select
                id="pool-select"
                className={styles.controlSelect}
                value={managedSelectionValue}
                onChange={(event) => {
                  setPoolSelection(event.target.value);
                  setPoolFeedback(null);
                }}
              >
                <option value="" disabled>
                  Select managed pool
                </option>
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
      {loading ? null : poolInsights.length ? (
        <>
          <div className={styles.poolDirectory}>
            {poolInsights.map((insight) => {
              const isSelected = insight.pool === poolSelection;
              const isManaged = Boolean(
                pools?.some((cfg) => cfg.pool === insight.pool),
              );
              return (
                <button
                  key={insight.pool}
                  type="button"
                  className={[
                    styles.poolCard,
                    isSelected ? styles.poolCardActive : "",
                    !isManaged ? styles.poolCardUnmanaged : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                  onClick={() => {
                    setPoolSelection(insight.pool);
                    setPoolFeedback(null);
                  }}
                >
                  <div className={styles.poolCardHeader}>
                    <div>
                      <span className={styles.poolCardTitle}>{insight.pool}</span>
                      {insight.description ? (
                        <span className={styles.poolCardDescription}>
                          {insight.description}
                        </span>
                      ) : null}
                    </div>
                    <StatusPill
                      label={insight.status}
                      tone={poolTone(insight.status)}
                    />
                  </div>
                  <div className={styles.poolCardMetrics}>
                    <span>
                      {insight.workerCount} {" "}
                      {insight.workerCount === 1 ? "worker" : "workers"}
                    </span>
                    <span>
                      {insight.lastHeartbeatAt
                        ? `Last heartbeat · ${formatRelativeTime(insight.lastHeartbeatAt)}`
                        : "No heartbeats"}
                    </span>
                  </div>
                  {insight.capabilities.length ? (
                    <div className={styles.capabilityGroup}>
                      {insight.capabilities.slice(0, 6).map((capability) => (
                        <span key={capability} className={styles.capabilityTag}>
                          {capability}
                        </span>
                      ))}
                      {insight.capabilities.length > 6 ? (
                        <span className={styles.capabilityTagMuted}>
                          +{insight.capabilities.length - 6} more
                        </span>
                      ) : null}
                    </div>
                  ) : (
                    <p className={styles.poolCardEmpty}>No capability tags</p>
                  )}
                  {!isManaged ? (
                    <p className={styles.poolCardNotice}>Unmanaged pool</p>
                  ) : null}
                </button>
              );
            })}
          </div>
          {selectedPoolInsight ? (
            <>
              <div className={styles.metricsRow}>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{selectedPoolWorkerCount}</span>
                  <span className={styles.metricLabel}>Workers in pool</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {selectedPoolInsight.healthyCount}
                  </span>
                  <span className={styles.metricLabel}>Healthy</span>
                </div>
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {selectedPoolInsight.staleCount}
                  </span>
                  <span className={styles.metricLabel}>Stale</span>
                </div>
                {selectedPoolInsight.unknownCount ? (
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>
                      {selectedPoolInsight.unknownCount}
                    </span>
                    <span className={styles.metricLabel}>Unknown</span>
                  </div>
                ) : null}
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>
                    {selectedPoolInsight.lastHeartbeatAt
                      ? formatRelativeTime(selectedPoolInsight.lastHeartbeatAt)
                      : "—"}
                  </span>
                  <span className={styles.metricLabel}>Last heartbeat</span>
                  {selectedPoolInsight.lastHeartbeatAt ? (
                    <span className={styles.metricDetail}>
                      {formatTimestamp(selectedPoolInsight.lastHeartbeatAt)}
                    </span>
                  ) : null}
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
              <div className={styles.poolInsightSummary}>
                <StatusPill
                  label={selectedPoolInsight.status}
                  tone={poolTone(selectedPoolInsight.status)}
                />
                <span>
                  {selectedPoolInsight.healthyCount} healthy · {selectedPoolInsight.staleCount} stale
                  {selectedPoolInsight.unknownCount
                    ? ` · ${selectedPoolInsight.unknownCount} unknown`
                    : ""}
                </span>
              </div>
              <div className={styles.heartbeatSection}>
                <div className={styles.heartbeatHeader}>
                  <h3 className={styles.heartbeatTitle}>Heartbeat timeline</h3>
                  {selectedPoolInsight.lastHeartbeatAt ? (
                    <span className={styles.smallText}>
                      Updated {formatTimestamp(selectedPoolInsight.lastHeartbeatAt)}
                    </span>
                  ) : null}
                </div>
                {selectedPoolInsight.heartbeatHistory.length ? (
                  <ol className={styles.heartbeatTimeline}>
                    {selectedPoolInsight.heartbeatHistory.map((event) => (
                      <li key={event.workerId} className={styles.timelineItem}>
                        <div className={styles.timelineMarker} aria-hidden="true" />
                        <div className={styles.timelineContent}>
                          <div className={styles.timelineRow}>
                            <span className={styles.monospace} title={event.workerId}>
                              {shortenIdentifier(event.workerId, 10)}
                            </span>
                            <StatusPill
                              label={event.status || "unknown"}
                              tone={workerTone(event.status)}
                            />
                          </div>
                          <div className={styles.timelineMeta}>
                            <span>
                              {event.lastHeartbeatAt
                                ? formatRelativeTime(event.lastHeartbeatAt)
                                : "No heartbeat"}
                            </span>
                            {event.lastHeartbeatAt ? (
                              <span className={styles.smallText}>
                                {formatTimestamp(event.lastHeartbeatAt)}
                              </span>
                            ) : null}
                            {event.version ? (
                              <span className={styles.smallText}>v{event.version}</span>
                            ) : null}
                          </div>
                        </div>
                      </li>
                    ))}
                  </ol>
                ) : (
                  <p className={styles.emptyState}>
                    No heartbeat history recorded for this pool yet.
                  </p>
                )}
              </div>
            </>
          ) : null}
          {pools && pools.length ? (
            selectedPoolConfig ? (
              <form className={styles.poolForm} onSubmit={handlePoolSubmit}>
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
              <p className={styles.poolNotice}>
                Select a managed pool to edit configuration values. Unmanaged pools are
                read-only until they are provisioned on the master.
              </p>
            )
          ) : (
            <p className={styles.emptyState}>
              Define a worker pool to begin tuning queue assignments.
            </p>
          )}
        </>
      ) : (
        <p className={styles.emptyState}>
          No worker pools are reporting yet. Register a worker to populate this view.
        </p>
      )}
    </DataPanel>
  );
}
