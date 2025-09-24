import { HttpClient } from "./httpClient";
import {
  finishTrace,
  startClientTrace,
  TraceAttributes,
} from "./observability/tracing";

const FALLBACK_BASE_URL = "http://localhost:8080";

export type TokenProvider = () => string | undefined;

const defaultTokenProvider: TokenProvider = () =>
  process.env.NEXT_PUBLIC_ARRGO_API_TOKEN?.trim() || undefined;

let resolveApiToken: TokenProvider = defaultTokenProvider;

export function setApiTokenProvider(provider?: TokenProvider) {
  resolveApiToken = provider ?? defaultTokenProvider;
}

function resolveBaseUrl(baseUrl?: string): string {
  const value = baseUrl ?? process.env.NEXT_PUBLIC_ARRGO_API_URL ?? FALLBACK_BASE_URL;
  return value.replace(/\/$/, "");
}

export interface LibraryEntity {
  id: string;
  entityType: string;
  title: string;
  synopsis: string;
}

export interface JobSummary {
  id: string;
  jobType: string;
  targetID: string;
  state: string;
  submittedAt?: string;
  payloadURI?: string;
  idempotencyKey?: string;
  attempt: number;
  lastAttemptAt?: string;
  completedAt?: string;
}

export interface WorkerSummary {
  id: string;
  pool: string;
  capabilities: string[];
  endpoint: string;
  version: string;
  status: string;
  registeredAt?: string;
  lastHeartbeatAt?: string;
  activeJobs: number;
}

export interface ManualSearchRelease {
  id: string;
  searchID: string;
  entityID: string;
  title: string;
  indexer: string;
  protocol: string;
  quality: string;
  publishDate?: string;
  sizeBytes: number;
  seeders: number;
  grabs: number;
  score: number;
  status: string;
  rejectionReason?: string;
  downloadURL: string;
  grabbedAt?: string;
}

export interface ManualSearchResult {
  id: string;
  entityID: string;
  query: string;
  executedAt?: string;
  results: ManualSearchRelease[];
}

export interface TriggerJobInput {
  jobType: string;
  targetID: string;
  payloadURI?: string;
  idempotencyKey?: string;
}

export interface WorkerPoolConfig {
  pool: string;
  description?: string;
  stream: string;
  consumerGroup: string;
  heartbeatIntervalSeconds: number;
  staleAfterSeconds: number;
  maxConcurrentJobs?: number;
}

export interface UpdateWorkerPoolInput {
  description?: string;
  stream?: string;
  consumerGroup?: string;
  heartbeatIntervalSeconds?: number;
  staleAfterSeconds?: number;
  maxConcurrentJobs?: number;
}

export interface PrincipalRole {
  name: string;
  description?: string;
  permissions: string[];
}

export interface Principal {
  key: {
    id: string;
    label: string;
    role: string;
    version: number;
    active: boolean;
    createdAt?: string;
    lastRotatedAt?: string;
    lastUsedAt?: string;
    revokedAt?: string;
  };
  role: PrincipalRole;
  permissions: string[];
}

export interface EpisodeCutoverOverride {
  episodeID: string;
  seasonNumber: number;
  episodeNumber: number;
  absoluteNumber?: number;
  title?: string;
  airDate?: string;
  notes?: string;
}

export interface AlbumEditionOverride {
  editionID: string;
  name: string;
  releaseDate?: string;
  isDefault?: boolean;
  notes?: string;
}

export interface TrackNumberOverride {
  trackID: string;
  discNumber?: number;
  trackNumber: number;
  totalTracks?: number;
  isrc?: string;
  notes?: string;
}

export interface MetadataOverrides {
  episodeCutovers: EpisodeCutoverOverride[];
  albumEditions: AlbumEditionOverride[];
  trackNumberOverrides: TrackNumberOverride[];
  updatedAt?: string;
}

export interface EpisodeTemplateSet {
  seriesFolderPattern: string;
  seasonFolderPattern: string;
  episodeFilePattern: string;
}

export interface MovieTemplateSet {
  movieFolderPattern: string;
  movieFilePattern: string;
}

export interface MusicTemplateSet {
  artistFolderPattern: string;
  albumFolderPattern: string;
  trackFilePattern: string;
}

