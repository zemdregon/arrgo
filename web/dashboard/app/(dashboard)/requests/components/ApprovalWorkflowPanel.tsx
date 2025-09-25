"use client";

import { useMemo, useState } from "react";
import { DataPanel } from "../../../../components/DataPanel";
import { StatusPill } from "../../../../components/StatusPill";
import styles from "./ApprovalWorkflowPanel.module.css";

type MediaKind = "Movie" | "Series" | "Album" | "Book";

type AutomationStep = {
  id: string;
  label: string;
  engine: string;
  result: "ready" | "blocked" | "warning";
  note: string;
};

type StatusEvent = {
  id: string;
  status: string;
  actor: string;
  at: string;
  note?: string;
};

type RequestRecord = {
  id: string;
  title: string;
  kind: MediaKind;
  requestedBy: string;
  submittedAt: string;
  slaDueAt: string;
  policy: string;
  automation: AutomationStep[];
  history: StatusEvent[];
};

const REQUESTS: RequestRecord[] = [
  {
    id: "REQ-4821",
    title: "Dune: Part Two",
    kind: "Movie",
    requestedBy: "sarah@arrgo.dev",
    submittedAt: "2025-10-03T16:24:00Z",
    slaDueAt: "2025-10-04T04:24:00Z",
    policy: "Premium Sci-Fi",
    automation: [
      {
        id: "auto-1",
        label: "Match quality against Dolby Vision policy",
        engine: "Policy Engine",
        result: "ready",
        note: "Meets 4K + Dolby Vision target",
      },
      {
        id: "auto-2",
        label: "Check theatrical availability",
        engine: "Indexer federation",
        result: "warning",
        note: "Cam releases filtered; waiting on official digital",
      },
      {
        id: "auto-3",
        label: "Schedule Plex library sync",
        engine: "Automation Runner",
        result: "ready",
        note: "Slot reserved in overnight batch",
      },
    ],
    history: [
      {
        id: "hist-1",
        status: "Requested",
        actor: "sarah@arrgo.dev",
        at: "2025-10-03T16:24:00Z",
      },
      {
        id: "hist-2",
        status: "Policy matched: Premium Sci-Fi",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T16:24:22Z",
      },
      {
        id: "hist-3",
        status: "Sent to approvers",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T16:24:45Z",
        note: "Assigned to Movies guild",
      },
    ],
  },
  {
    id: "REQ-4822",
    title: "Foundation — Season 3",
    kind: "Series",
    requestedBy: "guild.ops@arrgo.dev",
    submittedAt: "2025-10-03T09:12:00Z",
    slaDueAt: "2025-10-04T09:12:00Z",
    policy: "Premium Sci-Fi",
    automation: [
      {
        id: "auto-4",
        label: "Verify release window",
        engine: "Calendars",
        result: "ready",
        note: "Season confirmed for Q4 — no embargo",
      },
      {
        id: "auto-5",
        label: "Check worker capacity",
        engine: "Worker Director",
        result: "ready",
        note: "Indexer + download pool available",
      },
      {
        id: "auto-6",
        label: "Simulate import",
        engine: "Automation Runner",
        result: "ready",
        note: "90% success confidence",
      },
    ],
    history: [
      {
        id: "hist-4",
        status: "Requested",
        actor: "guild.ops@arrgo.dev",
        at: "2025-10-03T09:12:00Z",
      },
      {
        id: "hist-5",
        status: "Policy matched: Premium Sci-Fi",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T09:12:25Z",
      },
      {
        id: "hist-6",
        status: "Auto-approval suggestion",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T09:13:01Z",
        note: "Existing user quota available",
      },
    ],
  },
  {
    id: "REQ-4823",
    title: "The National — Laugh Track",
    kind: "Album",
    requestedBy: "music@arrgo.dev",
    submittedAt: "2025-10-02T22:40:00Z",
    slaDueAt: "2025-10-03T22:40:00Z",
    policy: "Audiophile",
    automation: [
      {
        id: "auto-7",
        label: "Check lossless sources",
        engine: "MusicBrainz + Qobuz",
        result: "blocked",
        note: "Lossless release not yet indexed",
      },
      {
        id: "auto-8",
        label: "Fallback to lossy",
        engine: "Automation Runner",
        result: "warning",
        note: "Policy requires manual approval for MP3 fallback",
      },
    ],
    history: [
      {
        id: "hist-7",
        status: "Requested",
        actor: "music@arrgo.dev",
        at: "2025-10-02T22:40:00Z",
      },
      {
        id: "hist-8",
        status: "Policy matched: Audiophile",
        actor: "automation@arrgo.dev",
        at: "2025-10-02T22:40:41Z",
      },
      {
        id: "hist-9",
        status: "Awaiting curator review",
        actor: "automation@arrgo.dev",
        at: "2025-10-02T22:41:05Z",
        note: "Lossless sources unavailable",
      },
    ],
  },
  {
    id: "REQ-4824",
    title: "Sherlock — Series 4",
    kind: "Series",
    requestedBy: "public-user-1024",
    submittedAt: "2025-10-03T01:15:00Z",
    slaDueAt: "2025-10-04T01:15:00Z",
    policy: "Legacy TV",
    automation: [
      {
        id: "auto-9",
        label: "Validate Plex availability",
        engine: "Library Sync",
        result: "ready",
        note: "Seasons 1-3 already present",
      },
      {
        id: "auto-10",
        label: "Quota enforcement",
        engine: "Policy Engine",
        result: "ready",
        note: "Public quota remaining: 12",
      },
    ],
    history: [
      {
        id: "hist-10",
        status: "Requested",
        actor: "public-user-1024",
        at: "2025-10-03T01:15:00Z",
      },
      {
        id: "hist-11",
        status: "Policy matched: Legacy TV",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T01:15:16Z",
      },
      {
        id: "hist-12",
        status: "Auto-approval suggestion",
        actor: "automation@arrgo.dev",
        at: "2025-10-03T01:16:04Z",
        note: "Seasons 1-3 complete, staff approval recommended",
      },
    ],
  },
];

