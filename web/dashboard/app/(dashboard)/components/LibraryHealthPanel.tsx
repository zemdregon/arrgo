"use client";

import Link from "next/link";
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

type LibraryStatus = "all" | "pending" | "quality" | "failures";

const LIBRARY_STATUS_OPTIONS: SelectOption[] = [
  { value: "all", label: "All statuses" },
  { value: "pending", label: "Pending imports" },
  { value: "quality", label: "Quality upgrades" },
  { value: "failures", label: "Failures" },
];

function entityDetailSegment(entityType: string): string | null {
  switch (entityType.toLowerCase()) {
    case "series":
    case "show":
      return "shows";
    case "movie":
    case "film":
      return "movies";
    case "album":
    case "music":
      return "albums";
    case "book":
      return "books";
    default:
      return null;
  }
}

function buildEntityDetailPath(entity: LibraryEntity): string | null {
  const segment = entityDetailSegment(entity.entityType || "");
  if (!segment) {
    return null;
  }
  return `/library/${segment}/${encodeURIComponent(entity.id)}`;
}

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
  const [libraryStatusFilter, setLibraryStatusFilter] =
    useState<LibraryStatus>("all");
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
  const libraryStatusCards = useMemo(
    () => [
      {
        key: "pending" as const,
        label: "Pending imports",
        description: "Queued automation waiting to import media.",
        count: libraryStats.statusTotals.pendingImports,
      },
      {
        key: "quality" as const,
        label: "Quality upgrades",
        description: "Upgrade and refresh jobs that still need attention.",
        count: libraryStats.statusTotals.qualityUpgrades,
      },
      {
        key: "failures" as const,
        label: "Failures",
        description: "Jobs that require operator intervention.",
        count: libraryStats.statusTotals.failures,
      },
    ],
    [
      libraryStats.statusTotals.failures,
      libraryStats.statusTotals.pendingImports,
      libraryStats.statusTotals.qualityUpgrades,
    ],
  );
  const lastFocusedEntityRef = useRef<string | undefined>();
  const handleStatusCardClick = useCallback((status: LibraryStatus) => {
    setLibraryStatusFilter((current) =>
      current === status ? "all" : status,
    );
  }, []);

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
    return entities.filter((entity) => {
      if (
        libraryTypeFilter !== "all" &&
        normalize(entity.entityType) !== libraryTypeFilter
      ) {
        return false;
      }

      switch (libraryStatusFilter) {
        case "pending":
          return (entity.pendingImports ?? 0) > 0;
        case "quality":
          return (entity.qualityUpgrades ?? 0) > 0;
        case "failures":
          return (entity.failures ?? 0) > 0;
        default:
          return true;
      }
    });
  }, [data, libraryStatusFilter, libraryTypeFilter]);

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
  const selectedLibraryStatusLabel =
    libraryStatusFilter === "all"
      ? "All statuses"
      : optionLabel(
          LIBRARY_STATUS_OPTIONS,
          libraryStatusFilter,
          "Selected status",
        );
  const libraryFilterTags = [
    libraryTypeFilter !== "all" ? selectedLibraryLabel : null,
    libraryStatusFilter !== "all" ? selectedLibraryStatusLabel : null,
  ].filter(Boolean);
  const libraryBaseNoun =
    libraryTypeFilter === "all" && libraryStatusFilter === "all"
      ? "entities"
      : "matches";
  const libraryNoun = libraryFilterTags.length
    ? `${libraryBaseNoun} · ${libraryFilterTags.join(" · ")}`
    : libraryBaseNoun;
  const librarySummaryLabel = libraryFilterTags.length
    ? `Filtered · ${libraryFilterTags.join(" · ")}`
    : "Total entities";
  const libraryFooter = libraryStats.total
    ? formatShowingSummary(visibleLibrary.length, 5, libraryNoun, libraryStats.total)
    : undefined;

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
          <div
            className={styles.statusCards}
            role="group"
            aria-label="Library status filters"
          >
            {libraryStatusCards.map((card) => {
              const isActive = libraryStatusFilter === card.key;
              const isEmpty = card.count === 0;
              const title = isActive
                ? "Showing " + card.label.toLowerCase() + ". Click to clear."
                : isEmpty
                  ? "No items currently tracked in this state."
                  : "Filter to " + card.label.toLowerCase();
              return (
                <button
                  key={card.key}
                  type="button"
                  className={styles.statusCard}
                  data-status={card.key}
                  data-active={isActive ? "true" : "false"}
                  data-empty={isEmpty ? "true" : "false"}
                  onClick={() => handleStatusCardClick(card.key)}
                  aria-pressed={isActive}
                  title={title}
                >
                  <span className={styles.statusCardLabel}>{card.label}</span>
                  <span className={styles.statusCardCount}>{card.count}</span>
                  <span className={styles.statusCardDescription}>
                    {card.description}
                  </span>
                </button>
              );
            })}
          </div>
          {visibleLibrary.length ? (
            <>
              <div className={styles.metricsRow}>
                <div
                  className={[
                    styles.metricCard,
                    libraryFilterTags.length ? styles.metricCardEmphasis : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                >
                  <span className={styles.metricValue}>{visibleLibrary.length}</span>
                  <span className={styles.metricLabel}>{librarySummaryLabel}</span>
                </div>
                {libraryFilterTags.length ? (
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
                  const detailPath = buildEntityDetailPath(entity);
                  const title = entity.title || entity.id;
                  const pending = entity.pendingImports ?? 0;
                  const quality = entity.qualityUpgrades ?? 0;
                  const failureCount = entity.failures ?? 0;
                  return (
                    <li
                      key={entity.id}
                      className={styles.entityItem}
                      data-entity-id={entity.id}
                      data-focus={isFocused ? "true" : undefined}
                      tabIndex={isFocused ? -1 : undefined}
                    >
                      <div className={styles.entityInfo}>
                        {detailPath ? (
                          <Link
                            className={[styles.entityTitle, styles.entityLink].join(" ")}
                            href={detailPath}
                          >
                            {title}
                          </Link>
                        ) : (
                          <span className={styles.entityTitle}>{title}</span>
                        )}
                        <span className={styles.entityType}>{entity.entityType}</span>
                      </div>
                      <div className={styles.entityStatusBadges}>
                        {pending ? (
                          <span
                            className={styles.entityStatusBadge}
                            data-status="pending"
                          >
                            Pending {pending}
                          </span>
                        ) : null}
                        {quality ? (
                          <span
                            className={styles.entityStatusBadge}
                            data-status="quality"
                          >
                            Upgrades {quality}
                          </span>
                        ) : null}
                        {failureCount ? (
                          <span
                            className={styles.entityStatusBadge}
                            data-status="failures"
                          >
                            Failures {failureCount}
                          </span>
                        ) : null}
                      </div>
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
          )}
        </>
      ) : (
        <p className={styles.emptyState}>
          No entities have been indexed yet. Once ingestion runs you&apos;ll see
          titles and counts here.
        </p>
      )}
    </DataPanel>
  );
}
