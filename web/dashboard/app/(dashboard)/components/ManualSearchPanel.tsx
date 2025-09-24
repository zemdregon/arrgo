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
import { StatusPill } from "../../../components/StatusPill";
import type {
  LibraryEntity,
  ManualSearchRelease,
  ManualSearchResult,
  RestApiClient,
} from "../../../lib/api";
import { createEntityOptions } from "../utils/dashboardData";
import {
  formatBytes,
  formatRelativeTime,
  formatTimestamp,
  releaseTone,
  shortenIdentifier,
} from "../utils/dashboardFormatting";
import styles from "../page.module.css";

interface ManualSearchPanelProps {
  data?: LibraryEntity[];
  restClient: RestApiClient;
  canWriteLibrary: boolean;
  focusedEntityId?: string;
}

export function ManualSearchPanel({
  data,
  restClient,
  canWriteLibrary,
  focusedEntityId,
}: ManualSearchPanelProps) {
  const libraryEntityOptions = useMemo(
    () => createEntityOptions(data),
    [data],
  );
  const [manualEntitySelection, setManualEntitySelection] = useState("");
  const [manualQuery, setManualQuery] = useState("");
  const [manualSearchResult, setManualSearchResult] =
    useState<ManualSearchResult | null>(null);
  const [manualSearchLoading, setManualSearchLoading] = useState(false);
  const [manualSearchError, setManualSearchError] = useState<string | null>(
    null,
  );
  const [manualSearchFeedback, setManualSearchFeedback] = useState<
    | {
        kind: "success" | "error";
        message: string;
      }
    | null
  >(null);
  const [grabInFlight, setGrabInFlight] = useState<string | null>(null);

  useEffect(() => {
    if (!libraryEntityOptions.length) {
      setManualEntitySelection("");
      setManualSearchResult(null);
      return;
    }
    if (
      manualEntitySelection &&
      libraryEntityOptions.some(
        (option) => option.value === manualEntitySelection,
      )
    ) {
      return;
    }
    setManualEntitySelection(libraryEntityOptions[0].value);
  }, [libraryEntityOptions, manualEntitySelection]);

  const lastFocusedEntityRef = useRef<string | undefined>();

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
    setManualEntitySelection(focusedEntityId);
  }, [focusedEntityId, libraryEntityOptions]);

  const loadManualSearch = useCallback(() => {
    setGrabInFlight(null);
    if (!manualEntitySelection) {
      setManualSearchResult(null);
      setManualSearchError(null);
      setManualSearchLoading(false);
      return Promise.resolve();
    }
    setManualSearchLoading(true);
    setManualSearchError(null);
    return restClient
      .fetchManualSearch(manualEntitySelection)
      .then((result) => {
        setManualSearchResult(result);
      })
      .catch((error) => {
        const message = error instanceof Error ? error.message : String(error);
        setManualSearchError(message);
        setManualSearchResult(null);
      })
      .finally(() => {
        setManualSearchLoading(false);
      });
  }, [manualEntitySelection, restClient]);

  useEffect(() => {
    setManualSearchFeedback(null);
    void loadManualSearch();
  }, [loadManualSearch]);

  const manualSearchSubmitDisabled =
    manualSearchLoading ||
    !manualEntitySelection ||
    !libraryEntityOptions.length ||
    !canWriteLibrary;

  const manualSearchResults = manualSearchResult?.results ?? [];
  const manualSearchExecutedAt = manualSearchResult?.executedAt;

  const handleManualSearchSubmit = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (!canWriteLibrary) {
        setManualSearchFeedback({
          kind: "error",
          message: "You do not have permission to run manual searches.",
        });
        return;
      }
      if (!manualEntitySelection) {
        setManualSearchFeedback({
          kind: "error",
          message: "Select an entity before running a manual search.",
        });
        return;
      }
      setManualSearchFeedback(null);
      setManualSearchLoading(true);
      setGrabInFlight(null);
      try {
        const result = await restClient.runManualSearch(
          manualEntitySelection,
          manualQuery,
        );
        setManualSearchResult(result);
        const count = result.results.length;
        setManualSearchFeedback({
          kind: "success",
          message:
            count > 0
              ? `Search returned ${count} release${count === 1 ? "" : "s"}.`
              : "Search completed with no matching releases.",
        });
        setManualSearchError(null);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setManualSearchFeedback({ kind: "error", message });
      } finally {
        setManualSearchLoading(false);
      }
    },
    [canWriteLibrary, manualEntitySelection, manualQuery, restClient],
  );

  const handleManualGrab = useCallback(
    async (release: ManualSearchRelease) => {
      if (!manualEntitySelection) {
        return;
      }
      if (!canWriteLibrary) {
        setManualSearchFeedback({
          kind: "error",
          message: "You do not have permission to grab releases.",
        });
        return;
      }
      setManualSearchFeedback(null);
      setGrabInFlight(release.id);
      try {
        const updated = await restClient.grabManualRelease(
          manualEntitySelection,
          release.id,
        );
        setManualSearchResult((current) => {
          if (!current) {
            return current;
          }
          return {
            ...current,
            results: current.results.map((candidate) =>
              candidate.id === updated.id ? updated : candidate,
            ),
          };
        });
        setManualSearchFeedback({
          kind: "success",
          message: `Marked "${release.title}" as grabbed.`,
        });
        setManualSearchError(null);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setManualSearchFeedback({ kind: "error", message });
      } finally {
        setGrabInFlight(null);
      }
    },
    [canWriteLibrary, manualEntitySelection, restClient],
  );

  return (
    <DataPanel
      title="Manual indexer search"
      description="Review indexer results and override automation when needed."
      loading={manualSearchLoading && !manualSearchResult}
      error={manualSearchError ?? undefined}
      onRetry={loadManualSearch}
      actions={
        libraryEntityOptions.length ? (
          <div className={styles.panelControls}>
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="manual-search-entity">
                Entity
              </label>
              <select
                id="manual-search-entity"
                className={styles.controlSelect}
                value={manualEntitySelection}
                onChange={(event) => {
                  setManualEntitySelection(event.target.value);
                  setManualSearchFeedback(null);
                  setGrabInFlight(null);
                }}
              >
                {libraryEntityOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label} · {option.entityType}
                  </option>
                ))}
              </select>
            </div>
          </div>
        ) : null
      }
    >
      {libraryEntityOptions.length ? (
        <>
          <form
            className={[styles.panelControls, styles.manualSearchForm].join(" ")}
            onSubmit={handleManualSearchSubmit}
          >
            <div className={styles.controlGroup}>
              <label className={styles.controlLabel} htmlFor="manual-search-query">
                Search query
              </label>
              <input
                id="manual-search-query"
                className={styles.controlInput}
                type="text"
                value={manualQuery}
                onChange={(event) => setManualQuery(event.target.value)}
                placeholder="Optional keywords"
                autoComplete="off"
              />
            </div>
            <button
              type="submit"
              className={styles.primaryButton}
              disabled={manualSearchSubmitDisabled}
            >
              {manualSearchLoading ? "Searching…" : "Search indexers"}
            </button>
            {!canWriteLibrary ? (
              <p className={styles.permissionNotice}>
                Requires <code>library:write</code> permission to run manual
                searches.
              </p>
            ) : null}
          </form>
          {manualSearchFeedback ? (
            <div
              className={[
                styles.formMessage,
                manualSearchFeedback.kind === "success"
                  ? styles.formMessageSuccess
                  : styles.formMessageError,
              ].join(" ")}
              role={manualSearchFeedback.kind === "error" ? "alert" : "status"}
              aria-live={
                manualSearchFeedback.kind === "error" ? "assertive" : "polite"
              }
            >
              <span>{manualSearchFeedback.message}</span>
              <button
                type="button"
                className={styles.messageDismiss}
                onClick={() => setManualSearchFeedback(null)}
              >
                Dismiss
              </button>
            </div>
          ) : null}
          {manualSearchResult ? (
            <>
              <div className={styles.searchMeta}>
                <span>
                  {manualSearchExecutedAt
                    ? [
                        formatRelativeTime(manualSearchExecutedAt),
                        formatTimestamp(manualSearchExecutedAt),
                      ].join(" · ")
                    : "Search not yet run"}
                </span>
                {manualSearchResult.query ? (
                  <span>Query “{manualSearchResult.query}”</span>
                ) : null}
                <span>
                  {manualSearchResults.length === 1
                    ? "1 result"
                    : `${manualSearchResults.length} results`}
                </span>
              </div>
              {manualSearchResults.length ? (
                <div className={styles.tableWrapper}>
                  <table className={styles.table}>
                    <thead>
                      <tr>
                        <th scope="col">Release</th>
                        <th scope="col">Indexer</th>
                        <th scope="col">Quality</th>
                        <th scope="col">Size</th>
                        <th scope="col">Published</th>
                        <th scope="col">Score</th>
                        <th scope="col">Status</th>
                        <th scope="col">Action</th>
                      </tr>
                    </thead>
                    <tbody>
                      {manualSearchResults.map((release) => {
                        const status = release.status || "unknown";
                        const isGrabbed = status.toLowerCase() === "grabbed";
                        const isRejected = status.toLowerCase() === "rejected";
                        const downloadURL = release.downloadURL?.trim();
                        return (
                          <tr key={release.id}>
                            <td>
                              <div className={styles.jobDetails}>
                                <span className={styles.releaseTitle}>
                                  {release.title}
                                </span>
                                {downloadURL ? (
                                  <a
                                    href={downloadURL}
                                    target="_blank"
                                    rel="noreferrer"
                                    className={styles.smallText}
                                    title={downloadURL}
                                  >
                                    {shortenIdentifier(downloadURL, 48)}
                                  </a>
                                ) : null}
                              </div>
                            </td>
                            <td>{release.indexer}</td>
                            <td>{release.quality || "—"}</td>
                            <td>{formatBytes(release.sizeBytes)}</td>
                            <td>
                              {release.publishDate ? (
                                <span className={styles.smallText}>
                                  {[
                                    formatRelativeTime(release.publishDate),
                                    formatTimestamp(release.publishDate),
                                  ].join(" · ")}
                                </span>
                              ) : (
                                "—"
                              )}
                            </td>
                            <td>
                              {Number.isFinite(release.score)
                                ? release.score.toFixed(1)
                                : "—"}
                            </td>
                            <td>
                              <div className={styles.releaseStatus}>
                                <StatusPill
                                  label={status}
                                  tone={releaseTone(status)}
                                />
                                {isRejected && release.rejectionReason ? (
                                  <span className={styles.rejectionNote}>
                                    {release.rejectionReason}
                                  </span>
                                ) : null}
                              </div>
                            </td>
                            <td>
                              {isGrabbed ? (
                                <span className={styles.smallText}>
                                  {release.grabbedAt
                                    ? [
                                        formatRelativeTime(release.grabbedAt),
                                        formatTimestamp(release.grabbedAt),
                                      ].join(" · ")
                                    : "Queued"}
                                </span>
                              ) : (
                                <button
                                  type="button"
                                  className={styles.secondaryButton}
                                  onClick={() => handleManualGrab(release)}
                                  disabled={
                                    grabInFlight === release.id ||
                                    manualSearchLoading ||
                                    !canWriteLibrary
                                  }
                                  title={
                                    !canWriteLibrary
                                      ? "Requires library:write permission"
                                      : undefined
                                  }
                                >
                                  {grabInFlight === release.id
                                    ? "Grabbing…"
                                    : "Grab"}
                                </button>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className={styles.emptyState}>
                  Search completed with no matching releases. Try refining the
                  query or selecting a different entity.
                </p>
              )}
            </>
          ) : (
            <p className={styles.emptyState}>
              Run a manual search to review candidate releases for the selected
              entity.
            </p>
          )}
        </>
      ) : (
        <p className={styles.emptyState}>
          Add library entities to enable manual search workflows.
        </p>
      )}
    </DataPanel>
  );
}
