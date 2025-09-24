"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "../../../../components/layout/PageHeader";
import { DataPanel } from "../../../../components/DataPanel";
import { StatusPill } from "../../../../components/StatusPill";
import { useEnvironment } from "../../../../components/environment/EnvironmentContext";
import { useAuth } from "../../../../components/auth/AuthContext";
import { useRestQuery } from "../../../../hooks/useRestQuery";
import { useRestClient } from "../../../../hooks/useRestClient";
import type {
  LibraryEntity,
  ManualSearchRelease,
  ManualSearchResult,
  MetadataOverrides,
  RenameJobRecord,
} from "../../../../lib/api";
import baseStyles from "../../../page.module.css";
import styles from "./page.module.css";

type Tone = "neutral" | "success" | "warning" | "danger";

type FeedbackMessage = { kind: "success" | "error"; message: string };

type EntityPageConfig = {
  label: string;
  pluralLabel: string;
  allowedTypes: string[];
};

const ENTITY_PAGE_CONFIG: Record<string, EntityPageConfig> = {
  shows: { label: "Show", pluralLabel: "Shows", allowedTypes: ["series", "show"] },
  movies: { label: "Movie", pluralLabel: "Movies", allowedTypes: ["movie", "film"] },
  albums: { label: "Album", pluralLabel: "Albums", allowedTypes: ["album", "music"] },
  books: { label: "Book", pluralLabel: "Books", allowedTypes: ["book"] },
};

const JOB_TYPE_OPTIONS = [
  { value: "scan", label: "Scan" },
  { value: "import", label: "Import" },
];