export interface RenameOperation {
  fileID: string;
  workType: string;
  sourcePath: string;
  targetPath: string;
  shouldRename: boolean;
  reason: string;
  lastRenamedAt?: string;
  lastRenamedJobID?: string;
}

export interface RenameJobRecord {
  id: string;
  entityID: string;
  entityType: string;
  requestedAt?: string;
  dryRun: boolean;
  episodeTemplate?: EpisodeTemplateSet;
  movieTemplate?: MovieTemplateSet;
  musicTemplate?: MusicTemplateSet;
  operations: RenameOperation[];
  changes: number;
}

interface RawEntity {
  ID: string;
  EntityType: string;
  Title: string;
  Synopsis: string;
}

interface RawJob {
  ID: string;
  JobType: string;
  TargetID: string;
  State: string;
  SubmittedAt?: string;
  PayloadURI?: string;
  IdempotencyKey?: string;
  Attempt: number;
  LastAttemptAt?: string;
  CompletedAt?: string;
}

interface RawWorker {
  id: string;
  pool: string;
  capabilities: string[];
  endpoint: string;
  version: string;
  status: string;
  registered_at?: string;
  last_heartbeat_at?: string;
  active_jobs: number;
}

interface RawWorkerPool {
  pool: string;
  description?: string;
  stream: string;
  consumer_group: string;
  heartbeat_interval_seconds: number;
  stale_after_seconds: number;
  max_concurrent_jobs?: number;
}

interface RawManualSearchRelease {
  ID: string;
  SearchID: string;
  EntityID: string;
  Title: string;
  Indexer: string;
  Protocol: string;
  Quality: string;
  PublishDate?: string;
  SizeBytes: number;
  Seeders: number;
  Grabs: number;
  Score: number;
  Status: string;
  RejectionReason?: string;
  DownloadURL: string;
  GrabbedAt?: string;
}

interface RawManualSearchResult {
  ID: string;
  EntityID: string;
  Query: string;
  ExecutedAt?: string;
  Results: RawManualSearchRelease[];
}

interface RawEpisodeCutoverOverride {
  episode_id: string;
  season_number: number;
  episode_number: number;
  absolute_number?: number;
  title?: string;
  air_date?: string;
  notes?: string;
}

interface RawAlbumEditionOverride {
  edition_id: string;
  name: string;
  release_date?: string;
  is_default?: boolean;
  notes?: string;
}

interface RawTrackNumberOverride {
  track_id: string;
  disc_number?: number;
  track_number: number;
  total_tracks?: number;
  isrc?: string;
  notes?: string;
}

interface RawMetadataOverrides {
  episode_cutovers?: RawEpisodeCutoverOverride[];
  album_editions?: RawAlbumEditionOverride[];
  track_number_overrides?: RawTrackNumberOverride[];
  updated_at?: string;
}

interface RawEpisodeTemplateSet {
  SeriesFolderPattern?: string;
  SeasonFolderPattern?: string;
  EpisodeFilePattern?: string;
}

interface RawMovieTemplateSet {
  MovieFolderPattern?: string;
  MovieFilePattern?: string;
}

interface RawMusicTemplateSet {
  ArtistFolderPattern?: string;
  AlbumFolderPattern?: string;
  TrackFilePattern?: string;
}

interface RawRenameOperation {
  FileID: string;
  WorkType: string;
  SourcePath: string;
  TargetPath: string;
  ShouldRename: boolean;
  Reason: string;
  LastRenamedAt?: string;
  LastRenamedJobID?: string;
}

interface RawRenameJobRecord {
  ID: string;
  EntityID: string;
  EntityType: string;
  RequestedAt?: string;
  DryRun: boolean;
  EpisodeTemplate?: RawEpisodeTemplateSet | null;
  MovieTemplate?: RawMovieTemplateSet | null;
  MusicTemplate?: RawMusicTemplateSet | null;
  Operations: RawRenameOperation[];
  Changes: number;
}

interface RawPrincipal {
  key: {
    id: string;
    label: string;
    role: string;
    version: number;
    active: boolean;
    created_at?: string;
    last_rotated_at?: string;
    last_used_at?: string;
    revoked_at?: string;
  };
  role: {
    name: string;
    description?: string;
    permissions?: string[];
  };
  permissions?: string[];
}

