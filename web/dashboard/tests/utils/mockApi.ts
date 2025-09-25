import type { Page } from "@playwright/test";

export interface MockResponseMetric {
  method: string;
  path: string;
  duration: number;
}

export interface MockMetrics {
  responses: MockResponseMetric[];
}

interface ManualSearchState {
  result: {
    ID: string;
    EntityID: string;
    Query: string;
    ExecutedAt: string;
    Results: Array<{
      ID: string;
      SearchID: string;
      EntityID: string;
      Title: string;
      Indexer: string;
      Protocol: string;
      Quality: string;
      PublishDate: string;
      SizeBytes: number;
      Seeders: number;
      Grabs: number;
      Score: number;
      Status: string;
      RejectionReason?: string;
      DownloadURL: string;
      GrabbedAt?: string;
    }>;
  };
}

interface WorkerPoolsState {
  pools: Array<{
    pool: string;
    description?: string;
    stream: string;
    consumer_group: string;
    heartbeat_interval_seconds: number;
    stale_after_seconds: number;
    max_concurrent_jobs?: number;
  }>;
}

interface MockState {
  manualSearch: ManualSearchState;
  workerPools: WorkerPoolsState;
}

export interface DashboardMocks {
  metrics: MockMetrics;
  state: MockState;
}

const AUTH_STORAGE_KEY = "arrgo:dashboard:auth";
const ENV_STORAGE_KEY = "arrgo:dashboard:environment";