function formatTimestamp(value?: string): string {
  if (!value) {
    return "—";
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

function formatRelativeTime(value?: string): string {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  const diff = Date.now() - date.getTime();
  if (diff < 0) {
    return formatTimestamp(value);
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
  return `${days}d ago`;
}

function formatBytes(value?: number): string {
  if (!value || value <= 0) {
    return "—";
  }
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const precision = size >= 100 || unitIndex === 0 ? 0 : 1;
  return `${size.toFixed(precision)} ${units[unitIndex]}`;
}

function releaseTone(status: string): Tone {
  switch (status?.toLowerCase()) {
    case "grabbed":
      return "success";
    case "rejected":
      return "danger";
    default:
      return "neutral";
  }
}

function shortenIdentifier(value: string, size = 12): string {
  if (!value) {
    return "—";
  }
  if (value.length <= size) {
    return value;
  }
  return `${value.slice(0, size)}…`;
}

function jobExecutionTone(record: RenameJobRecord): Tone {
  return record.dryRun ? "warning" : "success";
}

function jobExecutionLabel(record: RenameJobRecord): string {
  return record.dryRun ? "Dry run" : "Executed";
}

export default function LibraryEntityDetailPage() {
  const params = useParams<{ entityType: string; entityId: string }>();
  const routeSegment = typeof params?.entityType === "string" ? params.entityType.toLowerCase() : "";
  const entityIdParam = typeof params?.entityId === "string" ? params.entityId : "";
  const entityID = decodeURIComponent(entityIdParam).trim();

  const config = ENTITY_PAGE_CONFIG[routeSegment];

  const { current } = useEnvironment();
  const { status: authStatus, hasPermission } = useAuth();
  const queryClient = useQueryClient();
  const restClient = useRestClient();

  const apiBaseUrl = current.apiUrl;
  const isAuthenticated = authStatus === "authenticated";
  const canReadLibrary = isAuthenticated && hasPermission("library:read");
  const canWriteLibrary = isAuthenticated && hasPermission("library:write");

  const libraryQueryKey = useMemo(
    () => ["library", "entities", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const manualSearchKey = useMemo(
    () => ["library", "manual-search", entityID, apiBaseUrl] as const,
    [entityID, apiBaseUrl],
  );
  const metadataQueryKey = useMemo(
    () => ["library", "metadata-overrides", entityID, apiBaseUrl] as const,
    [entityID, apiBaseUrl],
  );
  const historyQueryKey = useMemo(
    () => ["library", "rename-history", entityID, apiBaseUrl] as const,
    [entityID, apiBaseUrl],
  );
  const jobsQueryKey = useMemo(
    () => ["library", "jobs", apiBaseUrl] as const,
    [apiBaseUrl],
  );

  const [jobType, setJobType] = useState(JOB_TYPE_OPTIONS[0].value);
  const [jobPayloadURI, setJobPayloadURI] = useState("");
  const [jobIdempotencyKey, setJobIdempotencyKey] = useState("");
  const [jobSubmitting, setJobSubmitting] = useState(false);
  const [jobFeedback, setJobFeedback] = useState<FeedbackMessage | null>(null);

  const [manualQuery, setManualQuery] = useState("");
  const [manualSearchSubmitting, setManualSearchSubmitting] = useState(false);
  const [manualSearchFeedback, setManualSearchFeedback] = useState<FeedbackMessage | null>(null);
  const [manualSearchResult, setManualSearchResult] = useState<ManualSearchResult | null>(null);
  const [grabInFlight, setGrabInFlight] = useState<string | null>(null);

  const libraryQuery = useRestQuery(libraryQueryKey, (client) => client.fetchEntities(), {
    enabled: canReadLibrary,
    staleTime: 60_000,
  });

  const entity: LibraryEntity | undefined = useMemo(() => {
    const entities = libraryQuery.data ?? [];
    return entities.find((item) => item.id === entityID);
  }, [libraryQuery.data, entityID]);

  const entityTypeMatches = useMemo(() => {
    if (!entity || !config) {
      return true;
    }
    return config.allowedTypes.includes(entity.entityType.toLowerCase());
  }, [entity, config]);

  const metadataQuery = useRestQuery(
    metadataQueryKey,
    (client) => client.fetchMetadataOverrides(entityID),
    {
      enabled: canReadLibrary && Boolean(entityID) && Boolean(entity),
      retry: false,
    },
  );

  const historyQuery = useRestQuery(
    historyQueryKey,
    (client) => client.fetchRenameHistory(entityID, 10),
    {
      enabled: canReadLibrary && Boolean(entityID) && Boolean(entity),
      retry: false,
    },
  );

  const manualSearchQuery = useRestQuery(
    manualSearchKey,
    (client) => client.fetchManualSearch(entityID),
    {
      enabled: canReadLibrary && Boolean(entityID) && Boolean(entity),
      retry: false,
      onSuccess: (result) => {
        setManualSearchResult(result ?? null);
      },
    },
  );

  const metadataOverrides: MetadataOverrides | null = metadataQuery.data ?? null;
  const renameHistory: RenameJobRecord[] = historyQuery.data ?? [];

  const metadataError = metadataQuery.error
    ? metadataQuery.error instanceof Error
      ? metadataQuery.error.message
      : String(metadataQuery.error)
    : undefined;
  const historyError = historyQuery.error
    ? historyQuery.error instanceof Error
      ? historyQuery.error.message
      : String(historyQuery.error)
    : undefined;
  const manualSearchError = manualSearchQuery.error
    ? manualSearchQuery.error instanceof Error
      ? manualSearchQuery.error.message
      : String(manualSearchQuery.error)
    : undefined;

  const manualSearchLoading = manualSearchQuery.isPending && !manualSearchResult;

  useEffect(() => {
    setJobFeedback(null);
    setManualSearchFeedback(null);
    setManualQuery("");
    setManualSearchResult(null);
    setGrabInFlight(null);
  }, [entityID]);

  const handleQueueJob = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!entityID || !entity || !entityTypeMatches) {
      setJobFeedback({ kind: "error", message: "Entity is not available in this environment." });
      return;
    }
    if (!canWriteLibrary) {
      setJobFeedback({
        kind: "error",
        message: "You do not have permission to queue jobs.",
      });
      return;
    }
    setJobSubmitting(true);
    setJobFeedback(null);
    try {
      const job = await restClient.triggerJob({
        jobType,
        targetID: entityID,
        payloadURI: jobPayloadURI,
        idempotencyKey: jobIdempotencyKey,
      });
      const jobTypeLabel =
        JOB_TYPE_OPTIONS.find((option) => option.value === jobType)?.label ?? jobType;
      setJobFeedback({
        kind: "success",
        message: `Queued ${jobTypeLabel} job (${shortenIdentifier(job.id)})`,
      });
      setJobPayloadURI("");
      setJobIdempotencyKey("");
      queryClient.invalidateQueries({ queryKey: jobsQueryKey });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setJobFeedback({ kind: "error", message });
    } finally {
      setJobSubmitting(false);
    }
  };

  const handleManualSearchSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!entityID || !entity || !entityTypeMatches) {
      setManualSearchFeedback({
        kind: "error",
        message: "Entity is not available in this environment.",
      });
      return;
    }
    if (!canWriteLibrary) {
      setManualSearchFeedback({
        kind: "error",
        message: "You do not have permission to run manual searches.",
      });
      return;
    }
    setManualSearchSubmitting(true);
    setManualSearchFeedback(null);
    setGrabInFlight(null);
    try {
      const result = await restClient.runManualSearch(entityID, manualQuery);
      setManualSearchResult(result);
      queryClient.setQueryData(manualSearchKey, result);
      const count = result.results.length;
      setManualSearchFeedback({
        kind: "success",
        message:
          count > 0
            ? `Search returned ${count} release${count === 1 ? "" : "s"}.`
            : "Search completed with no matching releases.",
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setManualSearchFeedback({ kind: "error", message });
    } finally {
      setManualSearchSubmitting(false);
    }
  };

  const handleManualGrab = useCallback(
    async (release: ManualSearchRelease) => {
      if (!entityID || !entity || !entityTypeMatches) {
        setManualSearchFeedback({
          kind: "error",
          message: "Entity is not available in this environment.",
        });
        return;
      }
      if (!canWriteLibrary) {
        setManualSearchFeedback({
          kind: "error",
          message: "You do not have permission to grab releases.",
        });
        return;
      }
      setGrabInFlight(release.id);
      setManualSearchFeedback(null);
      try {
        const updated = await restClient.grabManualRelease(entityID, release.id);
        setManualSearchResult((current) => {
          if (!current) {
            return current;
          }
          return {
            ...current,
            results: current.results.map((item) =>
              item.id === updated.id ? updated : item,
            ),
          };
        });
        queryClient.setQueryData(manualSearchKey, (current: ManualSearchResult | null) => {
          if (!current) {
            return current;
          }
          return {
            ...current,
            results: current.results.map((item) =>
              item.id === updated.id ? updated : item,
            ),
          };
        });
        setManualSearchFeedback({
          kind: "success",
          message: `Queued download for ${updated.title}.`,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setManualSearchFeedback({ kind: "error", message });
      } finally {
        setGrabInFlight(null);
      }
    },
    [canWriteLibrary, entity, entityID, entityTypeMatches, manualSearchKey, queryClient, restClient],
  );

  const pageTitle = entity?.title || entityID || "Unknown entity";
  const description = config
    ? `Inspect metadata, rename history, and manual controls for this ${config.label.toLowerCase()}.`
    : "This library view is only available for shows, movies, albums, and books.";

  const metadataLoading = libraryQuery.isPending || metadataQuery.isPending;
  const historyLoading = historyQuery.isPending;

  const manualSearchResults = manualSearchResult?.results ?? [];
  const overridesCount = metadataOverrides
    ? metadataOverrides.episodeCutovers.length +
      metadataOverrides.albumEditions.length +
      metadataOverrides.trackNumberOverrides.length
    : 0;

  return (
    <div className={styles.page}>
      <PageHeader
        eyebrow={config ? `${config.label} detail` : "Library detail"}
        title={pageTitle}
        description={description}
        actions={
          <Link className={styles.headerLink} href="/">
            Back to overview
          </Link>
        }
      />
      {!config ? (
        <DataPanel title="Unsupported entity type">
          <p className={styles.emptyState}>
            Detail views are available for shows, movies, albums, and books.
          </p>
        </DataPanel>
      ) : !canReadLibrary ? (
        <DataPanel title="Library access">
          <p className={styles.emptyState}>
            Requires <code>library:read</code> permission to view entity details.
          </p>
        </DataPanel>
      ) : !entityID ? (
        <DataPanel title="Entity not specified">
          <p className={styles.emptyState}>
            Provide an entity identifier to load detail information.
          </p>
        </DataPanel>
      ) : (
        <div className={styles.grid}>
          <DataPanel
            title="Metadata"
            description={`Catalog context for this ${config.label.toLowerCase()}.`}
            loading={metadataLoading}
            error={metadataError}
            onRetry={() => {
              metadataQuery.refetch();
              libraryQuery.refetch();
            }}
          >
            {!entity ? (
              <p className={styles.emptyState}>
                This entity is not registered in the selected environment.
              </p>
            ) : !entityTypeMatches ? (
              <p className={styles.emptyState}>
                This entity is stored as “{entity.entityType}”, which does not match the
                {" "}
                {config.label.toLowerCase()} view.
              </p>
            ) : (
              <div className={styles.metadataSection}>
                <ul className={styles.metadataList}>
                  <li className={styles.metadataItem}>
                    <span className={styles.metadataLabel}>Entity ID</span>
                    <span className={`${styles.metadataValue} ${styles.code}`}>
                      {entity.id}
                    </span>
                  </li>
                  <li className={styles.metadataItem}>
                    <span className={styles.metadataLabel}>Entity type</span>
                    <span className={styles.metadataValue}>{entity.entityType}</span>
                  </li>
                  <li className={styles.metadataItem}>
                    <span className={styles.metadataLabel}>Synopsis</span>
                    <span className={styles.synopsis}>
                      {entity.synopsis?.trim() ? entity.synopsis : "No synopsis available."}
                    </span>
                  </li>
                </ul>
                <div>
                  <h3 className={styles.overrideTitle}>Metadata overrides</h3>
                  {metadataOverrides && overridesCount > 0 ? (
                    <div className={styles.overrideGroup}>
                      {metadataOverrides.episodeCutovers.length ? (
                        <section className={styles.overrideSection}>
                          <h4 className={styles.overrideTitle}>Episode cutovers</h4>
                          <ul className={styles.overrideList}>
                            {metadataOverrides.episodeCutovers.map((override) => (
                              <li key={override.episodeID} className={styles.overrideItem}>
                                <div className={styles.overrideItemHeader}>
                                  <span>
                                    S{override.seasonNumber.toString().padStart(2, "0")}E
                                    {override.episodeNumber.toString().padStart(2, "0")} ·
                                    {" "}
                                    {override.title || override.episodeID}
                                  </span>
                                  {override.absoluteNumber ? (
                                    <span className={styles.overrideItemMeta}>
                                      Abs {override.absoluteNumber}
                                    </span>
                                  ) : null}
                                </div>
                                <div className={styles.overrideItemMeta}>
                                  <span>ID {override.episodeID}</span>
                                  {override.airDate ? (
                                    <span>Air date {formatTimestamp(override.airDate)}</span>
                                  ) : null}
                                </div>
                                {override.notes ? (
                                  <p className={styles.overrideNotes}>{override.notes}</p>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        </section>
                      ) : null}
                      {metadataOverrides.albumEditions.length ? (
                        <section className={styles.overrideSection}>
                          <h4 className={styles.overrideTitle}>Album editions</h4>
                          <ul className={styles.overrideList}>
                            {metadataOverrides.albumEditions.map((edition) => (
                              <li key={edition.editionID} className={styles.overrideItem}>
                                <div className={styles.overrideItemHeader}>
                                  <span>{edition.name || edition.editionID}</span>
                                  {edition.isDefault ? <span>Default</span> : null}
                                </div>
                                <div className={styles.overrideItemMeta}>
                                  <span>ID {edition.editionID}</span>
                                  {edition.releaseDate ? (
                                    <span>
                                      Release {formatTimestamp(edition.releaseDate)}
                                    </span>
                                  ) : null}
                                </div>
                                {edition.notes ? (
                                  <p className={styles.overrideNotes}>{edition.notes}</p>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        </section>
                      ) : null}
                      {metadataOverrides.trackNumberOverrides.length ? (
                        <section className={styles.overrideSection}>
                          <h4 className={styles.overrideTitle}>Track overrides</h4>
                          <ul className={styles.overrideList}>
                            {metadataOverrides.trackNumberOverrides.map((track) => (
                              <li key={track.trackID} className={styles.overrideItem}>
                                <div className={styles.overrideItemHeader}>
                                  <span>Track {track.trackNumber}</span>
                                  {track.discNumber ? <span>Disc {track.discNumber}</span> : null}
                                </div>
                                <div className={styles.overrideItemMeta}>
                                  <span>ID {track.trackID}</span>
                                  {track.totalTracks ? <span>Total {track.totalTracks}</span> : null}
                                  {track.isrc ? <span>ISRC {track.isrc}</span> : null}
                                </div>
                                {track.notes ? (
                                  <p className={styles.overrideNotes}>{track.notes}</p>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        </section>
                      ) : null}
                    </div>
                  ) : (
                    <p className={styles.emptyState}>
                      No metadata overrides have been configured for this entity.
                    </p>
                  )}
                  {metadataOverrides?.updatedAt ? (
                    <p className={styles.metadataUpdated}>
                      Updated {formatTimestamp(metadataOverrides.updatedAt)}
                    </p>
                  ) : null}
                </div>
              </div>
            )}
          </DataPanel>

          <DataPanel
            title="Manual operations"
            description="Queue automation jobs or override downloads for this entity."
            loading={manualSearchLoading || manualSearchSubmitting}
            error={manualSearchError}
            onRetry={() => manualSearchQuery.refetch()}
          >
            {!entity || !entityTypeMatches ? (
              <p className={styles.emptyState}>
                This entity is not available for manual operations in the selected environment.
              </p>
            ) : (
              <div className={styles.manualSection}>
                <form
                  className={[baseStyles.panelControls, baseStyles.inlineForm].join(" ")}
                  onSubmit={handleQueueJob}
                >
                  <div className={baseStyles.controlGroup}>
                    <label className={baseStyles.controlLabel} htmlFor="job-type">
                      Job type
                    </label>
                    <select
                      id="job-type"
                      className={baseStyles.controlSelect}
                      value={jobType}
                      onChange={(event) => {
                        setJobType(event.target.value);
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
                  <div className={baseStyles.controlGroup}>
                    <label className={baseStyles.controlLabel} htmlFor="job-payload">
                      Payload URI
                    </label>
                    <input
                      id="job-payload"
                      className={baseStyles.controlInput}
                      type="text"
                      value={jobPayloadURI}
                      onChange={(event) => setJobPayloadURI(event.target.value)}
                      placeholder="Optional"
                      autoComplete="off"
                    />
                  </div>
                  <div className={baseStyles.controlGroup}>
                    <label
                      className={baseStyles.controlLabel}
                      htmlFor="job-idempotency"
                    >
                      Idempotency key
                    </label>
                    <input
                      id="job-idempotency"
                      className={baseStyles.controlInput}
                      type="text"
                      value={jobIdempotencyKey}
                      onChange={(event) => setJobIdempotencyKey(event.target.value)}
                      placeholder="Optional"
                      autoComplete="off"
                    />
                  </div>
                  <button
                    type="submit"
                    className={baseStyles.primaryButton}
                    disabled={jobSubmitting || !canWriteLibrary}
                  >
                    {jobSubmitting ? "Queueing…" : "Queue job"}
                  </button>
                  {!canWriteLibrary ? (
                    <p className={styles.emptyState}>
                      Requires <code>library:write</code> permission to queue jobs.
                    </p>
                  ) : null}
                </form>
                {jobFeedback ? (
                  <div
                    className={[
                      baseStyles.formMessage,
                      jobFeedback.kind === "success"
                        ? baseStyles.formMessageSuccess
                        : baseStyles.formMessageError,
                    ].join(" ")}
                    role={jobFeedback.kind === "error" ? "alert" : "status"}
                  >
                    <span>{jobFeedback.message}</span>
                    <button
                      type="button"
                      className={baseStyles.messageDismiss}
                      onClick={() => setJobFeedback(null)}
                    >
                      Dismiss
                    </button>
                  </div>
                ) : null}
                <form
                  className={[baseStyles.panelControls, baseStyles.manualSearchForm].join(" ")}
                  onSubmit={handleManualSearchSubmit}
                >
                  <div className={baseStyles.controlGroup}>
                    <label
                      className={baseStyles.controlLabel}
                      htmlFor="manual-search-query"
                    >
                      Search query
                    </label>
                    <input
                      id="manual-search-query"
                      className={baseStyles.controlInput}
                      type="text"
                      value={manualQuery}
                      onChange={(event) => setManualQuery(event.target.value)}
                      placeholder="Optional keywords"
                      autoComplete="off"
                    />
                  </div>
                  <button
                    type="submit"
                    className={baseStyles.primaryButton}
                    disabled={manualSearchSubmitting || !canWriteLibrary}
                  >
                    {manualSearchSubmitting ? "Searching…" : "Search indexers"}
                  </button>
                  {!canWriteLibrary ? (
                    <p className={styles.emptyState}>
                      Requires <code>library:write</code> permission to run manual searches.
                    </p>
                  ) : null}
                </form>
                {manualSearchFeedback ? (
                  <div
                    className={[
                      baseStyles.formMessage,
                      manualSearchFeedback.kind === "success"
                        ? baseStyles.formMessageSuccess
                        : baseStyles.formMessageError,
                    ].join(" ")}
                    role={manualSearchFeedback.kind === "error" ? "alert" : "status"}
                  >
                    <span>{manualSearchFeedback.message}</span>
                    <button
                      type="button"
                      className={baseStyles.messageDismiss}
                      onClick={() => setManualSearchFeedback(null)}
                    >
                      Dismiss
                    </button>
                  </div>
                ) : null}
                <div>
                  <div className={styles.manualSummary}>
                    <span>
                      {manualSearchResult?.executedAt
                        ? `Last search ${formatRelativeTime(manualSearchResult.executedAt)}`
                        : "Run a manual search to review release candidates."}
                    </span>
                    <button
                      type="button"
                      className={baseStyles.refreshButton}
                      onClick={() => manualSearchQuery.refetch()}
                      disabled={manualSearchQuery.isFetching}
                    >
                      {manualSearchQuery.isFetching ? "Refreshing…" : "Refresh"}
                    </button>
                  </div>
                  {manualSearchResults.length ? (
                    <ul className={styles.manualResultList}>
                      {manualSearchResults.map((release) => (
                        <li key={release.id} className={styles.manualResultItem}>
                          <div className={styles.manualResultHeader}>
                            <div>
                              <h4 className={styles.manualResultTitle}>{release.title}</h4>
                              <div className={styles.manualResultMeta}>
                                <span>{release.indexer}</span>
                                <span>{release.protocol}</span>
                                <span>{release.quality}</span>
                                <span>{formatBytes(release.sizeBytes)}</span>
                                <span>Seeders {release.seeders}</span>
                                <span>Score {release.score.toFixed(1)}</span>
                              </div>
                            </div>
                            <StatusPill label={release.status} tone={releaseTone(release.status)} />
                          </div>
                          <div className={styles.manualResultBody}>
                            <div className={styles.manualResultActions}>
                              {release.status.toLowerCase() === "grabbed" ? (
                                <span>
                                  Grabbed {formatRelativeTime(release.grabbedAt)}
                                </span>
                              ) : (
                                <button
                                  type="button"
                                  className={baseStyles.primaryButton}
                                  onClick={() => handleManualGrab(release)}
                                  disabled={grabInFlight === release.id || !canWriteLibrary}
                                >
                                  {grabInFlight === release.id ? "Grabbing…" : "Grab release"}
                                </button>
                              )}
                              <a
                                className={styles.manualActionLink}
                                href={release.downloadURL}
                                target="_blank"
                                rel="noreferrer"
                              >
                                Open download link
                              </a>
                            </div>
                            {release.rejectionReason ? (
                              <p className={styles.manualResultRejection}>
                                {release.rejectionReason}
                              </p>
                            ) : null}
                          </div>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className={styles.emptyState}>
                      Run a manual search to review releases for this entity.
                    </p>
                  )}
                </div>
              </div>
            )}
          </DataPanel>

          <DataPanel
            title="Rename history"
            description="Review the most recent rename operations recorded for this entity."
            loading={historyLoading}
            error={historyError}
            onRetry={() => historyQuery.refetch()}
          >
            {!entity ? (
              <p className={styles.emptyState}>
                This entity is not registered in the selected environment.
              </p>
            ) : renameHistory.length === 0 ? (
              <p className={styles.emptyState}>
                No rename jobs have been recorded for this entity yet.
              </p>
            ) : (
              <ul className={styles.historyList}>
                {renameHistory.map((record) => (
                  <li key={record.id} className={styles.historyItem}>
                    <div className={styles.historyHeader}>
                      <div>
                        <h4 className={styles.historyHeading}>
                          Job {shortenIdentifier(record.id)}
                        </h4>
                        <p className={styles.historyDetails}>
                          Requested {formatTimestamp(record.requestedAt)}
                        </p>
                      </div>
                      <div className={styles.historyBadges}>
                        <StatusPill
                          label={jobExecutionLabel(record)}
                          tone={jobExecutionTone(record)}
                        />
                        <span className={styles.historyChanges}>
                          {record.changes} change{record.changes === 1 ? "" : "s"}
                        </span>
                      </div>
                    </div>
                    <ul className={styles.operationList}>
                      {record.operations.slice(0, 4).map((operation) => {
                        const operationLabel = operation.shouldRename ? "Renamed" : "Skipped";
                        const operationTone: Tone = operation.shouldRename
                          ? "success"
                          : operation.reason
                          ? "warning"
                          : "neutral";
                        return (
                          <li key={`${record.id}-${operation.fileID}`} className={styles.operationItem}>
                            <div className={styles.operationMeta}>
                              <StatusPill label={operationLabel} tone={operationTone} />
                              <span>{operation.workType}</span>
                              {operation.lastRenamedAt ? (
                                <span>Last renamed {formatRelativeTime(operation.lastRenamedAt)}</span>
                              ) : null}
                            </div>
                            <div className={styles.operationPaths}>
                              {operation.sourcePath}
                              {operation.targetPath
                                ? ` → ${operation.targetPath}`
                                : ""}
                            </div>
                            {operation.reason ? (
                              <p className={styles.operationReason}>{operation.reason}</p>
                            ) : null}
                          </li>
                        );
                      })}
                    </ul>
                    {record.operations.length > 4 ? (
                      <p className={styles.historyMore}>
                        +{record.operations.length - 4} additional operation
                        {record.operations.length - 4 === 1 ? "" : "s"}
                      </p>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </DataPanel>
        </div>
      )}
    </div>
  );
}