function normalizeTimestamp(value?: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const trimmed = value.trim();
  if (!trimmed || trimmed.startsWith("0001-01-01")) {
    return undefined;
  }
  return trimmed;
}

function mapLibraryEntity(entity: RawEntity): LibraryEntity {
  return {
    id: entity.ID,
    entityType: entity.EntityType,
    title: entity.Title,
    synopsis: entity.Synopsis,
  };
}

function mapJobSummary(job: RawJob): JobSummary {
  return {
    id: job.ID,
    jobType: job.JobType,
    targetID: job.TargetID,
    state: job.State,
    submittedAt: normalizeTimestamp(job.SubmittedAt),
    payloadURI: job.PayloadURI?.trim() || undefined,
    idempotencyKey: job.IdempotencyKey?.trim() || undefined,
    attempt: job.Attempt ?? 0,
    lastAttemptAt: normalizeTimestamp(job.LastAttemptAt),
    completedAt: normalizeTimestamp(job.CompletedAt),
  };
}

function mapWorkerSummary(worker: RawWorker): WorkerSummary {
  return {
    id: worker.id,
    pool: worker.pool,
    capabilities: worker.capabilities ?? [],
    endpoint: worker.endpoint,
    version: worker.version,
    status: worker.status,
    registeredAt: normalizeTimestamp(worker.registered_at),
    lastHeartbeatAt: normalizeTimestamp(worker.last_heartbeat_at),
    activeJobs: worker.active_jobs,
  };
}

function mapWorkerPool(pool: RawWorkerPool): WorkerPoolConfig {
  return {
    pool: pool.pool,
    description: pool.description?.trim() || undefined,
    stream: pool.stream,
    consumerGroup: pool.consumer_group,
    heartbeatIntervalSeconds: pool.heartbeat_interval_seconds ?? 0,
    staleAfterSeconds: pool.stale_after_seconds ?? 0,
    maxConcurrentJobs: pool.max_concurrent_jobs ?? undefined,
  };
}

function normalizeManualSearchRelease(
  release: RawManualSearchRelease,
): ManualSearchRelease {
  return {
    id: release.ID,
    searchID: release.SearchID,
    entityID: release.EntityID,
    title: release.Title,
    indexer: release.Indexer,
    protocol: release.Protocol,
    quality: release.Quality,
    publishDate: normalizeTimestamp(release.PublishDate),
    sizeBytes: release.SizeBytes ?? 0,
    seeders: release.Seeders ?? 0,
    grabs: release.Grabs ?? 0,
    score: release.Score ?? 0,
    status: release.Status,
    rejectionReason: release.RejectionReason?.trim() || undefined,
    downloadURL: release.DownloadURL,
    grabbedAt: normalizeTimestamp(release.GrabbedAt),
  };
}

function normalizeManualSearchResult(
  search: RawManualSearchResult,
): ManualSearchResult {
  return {
    id: search.ID,
    entityID: search.EntityID,
    query: search.Query?.trim() || "",
    executedAt: normalizeTimestamp(search.ExecutedAt),
    results: (search.Results ?? []).map(normalizeManualSearchRelease),
  };
}

function mapEpisodeCutoverOverride(
  override: RawEpisodeCutoverOverride,
): EpisodeCutoverOverride {
  return {
    episodeID: override.episode_id,
    seasonNumber: override.season_number ?? 0,
    episodeNumber: override.episode_number ?? 0,
    absoluteNumber: override.absolute_number ?? undefined,
    title: override.title?.trim() || undefined,
    airDate: normalizeTimestamp(override.air_date),
    notes: override.notes?.trim() || undefined,
  };
}

function mapAlbumEditionOverride(
  override: RawAlbumEditionOverride,
): AlbumEditionOverride {
  return {
    editionID: override.edition_id,
    name: override.name?.trim() || "",
    releaseDate: normalizeTimestamp(override.release_date),
    isDefault: override.is_default ?? undefined,
    notes: override.notes?.trim() || undefined,
  };
}