const dateFormatter = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
  timeStyle: "short",
});

const relativeFormatter = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

function formatRelative(target: string) {
  const targetDate = new Date(target).getTime();
  const now = Date.now();
  const diffMs = targetDate - now;
  const diffMinutes = diffMs / 60000;
  if (Math.abs(diffMinutes) < 60) {
    return relativeFormatter.format(Math.round(diffMinutes), "minute");
  }
  const diffHours = diffMinutes / 60;
  if (Math.abs(diffHours) < 48) {
    return relativeFormatter.format(Math.round(diffHours), "hour");
  }
  const diffDays = diffHours / 24;
  return relativeFormatter.format(Math.round(diffDays), "day");
}

function formatSince(target: string) {
  const targetDate = new Date(target).getTime();
  const now = Date.now();
  const diffMs = now - targetDate;
  const diffMinutes = diffMs / 60000;
  if (diffMinutes < 60) {
    return `${Math.max(1, Math.round(diffMinutes))}m ago`;
  }
  const diffHours = diffMinutes / 60;
  if (diffHours < 24) {
    return `${diffHours.toFixed(1)}h ago`;
  }
  const diffDays = diffHours / 24;
  return `${diffDays.toFixed(1)}d ago`;
}

function slaTone(record: RequestRecord) {
  const dueMs = new Date(record.slaDueAt).getTime();
  const now = Date.now();
  const diffHours = (dueMs - now) / 3600000;
  if (diffHours < 0) {
    return { tone: "danger" as const, label: "Past due" };
  }
  if (diffHours < 4) {
    return { tone: "warning" as const, label: `Due in ${diffHours.toFixed(1)}h` };
  }
  return { tone: "success" as const, label: `Due in ${formatRelative(record.slaDueAt)}` };
}

interface ApprovalWorkflowPanelProps {
  className?: string;
}

