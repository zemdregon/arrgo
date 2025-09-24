"use client";

import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import type {
  DownloadHistoryEntry,
  MetadataComparison,
  MetadataSourceSummary,
  ReadingList,
} from "../../../lib/api";
import {
  downloadTone,
  formatBytes,
  formatRelativeTime,
  formatTimestamp,
  type Tone,
} from "../utils/dashboardFormatting";
import styles from "../page.module.css";

interface ReadarrExperiencePanelProps {
  readingLists?: ReadingList[];
  metadataComparisons?: MetadataComparison[];
  downloadHistory?: DownloadHistoryEntry[];
  loading: boolean;
  error?: string;
  onRetry: () => void;
}

function progressPercent(completed: number, total: number): number {
  if (!total || total <= 0) {
    return 0;
  }
  return Math.min(100, Math.max(0, Math.round((completed / total) * 100)));
}

function confidencePercent(confidence: number): number {
  if (!Number.isFinite(confidence)) {
    return 0;
  }
  return Math.min(100, Math.max(0, Math.round(confidence * 100)));
}

function confidenceTone(confidence: number, mismatchCount: number): Tone {
  const percent = confidencePercent(confidence);
  if (mismatchCount === 0 && percent >= 90) {
    return "success";
  }
  if (percent >= 75) {
    return "neutral";
  }
  if (percent >= 50) {
    return "warning";
  }
  return "danger";
}

function confidenceLabel(confidence: number, mismatchCount: number): string {
  const percent = confidencePercent(confidence);
  if (mismatchCount === 0 && percent >= 90) {
    return "Synced";
  }
  if (percent >= 75) {
    return "Aligned";
  }
  if (percent >= 50) {
    return "Needs review";
  }
  return "Mismatched";
}

function metadataSourceSummary(source: MetadataSourceSummary | null | undefined): string {
  if (!source) {
    return "Missing";
  }
  const details: string[] = [];
  if (source.edition) {
    details.push(source.edition);
  }
  if (source.language) {
    details.push(source.language);
  }
  if (source.runtimeMinutes) {
    details.push(`${source.runtimeMinutes} min`);
  }
  if (source.quality) {
    details.push(source.quality);
  }
  if (source.format) {
    details.push(source.format);
  }
  return details.join(" · ") || "—";
}

function metadataIdentifier(source: MetadataSourceSummary | null | undefined): string | undefined {
  if (!source) {
    return undefined;
  }
  if (source.identifier) {
    return source.identifier;
  }
  if (source.publishedAt) {
    return formatTimestamp(source.publishedAt);
  }
  return undefined;
}

function timelineTimestamp(entry: DownloadHistoryEntry): string {
  return (
    formatTimestamp(entry.completedAt ?? entry.downloadedAt) +
    " · " +
    formatRelativeTime(entry.completedAt ?? entry.downloadedAt)
  );
}