function mapTrackNumberOverride(
  override: RawTrackNumberOverride,
): TrackNumberOverride {
  return {
    trackID: override.track_id,
    discNumber: override.disc_number ?? undefined,
    trackNumber: override.track_number ?? 0,
    totalTracks: override.total_tracks ?? undefined,
    isrc: override.isrc?.trim() || undefined,
    notes: override.notes?.trim() || undefined,
  };
}

function mapMetadataOverrides(
  overrides: RawMetadataOverrides,
): MetadataOverrides {
  return {
    episodeCutovers: (overrides.episode_cutovers ?? []).map(
      mapEpisodeCutoverOverride,
    ),
    albumEditions: (overrides.album_editions ?? []).map(
      mapAlbumEditionOverride,
    ),
    trackNumberOverrides: (overrides.track_number_overrides ?? []).map(
      mapTrackNumberOverride,
    ),
    updatedAt: normalizeTimestamp(overrides.updated_at),
  };
}

function mapEpisodeTemplateSet(
  template: RawEpisodeTemplateSet | null | undefined,
): EpisodeTemplateSet | undefined {
  if (!template) {
    return undefined;
  }
  return {
    seriesFolderPattern: template.SeriesFolderPattern ?? "",
    seasonFolderPattern: template.SeasonFolderPattern ?? "",
    episodeFilePattern: template.EpisodeFilePattern ?? "",
  };
}

function mapMovieTemplateSet(
  template: RawMovieTemplateSet | null | undefined,
): MovieTemplateSet | undefined {
  if (!template) {
    return undefined;
  }
  return {
    movieFolderPattern: template.MovieFolderPattern ?? "",
    movieFilePattern: template.MovieFilePattern ?? "",
  };
}

function mapMusicTemplateSet(
  template: RawMusicTemplateSet | null | undefined,
): MusicTemplateSet | undefined {
  if (!template) {
    return undefined;
  }
  return {
    artistFolderPattern: template.ArtistFolderPattern ?? "",
    albumFolderPattern: template.AlbumFolderPattern ?? "",
    trackFilePattern: template.TrackFilePattern ?? "",
  };
}

function mapRenameOperation(operation: RawRenameOperation): RenameOperation {
  return {
    fileID: operation.FileID,
    workType: operation.WorkType,
    sourcePath: operation.SourcePath,
    targetPath: operation.TargetPath,
    shouldRename: Boolean(operation.ShouldRename),
    reason: operation.Reason?.trim() || "",
    lastRenamedAt: normalizeTimestamp(operation.LastRenamedAt),
    lastRenamedJobID: operation.LastRenamedJobID?.trim() || undefined,
  };
}

function mapRenameJobRecord(record: RawRenameJobRecord): RenameJobRecord {
  return {
    id: record.ID,
    entityID: record.EntityID,
    entityType: record.EntityType,
    requestedAt: normalizeTimestamp(record.RequestedAt),
    dryRun: Boolean(record.DryRun),
    episodeTemplate: mapEpisodeTemplateSet(record.EpisodeTemplate),
    movieTemplate: mapMovieTemplateSet(record.MovieTemplate),
    musicTemplate: mapMusicTemplateSet(record.MusicTemplate),
    operations: (record.Operations ?? []).map(mapRenameOperation),
    changes: record.Changes ?? 0,
  };
}

function normalizePrincipal(payload: RawPrincipal): Principal {
  const key = payload.key ?? {
    id: "",
    label: "",
    role: "",
    version: 0,
    active: false,
  };
  const role = payload.role ?? { name: "", permissions: [] };
  return {
    key: {
      id: key.id,
      label: key.label,
      role: key.role,
      version: key.version ?? 0,
      active: Boolean(key.active),
      createdAt: normalizeTimestamp(key.created_at),
      lastRotatedAt: normalizeTimestamp(key.last_rotated_at),
      lastUsedAt: normalizeTimestamp(key.last_used_at),
      revokedAt: normalizeTimestamp(key.revoked_at),
    },
    role: {
      name: role.name,
      description: role.description?.trim() || undefined,
      permissions: (role.permissions ?? [])
        .map((permission) => permission.trim())
        .filter(Boolean),
    },
    permissions: (payload.permissions ?? [])
      .map((permission) => permission.trim())
      .filter(Boolean),
  };
}

export interface RestApiClientOptions {
  baseUrl?: string;
  tokenProvider?: TokenProvider;
}