export function ApprovalWorkflowPanel({ className }: ApprovalWorkflowPanelProps) {
  const [selectedIds, setSelectedIds] = useState<string[]>([REQUESTS[0]?.id ?? ""]);
  const [focusedId, setFocusedId] = useState<string>(REQUESTS[0]?.id ?? "");
  const [feedback, setFeedback] = useState<string | null>(null);

  const selectedRequests = useMemo(
    () => REQUESTS.filter((request) => selectedIds.includes(request.id)),
    [selectedIds],
  );

  const focusedRequest = useMemo(
    () => REQUESTS.find((request) => request.id === focusedId) ?? REQUESTS[0],
    [focusedId],
  );

  const toggleSelection = (id: string) => {
    setSelectedIds((current) => {
      if (current.includes(id)) {
        return current.filter((value) => value !== id);
      }
      return [...current, id];
    });
  };

  const handleBulkAction = (action: "approve" | "deny" | "automation") => {
    if (!selectedRequests.length) {
      setFeedback("Select at least one request to run a bulk action.");
      return;
    }
    const message =
      action === "approve"
        ? "Prepared approval packet"
        : action === "deny"
          ? "Logged denials and sent notifications"
          : "Generated automation preview";
    setFeedback(
      `${message} for ${selectedRequests.length} request${selectedRequests.length === 1 ? "" : "s"}.`,
    );
  };

  const handleSelectAll = () => {
    setSelectedIds(REQUESTS.map((request) => request.id));
  };

  const handleClear = () => {
    setSelectedIds([]);
  };

  const inSelection = (id: string) => selectedIds.includes(id);

  const automationHeadingId = focusedRequest ? `automation-${focusedRequest.id}` : undefined;
  const historyHeadingId = focusedRequest ? `history-${focusedRequest.id}` : undefined;

  return (
    <div className={className}>
      <DataPanel
        title="Approval workflow"
        description="Review requests with automation insights, run bulk actions, and audit status history before confirming outcomes."
      >
        <div className={styles.content}>
          <div className={styles.toolbar}>
            <div className={styles.bulkGroup}>
              <button
                type="button"
                className={styles.bulkButton}
                onClick={handleSelectAll}
              >
                Select all
              </button>
              <button type="button" className={styles.bulkButton} onClick={handleClear}>
                Clear
              </button>
              <button
                type="button"
                className={styles.bulkButton}
                data-tone="primary"
                onClick={() => handleBulkAction("approve")}
              >
                Approve selected
              </button>
              <button
                type="button"
                className={styles.bulkButton}
                data-tone="danger"
                onClick={() => handleBulkAction("deny")}
              >
                Deny selected
              </button>
              <button
                type="button"
                className={styles.bulkButton}
                data-tone="primary"
                onClick={() => handleBulkAction("automation")}
              >
                Preview automation
              </button>
            </div>
            {feedback ? <div className={styles.feedback}>{feedback}</div> : null}
          </div>

          <div className={styles.grid}>
            <div className={styles.list}>
              <div className={styles.listHeader}>
                <span>Select</span>
                <span>Request</span>
                <span className={styles.actionsColumn}>SLA</span>
              </div>
              {REQUESTS.map((request) => {
                const sla = slaTone(request);
                return (
                  <div
                    key={request.id}
                    className={styles.listRow}
                    data-focused={focusedRequest?.id === request.id ? "true" : undefined}
                    onClick={() => setFocusedId(request.id)}
                  >
                    <span className={styles.checkbox}>
                      <input
                        type="checkbox"
                        checked={inSelection(request.id)}
                        onChange={(event) => {
                          event.stopPropagation();
                          toggleSelection(request.id);
                        }}
                        aria-label={`Select ${request.title}`}
                      />
                    </span>
                    <div>
                      <p className={styles.rowTitle}>{request.title}</p>
                      <div className={styles.rowMeta}>
                        <span>{request.kind} • {request.policy}</span>
                        <span>Requested by {request.requestedBy}</span>
                        <span>Submitted {formatSince(request.submittedAt)}</span>
                      </div>
                    </div>
                    <div className={styles.actionsColumn}>
                      <StatusPill label={sla.label} tone={sla.tone} />
                    </div>
                  </div>
                );
              })}
            </div>

            {focusedRequest ? (
              <div className={styles.details}>
                <div className={styles.detailsHeader}>
                  <h3 className={styles.detailsTitle}>{focusedRequest.title}</h3>
                  <StatusPill label={focusedRequest.policy} tone="neutral" />
                </div>
                <div className={styles.detailMeta}>
                  <span>Requested by {focusedRequest.requestedBy}</span>
                  <span>Submitted {dateFormatter.format(new Date(focusedRequest.submittedAt))}</span>
                  <span>SLA due {dateFormatter.format(new Date(focusedRequest.slaDueAt))} ({formatRelative(focusedRequest.slaDueAt)})</span>
                </div>

                <section aria-labelledby={automationHeadingId}>
                  <h4 id={automationHeadingId} className={styles.sectionTitle}>
                    Automation preview
                  </h4>
                  <ul className={styles.automationList}>
                    {focusedRequest.automation.map((step) => (
                      <li key={step.id} className={styles.automationItem}>
                        <span className={styles.automationTitle}>{step.label}</span>
                        <span className={styles.automationMeta}>{step.engine}</span>
                        <StatusPill
                          label={
                            step.result === "ready"
                              ? "Ready"
                              : step.result === "blocked"
                                ? "Blocked"
                                : "Needs review"
                          }
                          tone={
                            step.result === "ready"
                              ? "success"
                              : step.result === "blocked"
                                ? "danger"
                                : "warning"
                          }
                        />
                        <span>{step.note}</span>
                      </li>
                    ))}
                  </ul>
                </section>

                <section aria-labelledby={historyHeadingId}>
                  <h4 id={historyHeadingId} className={styles.sectionTitle}>
                    Status history
                  </h4>
                  <div className={styles.timeline}>
                    {focusedRequest.history.map((event) => (
                      <div key={event.id} className={styles.timelineEntry}>
                        <span className={styles.timelineMarker} aria-hidden />
                        <div className={styles.timelineBody}>
                          <span className={styles.timelineStatus}>{event.status}</span>
                          <span className={styles.timelineMeta}>
                            {dateFormatter.format(new Date(event.at))} • {event.actor}
                          </span>
                          {event.note ? <span>{event.note}</span> : null}
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              </div>
            ) : null}
          </div>
        </div>
      </DataPanel>
    </div>
  );
}
