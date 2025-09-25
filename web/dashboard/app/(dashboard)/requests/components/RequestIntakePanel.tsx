"use client";

import { FormEvent, useMemo, useState } from "react";
import { DataPanel } from "../../../../components/DataPanel";
import { StatusPill } from "../../../../components/StatusPill";
import styles from "./RequestIntakePanel.module.css";

type RequestMode = "public" | "private";

type MediaAvailability = "ready" | "missing" | "upgrade" | "monitoring";

type MediaKind = "Movie" | "Series" | "Album" | "Book";

type MediaCandidate = {
  id: string;
  title: string;
  kind: MediaKind;
  year?: number;
  availability: MediaAvailability;
  requestQueue: number;
  qualityTarget: string;
  monitored: boolean;
};

type QuotaState = {
  total: number;
  used: number;
  resetsIn: string;
  policy: string;
};

const INITIAL_QUOTA: Record<RequestMode, QuotaState> = {
  public: {
    total: 20,
    used: 8,
    resetsIn: "3 days",
    policy: "Public requests limited to 20 per rolling week",
  },
  private: {
    total: 40,
    used: 26,
    resetsIn: "12 hours",
    policy: "Privileged queue tied to automation and staff overrides",
  },
};

const MEDIA_CATALOG: MediaCandidate[] = [
  {
    id: "tt15239678",
    title: "Dune: Part Two",
    kind: "Movie",
    year: 2024,
    availability: "ready",
    requestQueue: 2,
    qualityTarget: "4K Dolby Vision",
    monitored: true,
  },
  {
    id: "tt13622776",
    title: "Foundation",
    kind: "Series",
    year: 2021,
    availability: "monitoring",
    requestQueue: 1,
    qualityTarget: "UHD Season Pack",
    monitored: true,
  },
  {
    id: "tt15627332",
    title: "The Fall Guy",
    kind: "Movie",
    year: 2024,
    availability: "upgrade",
    requestQueue: 3,
    qualityTarget: "1080p HDR",
    monitored: false,
  },
  {
    id: "mbid-7e7f32",
    title: "The National — Laugh Track",
    kind: "Album",
    year: 2023,
    availability: "missing",
    requestQueue: 0,
    qualityTarget: "FLAC",
    monitored: false,
  },
  {
    id: "isbn-9780385547345",
    title: "The Midnight Library",
    kind: "Book",
    availability: "ready",
    requestQueue: 1,
    qualityTarget: "ePub + Audiobook",
    monitored: true,
  },
  {
    id: "tt7949218",
    title: "Mission: Impossible — Dead Reckoning",
    kind: "Movie",
    year: 2023,
    availability: "ready",
    requestQueue: 0,
    qualityTarget: "IMAX Enhanced",
    monitored: true,
  },
  {
    id: "tt1475582",
    title: "Sherlock",
    kind: "Series",
    availability: "missing",
    requestQueue: 4,
    qualityTarget: "1080p WEB",
    monitored: false,
  },
];

const KIND_FILTERS: Array<{ label: string; value: "all" | MediaKind }>= [
  { label: "All media", value: "all" },
  { label: "Movies", value: "Movie" },
  { label: "Series", value: "Series" },
  { label: "Albums", value: "Album" },
  { label: "Books", value: "Book" },
];

const AVAILABILITY_LABEL: Record<MediaAvailability, { label: string; tone: "success" | "warning" | "neutral" | "danger" }> = {
  ready: { label: "Available to auto-process", tone: "success" },
  missing: { label: "No sources discovered", tone: "danger" },
  upgrade: { label: "Upgrade candidates", tone: "warning" },
  monitoring: { label: "Monitoring indexers", tone: "neutral" },
};

interface RequestIntakePanelProps {
  className?: string;
}