export async function setupDashboardMocks(page: Page): Promise<DashboardMocks> {
  const now = new Date();
  const isoNow = now.toISOString();

  const state: MockState = {
    manualSearch: {
      result: {
        ID: "search-1",
        EntityID: "lib-foundation",
        Query: "",
        ExecutedAt: isoNow,
        Results: [
          {
            ID: "rel-foundation-1080p",
            SearchID: "search-1",
            EntityID: "lib-foundation",
            Title: "Foundation S01E01 1080p WEB",
            Indexer: "Helios",
            Protocol: "torrent",
            Quality: "1080p WEB",
            PublishDate: isoNow,
            SizeBytes: 2_560_000_000,
            Seeders: 142,
            Grabs: 58,
            Score: 98.4,
            Status: "available",
            DownloadURL: "https://releases.arrgo.dev/foundation-s01e01",
          },
        ],
      },
    },
    workerPools: {
      pools: [
        {
          pool: "default",
          description: "Default automation",
          stream: "arrgo.jobs.default",
          consumer_group: "workers-default",
          heartbeat_interval_seconds: 30,
          stale_after_seconds: 120,
          max_concurrent_jobs: 6,
        },
        {
          pool: "backlog",
          description: "Backlog catchup",
          stream: "arrgo.jobs.backlog",
          consumer_group: "workers-backlog",
          heartbeat_interval_seconds: 45,
          stale_after_seconds: 180,
        },
      ],
    },
  };

  const metrics: MockMetrics = { responses: [] };

  await page.addInitScript(
    ({ authKey, envKey, principalTimestamp }) => {
      const authPayload = {
        environments: {
          local: {
            keys: [
              {
                id: "playwright-operator",
                label: "Playwright operator",
                token: "test-token",
                tokenId: "playwright",
                role: "operator",
                permissions: [
                  "library:read",
                  "library:write",
                  "worker:read",
                  "worker:manage",
                  "observability:read",
                  "config:read",
                  "requests:manage",
                ],
                lastValidatedAt: principalTimestamp,
              },
            ],
            activeKeyId: "playwright-operator",
          },
        },
      };
      try {
        window.localStorage.setItem(envKey, "local");
        window.localStorage.setItem(authKey, JSON.stringify(authPayload));
      } catch (error) {
        // Ignore storage errors (e.g., sandbox restrictions).
      }
    },
    { authKey: AUTH_STORAGE_KEY, envKey: ENV_STORAGE_KEY, principalTimestamp: isoNow },
  );

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const { pathname, searchParams } = url;
    const method = request.method();
    const start = Date.now();

    if (method === "OPTIONS") {
      await route.fulfill({
        status: 204,
        headers: {
          "access-control-allow-origin": "*",
          "access-control-allow-methods": "GET,POST,PUT,OPTIONS",
          "access-control-allow-headers": "content-type, authorization",
        },
        body: "",
      });
      metrics.responses.push({ method, path: pathname, duration: Date.now() - start });
      return;
    }

    const jsonHeaders = {
      "access-control-allow-origin": "*",
      "content-type": "application/json",
    };

    const fulfill = async (status: number, body: unknown) => {
      await route.fulfill({
        status,
        headers: jsonHeaders,
        body: JSON.stringify(body),
      });
      metrics.responses.push({ method, path: pathname, duration: Date.now() - start });
    };

    if (method === "GET" && pathname === "/api/v1/auth/principal") {
      await fulfill(200, {
        principal: {
          key: {
            id: "playwright-operator",
            label: "Playwright operator",
            role: "operator",
            version: 1,
            active: true,
            created_at: isoNow,
            last_rotated_at: isoNow,
            last_used_at: isoNow,
          },
          role: {
            name: "operator",
            description: "Synthetic operator principal for automated tests",
            permissions: [
              "library:read",
              "library:write",
              "worker:read",
              "worker:manage",
              "observability:read",
              "config:read",
              "requests:manage",
            ],
          },
          permissions: [
            "library:read",
            "library:write",
            "worker:read",
            "worker:manage",
            "observability:read",
            "config:read",
            "requests:manage",
          ],
        },
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/library/entities") {
      await fulfill(200, {
        entities: [
          {
            ID: "lib-foundation",
            EntityType: "Series",
            Title: "Foundation",
            Synopsis: "A rebellion across the galaxy tests arrgo ingest orchestration.",
            PendingImports: 3,
            QualityUpgrades: 2,
            Failures: 0,
          },
          {
            ID: "lib-dune",
            EntityType: "Movie",
            Title: "Dune: Part Two",
            Synopsis: "Visual regression friendly blockbuster.",
            PendingImports: 1,
            QualityUpgrades: 1,
            Failures: 0,
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/jobs") {
      await fulfill(200, {
        jobs: [
          {
            ID: "job-queue-scan",
            JobType: "scan",
            TargetID: "lib-foundation",
            State: "queued",
            SubmittedAt: isoNow,
            PayloadURI: "",
            IdempotencyKey: "",
            Attempt: 0,
            LastAttemptAt: isoNow,
            CompletedAt: "",
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/workers") {
      await fulfill(200, {
        workers: [
          {
            id: "worker-alpha",
            pool: "default",
            capabilities: ["scan", "import", "analyze"],
            endpoint: "http://worker-alpha:9000",
            version: "1.18.0",
            status: "healthy",
            registered_at: isoNow,
            last_heartbeat_at: isoNow,
            active_jobs: 1,
          },
          {
            id: "worker-beta",
            pool: "backlog",
            capabilities: ["remediate", "repair"],
            endpoint: "http://worker-beta:9000",
            version: "1.18.0",
            status: "degraded",
            registered_at: isoNow,
            last_heartbeat_at: isoNow,
            active_jobs: 2,
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/worker-pools") {
      await fulfill(200, {
        pools: state.workerPools.pools,
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/queue/analytics") {
      await fulfill(200, {
        analytics: {
          updated_at: isoNow,
          observed_window_seconds: 900,
          stream_lag: [
            {
              stream: "arrgo.jobs.default",
              consumer_group: "workers-default",
              lag_seconds: 42,
              lag_messages: 18,
              oldest_pending_age_seconds: 320,
              last_produced_at: isoNow,
              last_consumed_at: isoNow,
              status: "warning",
            },
          ],
          retry_metrics: [
            {
              job_type: "scan",
              retry_count: 2,
              recent_failure_rate: 0.08,
              max_attempts: 5,
              oldest_retry_age_seconds: 540,
              last_retry_at: isoNow,
              status: "healthy",
            },
          ],
          sla: [
            {
              queue: "requests",
              target_seconds: 3600,
              p95_seconds: 2400,
              p99_seconds: 4000,
              success_rate: 0.94,
              breaches_today: 1,
              status: "warning",
              updated_at: isoNow,
            },
          ],
        },
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/observability/dashboard") {
      await fulfill(200, {
        snapshot: {
          updated_at: isoNow,
          metrics: [],
          traces: [],
          logs: [],
        },
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/infrastructure/configuration") {
      await fulfill(200, {
        configuration: {
          secrets: [
            {
              name: "arrgo/postgres/password",
              provider: "aws-secrets-manager",
              path: "arrgo/prod/postgres",
              last_rotated_at: isoNow,
              rotation_interval_hours: 168,
              owner: "sre@arrgo.dev",
              status: "ok",
            },
          ],
          environment: [
            {
              key: "ARRGO_API_URL",
              value: "https://api.arrgo.dev",
              scope: "dashboard",
              source: "terraform",
              required: true,
            },
            {
              key: "ARRGO_QUEUE_STREAM",
              value: "arrgo.jobs.default",
              scope: "workers",
              source: "terraform",
              required: true,
            },
          ],
          documentation_url: "https://docs.arrgo.dev",
          last_updated_at: isoNow,
        },
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/library/reading-lists") {
      await fulfill(200, {
        lists: [
          {
            id: "reading-1",
            name: "Hugo Award Winners",
            source: "Goodreads",
            curator: "arrgo automation",
            entry_count: 42,
            completed_count: 18,
            last_synced_at: isoNow,
            tags: ["sci-fi", "books"],
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/library/metadata/comparisons") {
      await fulfill(200, {
        comparisons: [
          {
            id: "compare-foundation",
            title: "Foundation",
            author: "Isaac Asimov",
            ebook: {
              provider: "Kindle",
              identifier: "ASIN-B000FC1PJI",
              language: "en",
              published_at: isoNow,
              format: "ebook",
            },
            audiobook: {
              provider: "Audible",
              identifier: "B01L0831K2",
              language: "en",
              runtime_minutes: 960,
              format: "audiobook",
            },
            confidence: 0.91,
            mismatched_fields: ["runtime"],
            last_evaluated_at: isoNow,
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/library/download-history") {
      await fulfill(200, {
        history: [
          {
            id: "download-1",
            entity_id: "lib-foundation",
            title: "Foundation S01E01 1080p",
            format: "torrent",
            status: "completed",
            size_bytes: 2_560_000_000,
            source_list: "Helios",
            client: "qBittorrent",
            downloaded_at: isoNow,
            completed_at: isoNow,
          },
        ],
      });
      return;
    }

    if (method === "GET" && pathname === "/api/v1/indexer/search") {
      const entityId = searchParams.get("entity_id");
      if (!entityId || entityId !== state.manualSearch.result.EntityID) {
        await fulfill(200, { search: null });
        return;
      }
      await fulfill(200, { search: state.manualSearch.result });
      return;
    }

    if (method === "POST" && pathname === "/api/v1/indexer/search") {
      const payload = JSON.parse(request.postData() ?? "{}") as {
        entity_id?: string;
        query?: string;
      };
      if (payload.entity_id === state.manualSearch.result.EntityID) {
        state.manualSearch.result = {
          ...state.manualSearch.result,
          Query: payload.query ?? "",
          ExecutedAt: new Date().toISOString(),
        };
      }
      await fulfill(200, { search: state.manualSearch.result });
      return;
    }

    if (method === "POST" && pathname === "/api/v1/indexer/releases/grab") {
      const payload = JSON.parse(request.postData() ?? "{}") as {
        entity_id?: string;
        release_id?: string;
      };
      const matchingRelease = state.manualSearch.result.Results.find(
        (release) => release.ID === payload.release_id,
      );
      if (matchingRelease) {
        matchingRelease.Status = "grabbed";
        matchingRelease.GrabbedAt = new Date().toISOString();
      }
      await fulfill(200, {
        release: matchingRelease ?? state.manualSearch.result.Results[0],
      });
      return;
    }

    if (method === "POST" && pathname === "/api/v1/jobs") {
      const payload = JSON.parse(request.postData() ?? "{}") as {
        job_type?: string;
        target_id?: string;
      };
      await fulfill(200, {
        job: {
          ID: `job-${Date.now()}`,
          JobType: payload.job_type ?? "scan",
          TargetID: payload.target_id ?? "unknown",
          State: "queued",
          SubmittedAt: new Date().toISOString(),
          PayloadURI: "",
          IdempotencyKey: "",
          Attempt: 0,
          LastAttemptAt: "",
          CompletedAt: "",
        },
      });
      return;
    }

    if (method === "PUT" && pathname.startsWith("/api/v1/worker-pools/")) {
      const poolId = decodeURIComponent(pathname.split("/").pop() ?? "");
      const payload = JSON.parse(request.postData() ?? "{}") as {
        description?: string;
        stream?: string;
        consumer_group?: string;
        heartbeat_interval_seconds?: number;
        stale_after_seconds?: number;
        max_concurrent_jobs?: number;
      };
      const pool = state.workerPools.pools.find((entry) => entry.pool === poolId);
      if (pool) {
        pool.description = payload.description ?? pool.description;
        pool.stream = payload.stream ?? pool.stream;
        pool.consumer_group = payload.consumer_group ?? pool.consumer_group;
        pool.heartbeat_interval_seconds =
          payload.heartbeat_interval_seconds ?? pool.heartbeat_interval_seconds;
        pool.stale_after_seconds = payload.stale_after_seconds ?? pool.stale_after_seconds;
        pool.max_concurrent_jobs = payload.max_concurrent_jobs ?? pool.max_concurrent_jobs;
      }
      await fulfill(200, {
        pool: pool ?? state.workerPools.pools[0],
      });
      return;
    }

    await fulfill(404, {
      error: `Unhandled mock for ${method} ${pathname}`,
    });
  });

  return { metrics, state };
}
