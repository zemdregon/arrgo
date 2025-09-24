"use client";

import { useEffect, useMemo, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import type { JobSummary } from "../../../lib/api";
import {
  buildJobStats,
  buildOptions,
  buildQueueAnalytics,
  formatShowingSummary,
  normalize,
  optionLabel,
} from "../utils/dashboardData";
import {
  formatRelativeTime,
  formatTimestamp,
  jobTone,
} from "../utils/dashboardFormatting";
import styles from "../page.module.css";

interface JobQueuePanelProps {
  data?: JobSummary[];
  loading: boolean;
  error?: string;
  onRetry: () => void;
}

export function JobQueuePanel({ data, loading, error, onRetry }: JobQueuePanelProps) {
  const [jobStateFilter, setJobStateFilter] = useState("all");
  const [selectedJob, setSelectedJob] = useState<JobSummary | null>(null);

  const jobStats = useMemo(() => buildJobStats(data), [data]);
  const queueAnalytics = useMemo(() => buildQueueAnalytics(data), [data]);
  const jobStateOptions = useMemo(
    () =>
      buildOptions(
        (data ?? []).map((job) => job.state),
        "Unknown state",
        "unknown",
      ),
    [data],
  );

  useEffect(() => {
    if (
      jobStateFilter !== "all" &&
      !jobStateOptions.some((option) => option.value === jobStateFilter)
    ) {
      setJobStateFilter("all");
    }
  }, [jobStateFilter, jobStateOptions]);

  useEffect(() => {
    if (selectedJob && !data?.some((job) => job.id === selectedJob.id)) {
      setSelectedJob(null);
    }
  }, [data, selectedJob]);

  const visibleJobs = useMemo(() => {
    const items = data ?? [];
    if (jobStateFilter === "all") {
      return items;
    }
    return items.filter((job) => normalize(job.state) === jobStateFilter);
  }, [data, jobStateFilter]);

  const jobFooter = jobStats.total
    ? formatShowingSummary(
        visibleJobs.length,
        8,
        jobStateFilter === "all" ? "jobs" : "matches",
        jobStats.total,
      )
    : undefined;

  const selectedJobLabel =
    jobStateFilter === "all"
      ? "All states"
      : optionLabel(jobStateOptions, jobStateFilter, "Selected state");

  return (
    <>
      <DataPanel
        title="Job queue"
        description="Latest orchestration jobs dispatched by the master node."
        loading={loading}
        error={error}
        onRetry={onRetry}
        actions={
          jobStats.total ? (
            <div className={styles.panelControls}>
              <div className={styles.controlGroup}>
                <label className={styles.controlLabel} htmlFor="job-state-filter">
                  Job state
                </label>
                <select
                  id="job-state-filter"
                  className={styles.controlSelect}
                  value={jobStateFilter}
                  onChange={(event) => setJobStateFilter(event.target.value)}
                >
                  <option value="all">All states</option>
                  {jobStateOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          ) : null
        }
        footer={jobFooter}
      >
        {jobStats.total ? (
          visibleJobs.length ? (
            <>
              <div className={styles.metricsRow}>
                <div
                  className={[
                    styles.metricCard,
                    jobStateFilter !== "all" ? styles.metricCardEmphasis : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                >
                  <span className={styles.metricValue}>{visibleJobs.length}</span>
                  <span className={styles.metricLabel}>
                    {jobStateFilter === "all"
                      ? "Total jobs"
                      : "Filtered · " + selectedJobLabel}
                  </span>
                </div>
                {jobStateFilter !== "all" ? (
                  <div className={styles.metricCard}>
                    <span className={styles.metricValue}>{jobStats.total}</span>
                    <span className={styles.metricLabel}>Cluster total</span>
                  </div>
                ) : null}
                {jobStats.counts.map(([state, count]) => {
                  const isActive =
                    jobStateFilter !== "all" && normalize(state) === jobStateFilter;
                  return (
                    <div
                      key={state}
                      className={[
                        styles.metricCard,
                        isActive ? styles.metricCardActive : "",
                      ]
                        .filter(Boolean)
                        .join(" ")}
                    >
                      <span className={styles.metricValue}>{count}</span>
                      <span className={styles.metricLabel}>{state}</span>
                    </div>
                  );
                })}
              </div>
              <div className={styles.analyticsGrid}>
                <div className={styles.analyticsCard}>
                  <h3 className={styles.analyticsTitle}>By job type</h3>
                  {queueAnalytics.byType.length ? (
                    <ul className={styles.analyticsList}>
                      {queueAnalytics.byType.map(([type, count]) => (
                        <li key={type} className={styles.analyticsListItem}>
                          <span className={styles.analyticsLabel}>{type}</span>
                          <span className={styles.analyticsValue}>{count}</span>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className={styles.emptyState}>No jobs recorded yet.</p>
                  )}
                </div>
                <div className={styles.analyticsCard}>
                  <h3 className={styles.analyticsTitle}>Recent activity</h3>
                  <dl className={styles.analyticsDetails}>
                    <div className={styles.analyticsDetailRow}>
                      <dt>Average attempts</dt>
                      <dd>
                        {queueAnalytics.total
                          ? queueAnalytics.averageAttempts.toFixed(1)
                          : "—"}
                      </dd>
                    </div>
                    <div className={styles.analyticsDetailRow}>
                      <dt>Last submission</dt>
                      <dd>
                        {queueAnalytics.latestSubmittedAt
                          ? [
                              formatRelativeTime(queueAnalytics.latestSubmittedAt),
                              formatTimestamp(queueAnalytics.latestSubmittedAt),
                            ].join(" · ")
                          : "—"}
                      </dd>
                    </div>
                  </dl>
                </div>
              </div>
              <div className={styles.tableWrapper}>
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th scope="col">Job</th>
                      <th scope="col">Target</th>
                      <th scope="col">State</th>
                      <th scope="col">Submitted</th>
                      <th scope="col">Attempt</th>
                      <th scope="col">Details</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleJobs.slice(0, 8).map((job) => (
                      <tr key={job.id}>
                        <td>
                          <div className={styles.jobDetails}>
                            <span className={styles.monospace}>{job.jobType}</span>
                            {job.payloadURI ? (
                              <span className={styles.smallText}>{job.payloadURI}</span>
                            ) : null}
                          </div>
                        </td>
                        <td className={styles.monospace}>{job.targetID}</td>
                        <td>
                          <StatusPill
                            label={job.state || "unknown"}
                            tone={jobTone(job.state)}
                          />
                        </td>
                        <td>{formatTimestamp(job.submittedAt)}</td>
                        <td>{job.attempt}</td>
                        <td>
                          <button
                            type="button"
                            className={styles.linkButton}
                            onClick={() => setSelectedJob(job)}
                          >
                            Inspect
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : (
            <p className={styles.emptyState}>
              No jobs match the selected state yet. Try a different filter to
              inspect other workloads.
            </p>
          )
        ) : (
          <p className={styles.emptyState}>
            The job queue is empty. Schedule a library scan or import to see
            activity here.
          </p>
        )}
      </DataPanel>
      {selectedJob ? (
        <div className={styles.drawerBackdrop} role="presentation">
          <div
            className={styles.drawer}
            role="dialog"
            aria-modal="true"
            aria-label={"Details for job " + selectedJob.id}
          >
            <div className={styles.drawerHeader}>
              <h2 className={styles.drawerTitle}>Job details</h2>
              <button
                type="button"
                className={styles.drawerClose}
                onClick={() => setSelectedJob(null)}
              >
                Close
              </button>
            </div>
            <dl className={styles.drawerDetails}>
              <div className={styles.drawerDetailRow}>
                <dt>Job ID</dt>
                <dd className={styles.monospace}>{selectedJob.id}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Job type</dt>
                <dd className={styles.monospace}>{selectedJob.jobType}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Target</dt>
                <dd className={styles.monospace}>{selectedJob.targetID}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Status</dt>
                <dd>
                  <StatusPill
                    label={selectedJob.state || "unknown"}
                    tone={jobTone(selectedJob.state)}
                  />
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Payload URI</dt>
                <dd className={styles.monospace}>
                  {selectedJob.payloadURI || "—"}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Idempotency key</dt>
                <dd className={styles.monospace}>
                  {selectedJob.idempotencyKey || "—"}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Attempt</dt>
                <dd>{selectedJob.attempt}</dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Submitted</dt>
                <dd>
                  {formatTimestamp(selectedJob.submittedAt)}
                  {selectedJob.submittedAt ? (
                    <span className={styles.drawerSubtle}>
                      {formatRelativeTime(selectedJob.submittedAt)}
                    </span>
                  ) : null}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Last attempt</dt>
                <dd>
                  {selectedJob.lastAttemptAt ? (
                    <>
                      {formatTimestamp(selectedJob.lastAttemptAt)}
                      <span className={styles.drawerSubtle}>
                        {formatRelativeTime(selectedJob.lastAttemptAt)}
                      </span>
                    </>
                  ) : (
                    "—"
                  )}
                </dd>
              </div>
              <div className={styles.drawerDetailRow}>
                <dt>Completed</dt>
                <dd>
                  {selectedJob.completedAt ? (
                    <>
                      {formatTimestamp(selectedJob.completedAt)}
                      <span className={styles.drawerSubtle}>
                        {formatRelativeTime(selectedJob.completedAt)}
                      </span>
                    </>
                  ) : (
                    "—"
                  )}
                </dd>
              </div>
            </dl>
          </div>
        </div>
      ) : null}
    </>
  );
}