export function ReadarrExperiencePanel({
  readingLists,
  metadataComparisons,
  downloadHistory,
  loading,
  error,
  onRetry,
}: ReadarrExperiencePanelProps) {
  const lists = (readingLists ?? []).slice(0, 4);
  const comparisons = (metadataComparisons ?? []).slice(0, 4);
  const history = (downloadHistory ?? []).slice(0, 6);

  return (
    <DataPanel
      title="Reading experiences"
      description="Track curated lists, metadata reconciliation, and download activity for the Readarr pipeline."
      loading={loading}
      error={error}
      onRetry={onRetry}
    >
      <div className={styles.readarrGrid}>
        <section className={styles.readarrSection} aria-label="Reading lists">
          <header className={styles.sectionHeader}>
            <h3 className={styles.sectionTitle}>Reading lists</h3>
            <p className={styles.sectionDescription}>
              Monitor synced lists and progress across audiobook and ebook collections.
            </p>
          </header>
          {lists.length ? (
            <ul className={styles.readingList}>
              {lists.map((list) => {
                const percent = progressPercent(list.completedCount, list.entryCount);
                return (
                  <li key={list.id} className={styles.readingListItem}>
                    <div className={styles.readingListHeader}>
                      <div>
                        <h4 className={styles.readingListTitle}>{list.name}</h4>
                        <div className={styles.smallText}>
                          {list.source}
                          {list.curator ? ` • Curated by ${list.curator}` : ""}
                        </div>
                      </div>
                      <StatusPill
                        label={`${list.completedCount}/${list.entryCount}`}
                        tone={percent >= 75 ? "success" : percent >= 40 ? "neutral" : "warning"}
                      />
                    </div>
                    <div className={styles.progressBar} aria-hidden="true">
                      <div
                        className={styles.progressValue}
                        style={{ width: `${percent}%` }}
                      />
                    </div>
                    <div className={styles.smallText}>
                      {percent}% complete
                      {list.lastSyncedAt
                        ? ` • Last synced ${formatRelativeTime(list.lastSyncedAt)}`
                        : ""}
                    </div>
                    {list.tags.length ? (
                      <div className={styles.badgeGroup}>
                        {list.tags.map((tag) => (
                          <span key={tag} className={styles.poolBadge}>
                            {tag}
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className={styles.emptyState}>No reading lists have been ingested yet.</p>
          )}
        </section>

        <section className={styles.readarrSection} aria-label="Metadata comparisons">
          <header className={styles.sectionHeader}>
            <h3 className={styles.sectionTitle}>Audiobook vs. ebook metadata</h3>
            <p className={styles.sectionDescription}>
              Compare normalized fields across providers to catch mismatches before requests reach the queue.
            </p>
          </header>
          {comparisons.length ? (
            <div className={styles.tableWrapper}>
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th scope="col">Title</th>
                    <th scope="col">Ebook</th>
                    <th scope="col">Audiobook</th>
                    <th scope="col">Confidence</th>
                  </tr>
                </thead>
                <tbody>
                  {comparisons.map((comparison) => {
                    const mismatchCount = comparison.mismatchedFields.length;
                    const confidence = confidencePercent(comparison.confidence);
                    const tone = confidenceTone(comparison.confidence, mismatchCount);
                    return (
                      <tr key={comparison.id}>
                        <td>
                          <div className={styles.metadataTitle}>{comparison.title}</div>
                          {comparison.author ? (
                            <div className={styles.smallText}>{comparison.author}</div>
                          ) : null}
                          {comparison.mismatchedFields.length ? (
                            <div className={styles.metadataMismatches}>
                              {comparison.mismatchedFields.map((field) => (
                                <span key={field} className={styles.metadataMismatchBadge}>
                                  {field}
                                </span>
                              ))}
                            </div>
                          ) : null}
                        </td>
                        <td>
                          {comparison.ebook ? (
                            <div className={styles.metadataSource}>
                              <div className={styles.metadataProvider}>{comparison.ebook.provider}</div>
                              {metadataIdentifier(comparison.ebook) ? (
                                <div className={styles.smallText}>
                                  {metadataIdentifier(comparison.ebook)}
                                </div>
                              ) : null}
                              <div className={styles.smallText}>
                                {metadataSourceSummary(comparison.ebook)}
                              </div>
                            </div>
                          ) : (
                            <StatusPill label="Missing" tone="danger" />
                          )}
                        </td>
                        <td>
                          {comparison.audiobook ? (
                            <div className={styles.metadataSource}>
                              <div className={styles.metadataProvider}>{comparison.audiobook.provider}</div>
                              {metadataIdentifier(comparison.audiobook) ? (
                                <div className={styles.smallText}>
                                  {metadataIdentifier(comparison.audiobook)}
                                </div>
                              ) : null}
                              <div className={styles.smallText}>
                                {metadataSourceSummary(comparison.audiobook)}
                              </div>
                            </div>
                          ) : (
                            <StatusPill label="Missing" tone="danger" />
                          )}
                        </td>
                        <td>
                          <div className={styles.metadataConfidence}>
                            <StatusPill
                              label={confidenceLabel(
                                comparison.confidence,
                                mismatchCount,
                              )}
                              tone={tone}
                            />
                            <div className={styles.smallText}>{confidence}% match</div>
                            {comparison.lastEvaluatedAt ? (
                              <div className={styles.smallText}>
                                Checked {formatRelativeTime(comparison.lastEvaluatedAt)}
                              </div>
                            ) : null}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          ) : (
            <p className={styles.emptyState}>
              No audiobook/ebook metadata comparisons are available yet.
            </p>
          )}
        </section>

        <section className={styles.readarrSection} aria-label="Download history">
          <header className={styles.sectionHeader}>
            <h3 className={styles.sectionTitle}>Download history</h3>
            <p className={styles.sectionDescription}>
              Review recent grabs and imports to validate automation coverage.
            </p>
          </header>
          {history.length ? (
            <ol className={styles.downloadTimeline}>
              {history.map((entry) => (
                <li key={entry.id} className={styles.downloadTimelineItem}>
                  <div className={styles.timelineMarker} aria-hidden="true" />
                  <div className={styles.timelineContent}>
                    <div className={styles.timelineHeader}>
                      <div>
                        <div className={styles.timelineTitle}>{entry.title}</div>
                        <div className={styles.smallText}>
                          {entry.format}
                          {entry.sourceList ? ` • List: ${entry.sourceList}` : ""}
                          {entry.client ? ` • Client: ${entry.client}` : ""}
                        </div>
                      </div>
                      <StatusPill label={entry.status} tone={downloadTone(entry.status)} />
                    </div>
                    <div className={styles.smallText}>{timelineTimestamp(entry)}</div>
                    <div className={styles.smallText}>
                      {formatBytes(entry.sizeBytes)}
                      {entry.failureReason ? ` • ${entry.failureReason}` : ""}
                    </div>
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <p className={styles.emptyState}>No downloads have been recorded yet.</p>
          )}
        </section>
      </div>
    </DataPanel>
  );
}