export function RequestIntakePanel({ className }: RequestIntakePanelProps) {
  const [mode, setMode] = useState<RequestMode>("public");
  const [quota, setQuota] = useState<Record<RequestMode, QuotaState>>(INITIAL_QUOTA);
  const [query, setQuery] = useState("");
  const [kindFilter, setKindFilter] = useState<(typeof KIND_FILTERS)[number]["value"]>("all");
  const [feedback, setFeedback] = useState<string | null>(null);
  const [submittingId, setSubmittingId] = useState<string | null>(null);

  const quotaState = quota[mode];
  const quotaRemaining = Math.max(0, quotaState.total - quotaState.used);
  const quotaPercent = Math.min(100, Math.round((quotaState.used / quotaState.total) * 100));

  const filteredResults = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return MEDIA_CATALOG.filter((candidate) => {
      if (kindFilter !== "all" && candidate.kind !== kindFilter) {
        return false;
      }
      if (!normalizedQuery) {
        return true;
      }
      const haystack = `${candidate.title} ${candidate.year ?? ""}`.toLowerCase();
      return haystack.includes(normalizedQuery);
    });
  }, [kindFilter, query]);

  const availability = (status: MediaAvailability) => AVAILABILITY_LABEL[status];

  const handleModeChange = (next: RequestMode) => {
    setMode(next);
    setFeedback(null);
  };

  const handleSearchSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
  };

  const handleRequest = (candidate: MediaCandidate) => {
    if (quotaRemaining <= 0) {
      setFeedback("Quota exhausted — escalate or extend policy to accept more requests.");
      return;
    }

    setSubmittingId(candidate.id);
    setFeedback(null);

    window.setTimeout(() => {
      setQuota((current) => ({
        ...current,
        [mode]: {
          ...current[mode],
          used: Math.min(current[mode].total, current[mode].used + 1),
        },
      }));
      setSubmittingId(null);
      setFeedback(
        `${candidate.title} queued for ${mode === "public" ? "community" : "staff"} review.`,
      );
    }, 400);
  };

  const selectedFilter = KIND_FILTERS.find((item) => item.value === kindFilter)?.label ?? "All media";

  return (
    <div className={className}>
      <DataPanel
        title="Request intake"
        description="Accept requests directly in arrgo with policy-aware quotas, availability insights, and search that spans automation sources."
      >
        <div className={styles.content}>
          <div className={styles.modeToggle} role="radiogroup" aria-label="Request visibility">
            <button
              type="button"
              className={styles.modeOption}
              data-active={mode === "public" ? "true" : undefined}
              onClick={() => handleModeChange("public")}
            >
              Public queue
            </button>
            <button
              type="button"
              className={styles.modeOption}
              data-active={mode === "private" ? "true" : undefined}
              onClick={() => handleModeChange("private")}
            >
              Private / staff
            </button>
          </div>

          <div className={styles.quotaCard}>
            <div className={styles.quotaHeader}>
              <div>
                <div className={styles.quotaLabel}>Quota usage</div>
                <div className={styles.quotaValue}>
                  {quotaState.used} / {quotaState.total}
                </div>
              </div>
              <StatusPill
                label={quotaRemaining > 0 ? `${quotaRemaining} requests remaining` : "Quota exhausted"}
                tone={quotaRemaining > 5 ? "success" : quotaRemaining > 0 ? "warning" : "danger"}
              />
            </div>
            <div className={styles.quotaProgress}>
              <span className={styles.quotaBarFill} style={{ width: `${quotaPercent}%` }} />
            </div>
            <div className={styles.quotaMeta}>
              <span>{quotaState.policy}</span>
              <span>Resets in {quotaState.resetsIn}</span>
            </div>
          </div>

          <form className={styles.searchForm} onSubmit={handleSearchSubmit}>
            <div className={styles.searchInputs}>
              <input
                type="search"
                placeholder="Search movies, shows, albums, and books"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                aria-label="Search media"
              />
              <select
                value={kindFilter}
                onChange={(event) => {
                  setKindFilter(event.target.value as (typeof KIND_FILTERS)[number]["value"]);
                }}
                aria-label="Filter media type"
              >
                {KIND_FILTERS.map((filter) => (
                  <option key={filter.value} value={filter.value}>
                    {filter.label}
                  </option>
                ))}
              </select>
              <button type="submit">Search</button>
            </div>
            <div className={styles.searchMeta}>
              <span>{filteredResults.length} matches • {selectedFilter}</span>
              <span>mode: {mode === "public" ? "Public" : "Private"}</span>
            </div>
          </form>

          {feedback ? <div className={styles.feedback}>{feedback}</div> : null}

          {filteredResults.length ? (
            <div className={styles.results}>
              {filteredResults.map((candidate) => {
                const status = availability(candidate.availability);
                return (
                  <article key={candidate.id} className={styles.resultCard}>
                    <div className={styles.resultHeader}>
                      <div>
                        <h3 className={styles.resultTitle}>{candidate.title}</h3>
                        <div className={styles.resultMeta}>
                          <span className={styles.tag}>{candidate.kind}</span>
                          {candidate.year ? <span className={styles.tag}>{candidate.year}</span> : null}
                          <span className={styles.tag}>{candidate.qualityTarget}</span>
                        </div>
                      </div>
                      <StatusPill label={status.label} tone={status.tone} />
                    </div>
                    <div className={styles.resultBody}>
                      <div className={styles.availability}>
                        <span>
                          {candidate.requestQueue} in queue • {candidate.monitored ? "Monitored" : "Unmonitored"}
                        </span>
                      </div>
                      <div className={styles.resultActions}>
                        <button
                          type="button"
                          className={styles.requestButton}
                          disabled={submittingId === candidate.id || quotaRemaining <= 0}
                          onClick={() => handleRequest(candidate)}
                        >
                          {submittingId === candidate.id ? "Queuing…" : "Request"}
                        </button>
                      </div>
                    </div>
                  </article>
                );
              })}
            </div>
          ) : (
            <div className={styles.emptyState}>
              No media matched “{query}”. Try a different title or broaden your filters.
            </div>
          )}
        </div>
      </DataPanel>
    </div>
  );
}
