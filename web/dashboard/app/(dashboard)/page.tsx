"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
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
} from "../../lib/api";
import { LibraryHealthPanel } from "./components/LibraryHealthPanel";
import { ManualSearchPanel } from "./components/ManualSearchPanel";
import { JobQueuePanel } from "./components/JobQueuePanel";
import { WorkersPanel } from "./components/WorkersPanel";
import { WorkerPoolPanel } from "./components/WorkerPoolPanel";
import { ReadarrExperiencePanel } from "./components/ReadarrExperiencePanel";
import styles from "./page.module.css";

export default function DashboardPage() {
  const { current } = useEnvironment();
  const { status: authStatus, hasPermission } = useAuth();
  const apiBaseUrl = current.apiUrl;

  const isAuthenticated = authStatus === "authenticated";
  const canReadLibrary = isAuthenticated && hasPermission("library:read");
  const canReadWorkers = isAuthenticated && hasPermission("worker:read");
  const canWriteLibrary = isAuthenticated && hasPermission("library:write");
  const canManageWorkers = isAuthenticated && hasPermission("worker:manage");

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
    readarrLoading;

  const handleRefreshAll = useCallback(() => {
    refreshLibrary();
    refreshJobs();
    refreshWorkers();
    refreshPools();
    refreshReadarr();
  }, [
    refreshJobs,
    refreshLibrary,
    refreshPools,
    refreshReadarr,
    refreshWorkers,
  ]);

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
        />
        <ManualSearchPanel
          data={libraryData}
          restClient={restClient}
          canWriteLibrary={canWriteLibrary}
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
      </div>
    </div>
  );
}