export class RestApiClient {
  private readonly options: RestApiClientOptions;

  private readonly baseUrl: string;

  private readonly http: HttpClient;

  constructor(options: RestApiClientOptions = {}) {
    this.options = { ...options };
    this.baseUrl = resolveBaseUrl(options.baseUrl);
    const tokenProvider = options.tokenProvider ?? (() => resolveApiToken());
    this.http = new HttpClient({
      baseUrl: this.baseUrl,
      tokenProvider,
      traceAttributes: { adapter: "rest" },
    });
  }

  getBaseUrl(): string {
    return this.baseUrl;
  }

  withBaseUrl(baseUrl?: string): RestApiClient {
    if (!baseUrl) {
      return this;
    }
    const normalized = resolveBaseUrl(baseUrl);
    if (normalized === this.baseUrl) {
      return this;
    }
    return new RestApiClient({ ...this.options, baseUrl: normalized });
  }

  async fetchEntities(): Promise<LibraryEntity[]> {
    const payload = await this.http.get<{ entities: RawEntity[] }>(
      "/api/v1/library/entities",
      {
        traceName: "REST GET /api/v1/library/entities",
        traceAttributes: { resource: "library.entities" },
      },
    );
    return payload.entities.map(mapLibraryEntity);
  }

  async fetchJobs(): Promise<JobSummary[]> {
    const payload = await this.http.get<{ jobs: RawJob[] }>("/api/v1/jobs", {
      traceName: "REST GET /api/v1/jobs",
      traceAttributes: { resource: "library.jobs" },
    });
    return payload.jobs.map(mapJobSummary);
  }

  async fetchWorkers(): Promise<WorkerSummary[]> {
    const payload = await this.http.get<{ workers: RawWorker[] }>(
      "/api/v1/workers",
      {
        traceName: "REST GET /api/v1/workers",
        traceAttributes: { resource: "worker.status" },
      },
    );
    return payload.workers.map(mapWorkerSummary);
  }

  async triggerJob(input: TriggerJobInput): Promise<JobSummary> {
    const payload: Record<string, string> = {
      job_type: input.jobType,
      target_id: input.targetID,
    };
    const payloadURI = input.payloadURI?.trim();
    if (payloadURI) {
      payload.payload_uri = payloadURI;
    }
    const idempotencyKey = input.idempotencyKey?.trim();
    if (idempotencyKey) {
      payload.idempotency_key = idempotencyKey;
    }

    const response = await this.http.post<{ job: RawJob }>(
      "/api/v1/jobs",
      payload,
      {
        traceName: "REST POST /api/v1/jobs",
        traceAttributes: {
          resource: "library.jobs",
          jobType: input.jobType,
        },
      },
    );
    return mapJobSummary(response.job);
  }

  async fetchWorkerPools(): Promise<WorkerPoolConfig[]> {
    const payload = await this.http.get<{ pools: RawWorkerPool[] }>(
      "/api/v1/worker-pools",
      {
        traceName: "REST GET /api/v1/worker-pools",
        traceAttributes: { resource: "worker.pools" },
      },
    );
    return payload.pools.map(mapWorkerPool);
  }

  async updateWorkerPool(
    pool: string,
    input: UpdateWorkerPoolInput,
  ): Promise<WorkerPoolConfig> {
    const body: Record<string, unknown> = {};
    if (input.description !== undefined) {
      body.description = input.description;
    }
    if (input.stream !== undefined) {
      body.stream = input.stream;
    }
    if (input.consumerGroup !== undefined) {
      body.consumer_group = input.consumerGroup;
    }
    if (input.heartbeatIntervalSeconds !== undefined) {
      body.heartbeat_interval_seconds = input.heartbeatIntervalSeconds;
    }
    if (input.staleAfterSeconds !== undefined) {
      body.stale_after_seconds = input.staleAfterSeconds;
    }
    if (input.maxConcurrentJobs !== undefined) {
      body.max_concurrent_jobs = input.maxConcurrentJobs;
    }

    const response = await this.http.put<{ pool: RawWorkerPool }>(
      `/api/v1/worker-pools/${encodeURIComponent(pool)}`,
      body,
      {
        traceName: "REST PUT /api/v1/worker-pools",
        traceAttributes: { resource: "worker.pools", pool },
      },
    );
    return mapWorkerPool(response.pool);
  }

