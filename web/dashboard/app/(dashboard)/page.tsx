"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { PageHeader } from "../../components/layout/PageHeader";
import { useEnvironment } from "../../components/environment/EnvironmentContext";
import { useAuth } from "../../components/auth/AuthContext";
import { useRestClient } from "../../hooks/useRestClient";
import { useRestQuery } from "../../hooks/useRestQuery";
import { useGrpcQuery } from "../../hooks/useGrpcQuery";
import type {
  DownloadHistoryEntry,
  JobSummary,
  LibraryEntity,
  MetadataComparison,
  ReadingList,
  WorkerPoolConfig,
  WorkerSummary,
  ObservabilitySnapshot,
  ConfigurationSummary,
} from "../../lib/api";
import { LibraryHealthPanel } from "./components/LibraryHealthPanel";
import { ManualSearchPanel } from "./components/ManualSearchPanel";
import { JobQueuePanel } from "./components/JobQueuePanel";
import { WorkersPanel } from "./components/WorkersPanel";
import { WorkerPoolPanel } from "./components/WorkerPoolPanel";
import { ReadarrExperiencePanel } from "./components/ReadarrExperiencePanel";
import { ObservabilityPanel } from "./components/ObservabilityPanel";
import { ConfigurationPanel } from "./components/ConfigurationPanel";
import styles from "./page.module.css";

