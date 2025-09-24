"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { DataPanel } from "../../../components/DataPanel";
import type { RestApiClient, LibraryEntity } from "../../../lib/api";
import {
  buildLibraryStats,
  buildOptions,
  createEntityOptions,
  formatShowingSummary,
  normalize,
  optionLabel,
  type SelectOption,
} from "../utils/dashboardData";
import styles from "../page.module.css";

const JOB_TYPE_OPTIONS: SelectOption[] = [
  { value: "scan", label: "Scan" },
  { value: "import", label: "Import" },
];

interface LibraryHealthPanelProps {
  data?: LibraryEntity[];
  loading: boolean;
  error?: string;
  onRetry: () => void;
  restClient: RestApiClient;
  canWriteLibrary: boolean;
  onJobsRefetch: () => void;
  focusedEntityId?: string;
}

export function LibraryHealthPanel({
  data,
  loading,
  error,
  onRetry,
  restClient,
  canWriteLibrary,
  onJobsRefetch,
  focusedEntityId,
}: LibraryHealthPanelProps) {
  const [libraryTypeFilter, setLibraryTypeFilter] = useState("all");
  const [jobTypeSelection, setJobTypeSelection] = useState(
    JOB_TYPE_OPTIONS[0]?.value ?? "scan",
  );
  const [jobTargetSelection, setJobTargetSelection] = useState("");
  const [jobPayloadURI, setJobPayloadURI] = useState("");
  const [jobIdempotencyKey, setJobIdempotencyKey] = useState("");
  const [jobSubmitting, setJobSubmitting] = useState(false);
  const [jobFeedback, setJobFeedback] = useState<
    | {
        kind: "success" | "error";
        message: string;
      }
    | null
  >(null);

  const libraryEntityOptions = useMemo(
    () => createEntityOptions(data),
    [data],
  );
  const libraryTypeOptions = useMemo(
    () =>
      buildOptions(
        (data ?? []).map((entity) => entity.entityType),
        "Unknown",
        "unknown",
      ),
    [data],
  );
  const libraryStats = useMemo(() => buildLibraryStats(data), [data]);
  const lastFocusedEntityRef = useRef<string | undefined>();

  useEffect(() => {
    if (
      libraryTypeFilter !== "all" &&
      !libraryTypeOptions.some((option) => option.value === libraryTypeFilter)
    ) {
      setLibraryTypeFilter("all");
    }
  }, [libraryTypeFilter, libraryTypeOptions]);

  useEffect(() => {
    if (!libraryEntityOptions.length) {
      setJobTargetSelection("");
      return;
    }
    if (
      jobTargetSelection &&
      libraryEntityOptions.some((option) => option.value === jobTargetSelection)
    ) {
      return;
    }
    setJobTargetSelection(libraryEntityOptions[0].value);
  }, [jobTargetSelection, libraryEntityOptions]);

  useEffect(() => {
    if (!focusedEntityId) {
      lastFocusedEntityRef.current = undefined;
      return;
    }
    if (lastFocusedEntityRef.current === focusedEntityId) {
      return;
    }
    const match = libraryEntityOptions.find(
      (option) => option.value === focusedEntityId,
    );
    if (!match) {
      return;
    }
    lastFocusedEntityRef.current = focusedEntityId;
    setJobTargetSelection(focusedEntityId);
  }, [focusedEntityId, libraryEntityOptions]);

  const visibleLibrary = useMemo(() => {
    const entities = data ?? [];
    if (libraryTypeFilter === "all") {
      return entities;
    }
    return entities.filter(
      (entity) => normalize(entity.entityType) === libraryTypeFilter,
    );
  }, [data, libraryTypeFilter]);

  const libraryFooter = libraryStats.total
    ? formatShowingSummary(
        visibleLibrary.length,
        5,
        libraryTypeFilter === "all" ? "entities" : "matches",
        libraryStats.total,
      )
    : undefined;

  const jobSubmitDisabled =
    !canWriteLibrary ||
    !libraryEntityOptions.length ||
    !jobTargetSelection ||
    jobSubmitting;

  const handleTriggerJob = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (!canWriteLibrary) {
        setJobFeedback({
          kind: "error",
          message: "You do not have permission to queue jobs.",
        });
        return;
      }
      if (!jobTargetSelection) {
        setJobFeedback({
          kind: "error",
          message: "Select a target entity before queueing a job.",
        });
        return;
      }

      const jobTypeValue = jobTypeSelection;
      const jobTargetValue = jobTargetSelection;
      const jobTargetOption = libraryEntityOptions.find(
        (option) => option.value === jobTargetValue,
      );
      const jobTargetDescription = jobTargetOption
        ? jobTargetOption.label + " · " + jobTargetOption.entityType
        : jobTargetValue;
      const jobTypeLabel =
        JOB_TYPE_OPTIONS.find((option) => option.value === jobTypeValue)?.label ??
        jobTypeValue;

      setJobSubmitting(true);
      setJobFeedback(null);
      try {
        await restClient.triggerJob({
          jobType: jobTypeValue,
          targetID: jobTargetValue,
          payloadURI: jobPayloadURI,
          idempotencyKey: jobIdempotencyKey,
        });
        setJobFeedback({
          kind: "success",
          message:
            "Queued " + jobTypeLabel + " job for " + jobTargetDescription + ".",
        });
        setJobPayloadURI("");
        setJobIdempotencyKey("");
        onJobsRefetch();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setJobFeedback({ kind: "error", message });
      } finally {
        setJobSubmitting(false);
      }
    },
    [
      canWriteLibrary,
      jobIdempotencyKey,
      jobPayloadURI,
      jobTargetSelection,
      jobTypeSelection,
      libraryEntityOptions,
      onJobsRefetch,
      restClient,
    ],
  );

  const selectedLibraryLabel =
    libraryTypeFilter === "all"
      ? "All types"
      : optionLabel(libraryTypeOptions, libraryTypeFilter, "Selected type");

  return (
    <DataPanel
      title="Library health"
      description="Snapshot of tracked media entities grouped by type."
      loading={loading}
      error={error}
      onRetry={onRetry}
      actions={
        libraryStats.total ? (
          <>
            <div className={styles.panelControls}>
              <div className={styles.controlGroup}>
                <label
                  className={styles.controlLabel}
                  htmlFor="library-type-filter"
                >
                  Entity type
                </label>
                <select
                  id="library-type-filter"
                  className={styles.controlSelect}
                  value={libraryTypeFilter}
                  onChange={(event) => setLibraryTypeFilter(event.target.value)}
                >
                  <option value="all">All types</option>
                  {libraryTypeOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            {libraryEntityOptions.length ? (
              <form
                className={[styles.panelControls, styles.inlineForm].join(" ")}
                onSubmit={handleTriggerJob}
              >
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="job-type-select"
                  >
                    Job type
                  </label>
                  <select
                    id="job-type-select"
                    className={styles.controlSelect}
                    value={jobTypeSelection}
                    onChange={(event) => {
                      setJobTypeSelection(event.target.value);
                      setJobFeedback(null);
                    }}
                  >
                    {JOB_TYPE_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="job-target-select"
                  >
                    Target entity
                  </label>
                  <select
                    id="job-target-select"
                    className={styles.controlSelect}
                    value={jobTargetSelection}
                    onChange={(event) => {
                      setJobTargetSelection(event.target.value);
                      setJobFeedback(null);
                    }}
                  >
                    {libraryEntityOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label} · {option.entityType}
                      </option>
                    ))}
                  </select>
                </div>
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="job-payload-input"
                  >
                    Payload URI
                  </label>
                  <input
                    id="job-payload-input"
                    className={styles.controlInput}
                    type="text"
                    value={jobPayloadURI}
                    onChange={(event) => {
                      setJobPayloadURI(event.target.value);
                      setJobFeedback(null);
                    }}
                    placeholder="Optional"
                    autoComplete="off"
                  />
                </div>
                <div className={styles.controlGroup}>
                  <label
                    className={styles.controlLabel}
                    htmlFor="job-idempotency-input"
                  >
                    Idempotency key
                  </label>
                  <input
                    id="job-idempotency-input"
                    className={styles.controlInput}
                    type="text"
                    value={jobIdempotencyKey}
                    onChange={(event) => {
                      setJobIdempotencyKey(event.target.value);
                      setJobFeedback(null);
                    }}
                    placeholder="Optional"
                    autoComplete="off"
                  />
                </div>
                <button
                  type="submit"
                  className={styles.primaryButton}
                  disabled={jobSubmitDisabled}
                >
                  {jobSubmitting ? "Queueing…" : "Queue job"}
                </button>
              </form>
            ) : null}
            {!canWriteLibrary ? (
              <p className={styles.permissionNotice}>
                Requires <code>library:write</code> permission to queue jobs.
              </p>
            ) : null}
          </>
        ) : null
      }
      footer={libraryFooter}
    >
      {libraryStats.total ? (
        visibleLibrary.length ? (
          <>
            {jobFeedback ? (
              <div
                className={[
                  styles.formMessage,
                  jobFeedback.kind === "success"
                    ? styles.formMessageSuccess
                    : styles.formMessageError,
                ].join(" ")}
                role={jobFeedback.kind === "error" ? "alert" : "status"}
                aria-live={jobFeedback.kind === "error" ? "assertive" : "polite"}
              >
                <span>{jobFeedback.message}</span>
                <button
                  type="button"
                  className={styles.messageDismiss}
                  onClick={() => setJobFeedback(null)}
                >
                  Dismiss
                </button>
              </div>
            ) : null}
            <div className={styles.metricsRow}>
              <div
                className={[
                  styles.metricCard,
                  libraryTypeFilter !== "all" ? styles.metricCardEmphasis : "",
                ]
                  .filter(Boolean)
                  .join(" ")}
              >
                <span className={styles.metricValue}>{visibleLibrary.length}</span>
                <span className={styles.metricLabel}>
                  {libraryTypeFilter === "all"
                    ? "Total entities"
                    : "Filtered · " + selectedLibraryLabel}
                </span>
              </div>
              {libraryTypeFilter !== "all" ? (
                <div className={styles.metricCard}>
                  <span className={styles.metricValue}>{libraryStats.total}</span>
                  <span className={styles.metricLabel}>Cluster total</span>
                </div>
              ) : null}
              {libraryStats.breakdown.map(([type, count]) => {
                const isActive =
                  libraryTypeFilter !== "all" &&
                  normalize(type) === libraryTypeFilter;
                return (
                  <div
                    key={type}
                    className={[
                      styles.metricCard,
                      isActive ? styles.metricCardActive : "",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                  >
                    <span className={styles.metricValue}>{count}</span>
                    <span className={styles.metricLabel}>{type}</span>
                  </div>
                );
              })}
            </div>
            <ul className={styles.entityList}>
              {visibleLibrary.slice(0, 5).map((entity) => {
                const isFocused = entity.id === focusedEntityId;
                return (
                  <li
                    key={entity.id}
                    className={styles.entityItem}
                    data-entity-id={entity.id}
                    data-focus={isFocused ? "true" : undefined}
                    tabIndex={isFocused ? -1 : undefined}
                  >
                  <span className={styles.entityTitle}>
                    {entity.title || entity.id}
                  </span>
                  <span className={styles.entityType}>{entity.entityType}</span>
                  </li>
                );
              })}
            </ul>
          </>
        ) : (
          <p className={styles.emptyState}>
            No entities match the selected filter yet. Adjust the filters to
            explore other media types.
          </p>
        )
      ) : (
        <p className={styles.emptyState}>
          No entities have been indexed yet. Once ingestion runs you&apos;ll see
          titles and counts here.
        </p>
      )}
    </DataPanel>
  );
}