  async fetchManualSearch(entityID: string): Promise<ManualSearchResult | null> {
    const payload = await this.http.get<{ search: RawManualSearchResult | null }>(
      `/api/v1/indexer/search?entity_id=${encodeURIComponent(entityID)}`,
      {
        traceName: "REST GET /api/v1/indexer/search",
        traceAttributes: { resource: "indexer.manual_search", entityID },
      },
    );
    return payload.search ? normalizeManualSearchResult(payload.search) : null;
  }

  async runManualSearch(
    entityID: string,
    query: string,
  ): Promise<ManualSearchResult> {
    const response = await this.http.post<{ search: RawManualSearchResult }>(
      "/api/v1/indexer/search",
      { entity_id: entityID, query },
      {
        traceName: "REST POST /api/v1/indexer/search",
        traceAttributes: {
          resource: "indexer.manual_search",
          entityID,
        },
      },
    );
    return normalizeManualSearchResult(response.search);
  }

  async grabManualRelease(
    entityID: string,
    releaseID: string,
  ): Promise<ManualSearchRelease> {
    const response = await this.http.post<{ release: RawManualSearchRelease }>(
      "/api/v1/indexer/releases/grab",
      { entity_id: entityID, release_id: releaseID },
      {
        traceName: "REST POST /api/v1/indexer/releases/grab",
        traceAttributes: {
          resource: "indexer.manual_search",
          entityID,
          releaseID,
        },
      },
    );
    return normalizeManualSearchRelease(response.release);
  }

  async fetchMetadataOverrides(
    entityID: string,
  ): Promise<MetadataOverrides | null> {
    const trimmed = entityID.trim();
    if (!trimmed) {
      throw new Error("entityID is required to fetch metadata overrides");
    }
    const payload = await this.http.get<{ overrides: RawMetadataOverrides | null }>(
      `/api/v1/library/overrides/${encodeURIComponent(trimmed)}`,
      {
        traceName: "REST GET /api/v1/library/overrides/{entity}",
        traceAttributes: {
          resource: "library.metadata_overrides",
          entityID: trimmed,
        },
      },
    );
    return payload.overrides ? mapMetadataOverrides(payload.overrides) : null;
  }

  async fetchRenameHistory(
    entityID: string,
    limit?: number,
  ): Promise<RenameJobRecord[]> {
    const trimmed = entityID.trim();
    if (!trimmed) {
      throw new Error("entityID is required to fetch rename history");
    }
    const params = new URLSearchParams();
    if (typeof limit === "number" && limit > 0) {
      params.set("limit", String(limit));
    }
    const query = params.toString();
    const path = `/api/v1/library/entities/${encodeURIComponent(trimmed)}/rename-history${
      query ? `?${query}` : ""
    }`;
    const payload = await this.http.get<{ history?: RawRenameJobRecord[] | null }>(
      path,
      {
        traceName: "REST GET /api/v1/library/entities/{entity}/rename-history",
        traceAttributes: {
          resource: "library.rename_history",
          entityID: trimmed,
        },
      },
    );
    const history = payload.history ?? [];
    return history.map(mapRenameJobRecord);
  }

  async fetchPrincipal(): Promise<Principal> {
    const payload = await this.http.get<{ principal: RawPrincipal }>(
      "/api/v1/auth/principal",
      {
        traceName: "REST GET /api/v1/auth/principal",
        traceAttributes: { resource: "auth.principal" },
      },
    );
    return normalizePrincipal(payload.principal);
  }
}

const defaultRestClient = new RestApiClient();

export function createRestApiClient(options?: RestApiClientOptions): RestApiClient {
  return new RestApiClient(options);
}

export function fetchEntities(baseUrl?: string): Promise<LibraryEntity[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchEntities();
}

export function fetchJobs(baseUrl?: string): Promise<JobSummary[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchJobs();
}

export function fetchWorkers(baseUrl?: string): Promise<WorkerSummary[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchWorkers();
}

export function triggerJob(
  input: TriggerJobInput,
  baseUrl?: string,
): Promise<JobSummary> {
  return defaultRestClient.withBaseUrl(baseUrl).triggerJob(input);
}