export default function DashboardPage() {
  const { current } = useEnvironment();
  const { status: authStatus, hasPermission } = useAuth();
  const searchParams = useSearchParams();
  const apiBaseUrl = current.apiUrl;
  const focusEntityId = searchParams.get("focusEntity");

  const isAuthenticated = authStatus === "authenticated";
  const canReadLibrary = isAuthenticated && hasPermission("library:read");
  const canReadWorkers = isAuthenticated && hasPermission("worker:read");
  const canWriteLibrary = isAuthenticated && hasPermission("library:write");
  const canManageWorkers = isAuthenticated && hasPermission("worker:manage");
  const canViewObservability =
    isAuthenticated &&
    (hasPermission("observability:read") ||
      hasPermission("worker:read") ||
      hasPermission("library:read"));
  const canViewConfiguration =
    isAuthenticated &&
    (hasPermission("config:read") || hasPermission("worker:manage"));

  const queryClient = useQueryClient();
  const restClient = useRestClient();

  const libraryQueryKey = useMemo(
    () => ["library", "entities", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const jobsQueryKey = useMemo(
    () => ["library", "jobs", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const workersQueryKey = useMemo(
    () => ["workers", "summary", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const poolsQueryKey = useMemo(
    () => ["workers", "pools", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const observabilityQueryKey = useMemo(
    () => ["observability", "snapshot", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const configurationQueryKey = useMemo(
    () => ["configuration", "summary", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const readingListsQueryKey = useMemo(
    () => ["library", "reading-lists", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const metadataComparisonsQueryKey = useMemo(
    () => ["library", "metadata-comparisons", apiBaseUrl] as const,
    [apiBaseUrl],
  );
  const downloadHistoryQueryKey = useMemo(
    () => ["library", "download-history", apiBaseUrl] as const,
    [apiBaseUrl],
  );

  const libraryQuery = useRestQuery<LibraryEntity[]>(
    libraryQueryKey,
    (client) => client.fetchEntities(),
    {
      enabled: canReadLibrary,
    },
  );
  const jobsQuery = useRestQuery<JobSummary[]>(
    jobsQueryKey,
    (client) => client.fetchJobs(),
    {
      enabled: canReadLibrary,
    },
  );
  const workersQuery = useRestQuery<WorkerSummary[]>(
    workersQueryKey,
    (client) => client.fetchWorkers(),
    {
      enabled: canReadWorkers,
      refetchInterval: 30000,
    },
  );
  const poolsQuery = useRestQuery<WorkerPoolConfig[]>(
    poolsQueryKey,
    (client) => client.fetchWorkerPools(),
    {
      enabled: canReadWorkers,
    },
  );
  const readingListsQuery = useRestQuery<ReadingList[]>(
    readingListsQueryKey,
    (client) => client.fetchReadingLists(),
    {
      enabled: canReadLibrary,
    },
  );
  const metadataComparisonsQuery = useRestQuery<MetadataComparison[]>(
    metadataComparisonsQueryKey,
    (client) => client.fetchMetadataComparisons(),
    {
      enabled: canReadLibrary,
    },
  );
  const downloadHistoryQuery = useRestQuery<DownloadHistoryEntry[]>(
    downloadHistoryQueryKey,
    (client) => client.fetchDownloadHistory(),
    {
      enabled: canReadLibrary,
    },
  );

  const observabilityQuery = useRestQuery<ObservabilitySnapshot>(
    observabilityQueryKey,
    (client) => client.fetchObservabilitySnapshot(),
    {
      enabled: canViewObservability,
      refetchInterval: 30000,
    },
  );

  const configurationQuery = useRestQuery<ConfigurationSummary>(
    configurationQueryKey,
    (client) => client.fetchConfigurationSummary(),
    {
      enabled: canViewConfiguration,
      refetchInterval: 60000,
    },
  );

  const grpcLibraryQuery = useGrpcQuery<LibraryEntity[]>(
    ["grpc", "library", apiBaseUrl],
    (client) => client.listLibraryEntities(),
    {
      enabled: canReadLibrary,
      errorRetryCount: 0,
    },
  );

  useEffect(() => {
    if (grpcLibraryQuery.data) {
      queryClient.setQueryData(libraryQueryKey, grpcLibraryQuery.data);
    }
  }, [grpcLibraryQuery.data, libraryQueryKey, queryClient]);

  const {
    data: libraryData,
    isPending: libraryPending,
    error: libraryErrorObj,
    refetch: refetchLibrary,
  } = libraryQuery;
  const libraryError = libraryErrorObj?.message;
  const libraryLoading = libraryPending && !libraryData;
  const refreshLibrary = useCallback(() => {
    void refetchLibrary();
  }, [refetchLibrary]);

  const {
    data: jobsData,
    isPending: jobsPending,
    error: jobsErrorObj,
    refetch: refetchJobs,
  } = jobsQuery;
  const jobsError = jobsErrorObj?.message;
  const jobsLoading = jobsPending && !jobsData;
  const refreshJobs = useCallback(() => {
    void refetchJobs();
  }, [refetchJobs]);

  const {
    data: workersData,
    isPending: workersPending,
    error: workersErrorObj,
    refetch: refetchWorkers,
  } = workersQuery;
  const workersError = workersErrorObj?.message;
  const workersLoading = workersPending && !workersData;
  const refreshWorkers = useCallback(() => {
    void refetchWorkers();
  }, [refetchWorkers]);

  const {
    data: poolsData,
    isPending: poolsPending,
    error: poolsErrorObj,
    refetch: refetchPools,
  } = poolsQuery;
  const poolsError = poolsErrorObj?.message;
  const poolsLoading = poolsPending && !poolsData;
  const refreshPools = useCallback(() => {
    void refetchPools();
  }, [refetchPools]);

  const {
    data: readingListsData,
    isPending: readingListsPending,
    error: readingListsErrorObj,
    refetch: refetchReadingLists,
  } = readingListsQuery;
  const readingListsError = readingListsErrorObj?.message;
  const readingListsLoading = readingListsPending && !readingListsData;
  const refreshReadingLists = useCallback(() => {
    void refetchReadingLists();
  }, [refetchReadingLists]);

  const {
    data: metadataComparisonsData,
    isPending: metadataComparisonsPending,
    error: metadataComparisonsErrorObj,
    refetch: refetchMetadataComparisons,
  } = metadataComparisonsQuery;
  const metadataComparisonsError = metadataComparisonsErrorObj?.message;
  const metadataComparisonsLoading =
    metadataComparisonsPending && !metadataComparisonsData;
  const refreshMetadataComparisons = useCallback(() => {
    void refetchMetadataComparisons();
  }, [refetchMetadataComparisons]);

  const {
    data: downloadHistoryData,
    isPending: downloadHistoryPending,
    error: downloadHistoryErrorObj,
    refetch: refetchDownloadHistory,
  } = downloadHistoryQuery;
  const downloadHistoryError = downloadHistoryErrorObj?.message;
  const downloadHistoryLoading = downloadHistoryPending && !downloadHistoryData;
  const refreshDownloadHistory = useCallback(() => {
    void refetchDownloadHistory();
  }, [refetchDownloadHistory]);

  const {
    data: observabilityData,
    isPending: observabilityPending,
    error: observabilityErrorObj,
    refetch: refetchObservability,
  } = observabilityQuery;
  const observabilityError = observabilityErrorObj?.message;
  const observabilityLoading = observabilityPending && !observabilityData;
  const refreshObservability = useCallback(() => {
    void refetchObservability();
  }, [refetchObservability]);

  const {
    data: configurationData,
    isPending: configurationPending,
    error: configurationErrorObj,
    refetch: refetchConfiguration,
  } = configurationQuery;
  const configurationError = configurationErrorObj?.message;
  const configurationLoading = configurationPending && !configurationData;
  const refreshConfiguration = useCallback(() => {
    void refetchConfiguration();
  }, [refetchConfiguration]);

  const readarrLoading =
    readingListsLoading ||
    metadataComparisonsLoading ||
    downloadHistoryLoading;
  const readarrError =
    readingListsError ?? metadataComparisonsError ?? downloadHistoryError;
  const refreshReadarr = useCallback(() => {
    refreshReadingLists();
    refreshMetadataComparisons();
    refreshDownloadHistory();
  }, [
    refreshDownloadHistory,
    refreshMetadataComparisons,
    refreshReadingLists,
  ]);

  const aggregateLoading =
    libraryLoading ||
    jobsLoading ||
    workersLoading ||
    poolsLoading ||
    readarrLoading ||
    (canViewObservability && observabilityLoading) ||
    (canViewConfiguration && configurationLoading);

  const handleRefreshAll = useCallback(() => {
    refreshLibrary();
    refreshJobs();
    refreshWorkers();
    refreshPools();
    refreshReadarr();
    if (canViewObservability) {
      refreshObservability();
    }
    if (canViewConfiguration) {
      refreshConfiguration();
    }
  }, [
    refreshJobs,
    refreshLibrary,
    refreshPools,
    refreshReadarr,
    refreshWorkers,
    refreshObservability,
    refreshConfiguration,
    canViewObservability,
    canViewConfiguration,
  ]);

  useEffect(() => {
    if (!focusEntityId || libraryLoading || !(libraryData?.length ?? 0)) {
      return;
    }
    if (typeof window === "undefined") {
      return;
    }
    const escapeSelector = (value: string) => {
      if (window.CSS && typeof window.CSS.escape === "function") {
        return window.CSS.escape(value);
      }
      return value.replace(/"/g, '\\"').replace(/\\/g, "\\\\");
    };
    const selector = `[data-entity-id="${escapeSelector(focusEntityId)}"]`;
    const element = document.querySelector<HTMLElement>(selector);
    if (element) {
      element.scrollIntoView({ behavior: "smooth", block: "center" });
      if (typeof element.focus === "function") {
        element.focus({ preventScroll: true });
      }
    }
  }, [focusEntityId, libraryData, libraryLoading]);

  return (
    <div className={styles.page}>
      <PageHeader
        eyebrow="arrgo"
        title="Cluster overview"
        description="Stay on top of library health, orchestration jobs, and worker availability across your arrgo deployment."
        actions={
          <button
            className={styles.refreshButton}
            disabled={aggregateLoading}
            onClick={handleRefreshAll}
            type="button"
          >
            {aggregateLoading ? "Refreshing…" : "Refresh data"}
          </button>
        }
      />
      <div className={styles.grid}>
        <LibraryHealthPanel
          data={libraryData}
          loading={libraryLoading}
          error={libraryError}
          onRetry={refreshLibrary}
          restClient={restClient}
          canWriteLibrary={canWriteLibrary}
          onJobsRefetch={refreshJobs}
          focusedEntityId={focusEntityId ?? undefined}
        />
        <ManualSearchPanel
          data={libraryData}
          restClient={restClient}
          canWriteLibrary={canWriteLibrary}
          focusedEntityId={focusEntityId ?? undefined}
        />
        <ReadarrExperiencePanel
          readingLists={readingListsData}
          metadataComparisons={metadataComparisonsData}
          downloadHistory={downloadHistoryData}
          loading={readarrLoading}
          error={readarrError}
          onRetry={refreshReadarr}
        />
        <JobQueuePanel
          data={jobsData}
          loading={jobsLoading}
          error={jobsError}
          onRetry={refreshJobs}
          restClient={restClient}
          libraryEntities={libraryData}
        />
        <WorkersPanel
          data={workersData}
          loading={workersLoading}
          error={workersError}
          onRetry={refreshWorkers}
        />
        <WorkerPoolPanel
          pools={poolsData}
          workers={workersData}
          loading={poolsLoading}
          error={poolsError}
          onRetry={refreshPools}
          restClient={restClient}
          canManageWorkers={canManageWorkers}
          onPoolsRefetch={refreshPools}
        />
        {canViewObservability ? (
          <ObservabilityPanel
            snapshot={observabilityData}
            loading={observabilityLoading}
            error={observabilityError}
            onRetry={refreshObservability}
          />
        ) : null}
        {canViewConfiguration ? (
          <ConfigurationPanel
            summary={configurationData}
            loading={configurationLoading}
            error={configurationError}
            onRetry={refreshConfiguration}
          />
        ) : null}
      </div>
    </div>
  );
}