export function fetchWorkerPools(baseUrl?: string): Promise<WorkerPoolConfig[]> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchWorkerPools();
}

export function updateWorkerPool(
  pool: string,
  input: UpdateWorkerPoolInput,
  baseUrl?: string,
): Promise<WorkerPoolConfig> {
  return defaultRestClient.withBaseUrl(baseUrl).updateWorkerPool(pool, input);
}

export function fetchManualSearch(
  entityID: string,
  baseUrl?: string,
): Promise<ManualSearchResult | null> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchManualSearch(entityID);
}

export function runManualSearch(
  entityID: string,
  query: string,
  baseUrl?: string,
): Promise<ManualSearchResult> {
  return defaultRestClient.withBaseUrl(baseUrl).runManualSearch(entityID, query);
}

export function grabManualRelease(
  entityID: string,
  releaseID: string,
  baseUrl?: string,
): Promise<ManualSearchRelease> {
  return defaultRestClient
    .withBaseUrl(baseUrl)
    .grabManualRelease(entityID, releaseID);
}

export function fetchMetadataOverrides(
  entityID: string,
  baseUrl?: string,
): Promise<MetadataOverrides | null> {
  return defaultRestClient
    .withBaseUrl(baseUrl)
    .fetchMetadataOverrides(entityID);
}

export function fetchRenameHistory(
  entityID: string,
  limit?: number,
  baseUrl?: string,
): Promise<RenameJobRecord[]> {
  return defaultRestClient
    .withBaseUrl(baseUrl)
    .fetchRenameHistory(entityID, limit);
}

export function fetchPrincipal(baseUrl?: string): Promise<Principal> {
  return defaultRestClient.withBaseUrl(baseUrl).fetchPrincipal();
}

export interface GrpcLibraryClientOptions extends RestApiClientOptions {}

export class GrpcLibraryClient {
  private readonly options: GrpcLibraryClientOptions;

  private readonly restClient: RestApiClient;

  constructor(options: GrpcLibraryClientOptions = {}) {
    this.options = { ...options };
    this.restClient = new RestApiClient(options);
  }

  getBaseUrl(): string {
    return this.restClient.getBaseUrl();
  }

  withBaseUrl(baseUrl?: string): GrpcLibraryClient {
    if (!baseUrl) {
      return this;
    }
    const normalized = resolveBaseUrl(baseUrl);
    if (normalized === this.getBaseUrl()) {
      return this;
    }
    return new GrpcLibraryClient({ ...this.options, baseUrl: normalized });
  }

  private async tracedCall<T>(
    method: string,
    executor: () => Promise<T>,
    successAttributes?: (result: T) => TraceAttributes,
  ): Promise<T> {
    const trace = startClientTrace(`gRPC ${method}`, {
      adapter: "grpc",
      transport: "grpc",
      service: "arrgo.api.v1.LibraryService",
      method,
      baseUrl: this.getBaseUrl(),
    });
    try {
      const result = await executor();
      const extra = successAttributes ? successAttributes(result) : undefined;
      finishTrace(trace, "success", extra);
      return result;
    } catch (error) {
      finishTrace(trace, "error", undefined, error);
      throw error;
    }
  }

  listLibraryEntities(): Promise<LibraryEntity[]> {
    return this.tracedCall(
      "LibraryService/ListLibraryEntities",
      () => this.restClient.fetchEntities(),
      (entities) => ({ resultCount: entities.length }),
    );
  }

  listJobs(): Promise<JobSummary[]> {
    return this.tracedCall(
      "LibraryService/ListJobs",
      () => this.restClient.fetchJobs(),
      (jobs) => ({ resultCount: jobs.length }),
    );
  }

  triggerJob(input: TriggerJobInput): Promise<JobSummary> {
    return this.tracedCall(
      "LibraryService/TriggerJob",
      () => this.restClient.triggerJob(input),
      (job) => ({ jobType: job.jobType }),
    );
  }
}

const defaultGrpcClient = new GrpcLibraryClient();

export function createGrpcLibraryClient(
  options?: GrpcLibraryClientOptions,
): GrpcLibraryClient {
  return new GrpcLibraryClient(options);
}

export { defaultRestClient as restClient, defaultGrpcClient as grpcLibraryClient };
