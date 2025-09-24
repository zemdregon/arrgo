"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DataPanel } from "../../../components/DataPanel";
import type {
  AlbumEditionOverride,
  EpisodeCutoverOverride,
  LibraryEntity,
  MetadataOverrides,
  MetadataOverridesInput,
  ReleaseProfile,
  RestApiClient,
  RetentionPolicy,
  RetentionTargetOverride,
  RetentionWorkflow,
  TrackNumberOverride,
  UpsertReleaseProfileInput,
  UpsertRetentionWorkflowInput,
} from "../../../lib/api";
import styles from "./PolicyManagementPanel.module.css";

type PolicyTab = "release" | "retention" | "overrides";

const TAB_LABELS: Record<PolicyTab, string> = {
  release: "Release profiles",
  retention: "Retention rules",
  overrides: "Metadata overrides",
};

interface PolicyManagementPanelProps {
  restClient: RestApiClient;
  apiBaseUrl: string;
  canEditReleaseProfiles: boolean;
  canEditRetention: boolean;
  canEditOverrides: boolean;
  libraryEntities?: LibraryEntity[];
}

export function PolicyManagementPanel({
  restClient,
  apiBaseUrl,
  canEditReleaseProfiles,
  canEditRetention,
  canEditOverrides,
  libraryEntities,
}: PolicyManagementPanelProps) {
  const [activeTab, setActiveTab] = useState<PolicyTab>("release");
  const queryClient = useQueryClient();

  const releaseProfilesKey = useMemo(
    () => ["policies", "release-profiles", apiBaseUrl],
    [apiBaseUrl],
  );
  const retentionKey = useMemo(
    () => ["policies", "retention", apiBaseUrl],
    [apiBaseUrl],
  );

  const releaseProfilesQuery = useQuery({
    queryKey: releaseProfilesKey,
    queryFn: () => restClient.fetchReleaseProfiles(),
  });

  const retentionQuery = useQuery({
    queryKey: retentionKey,
    queryFn: () => restClient.fetchRetentionWorkflows(),
  });

  const releaseProfiles = releaseProfilesQuery.data ?? [];
  const retentionWorkflows = retentionQuery.data ?? [];

  const activeError =
    activeTab === "release"
      ? releaseProfilesQuery.error
      : activeTab === "retention"
        ? retentionQuery.error
        : undefined;
  const errorMessage =
    activeError instanceof Error
      ? activeError.message
      : activeError
        ? String(activeError)
        : undefined;

  const loadingState =
    activeTab === "release"
      ? releaseProfilesQuery.isPending
      : activeTab === "retention"
        ? retentionQuery.isPending
        : false;

  const handleRetry = useCallback(() => {
    if (activeTab === "release") {
      void releaseProfilesQuery.refetch();
    } else if (activeTab === "retention") {
      void retentionQuery.refetch();
    }
  }, [activeTab, releaseProfilesQuery, retentionQuery]);

  const handleProfileUpsert = useCallback(
    (profile: ReleaseProfile) => {
      queryClient.setQueryData<ReleaseProfile[]>(releaseProfilesKey, (current) => {
        const next = (current ?? []).slice();
        const index = next.findIndex((item) => item.id === profile.id);
        if (index >= 0) {
          next[index] = profile;
        } else {
          next.push(profile);
        }
        return next;
      });
    },
    [queryClient, releaseProfilesKey],
  );

  const handleProfileDelete = useCallback(
    (profileID: string) => {
      queryClient.setQueryData<ReleaseProfile[]>(releaseProfilesKey, (current) =>
        (current ?? []).filter((profile) => profile.id !== profileID),
      );
    },
    [queryClient, releaseProfilesKey],
  );

  const handleRetentionUpsert = useCallback(
    (workflow: RetentionWorkflow) => {
      queryClient.setQueryData<RetentionWorkflow[]>(retentionKey, (current) => {
        const next = (current ?? []).slice();
        const index = next.findIndex((item) => item.name === workflow.name);
        if (index >= 0) {
          next[index] = workflow;
        } else {
          next.push(workflow);
        }
        return next;
      });
    },
    [queryClient, retentionKey],
  );

  const handleRetentionDelete = useCallback(
    (workflowName: string) => {
      queryClient.setQueryData<RetentionWorkflow[]>(retentionKey, (current) =>
        (current ?? []).filter((workflow) => workflow.name !== workflowName),
      );
    },
    [queryClient, retentionKey],
  );

  return (
    <div className={styles.panelWrapper}>
      <DataPanel
        title="Policy management"
        description="Manage automation policies for quality scoring, replication retention, and entity-specific metadata overrides."
        loading={loadingState}
        error={errorMessage}
        onRetry={loadingState ? undefined : handleRetry}
        actions={
          <div className={styles.tabList} role="tablist" aria-label="Policy sections">
            {(Object.keys(TAB_LABELS) as PolicyTab[]).map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={activeTab === tab}
                className={
                  activeTab === tab
                    ? `${styles.tabButton} ${styles.tabButtonActive}`
                    : styles.tabButton
                }
                onClick={() => setActiveTab(tab)}
              >
                {TAB_LABELS[tab]}
              </button>
            ))}
          </div>
        }
      >
        <div className={styles.panelContent}>
          {activeTab === "release" ? (
            <ReleaseProfilesSection
              profiles={releaseProfiles}
              restClient={restClient}
              onProfileSaved={handleProfileUpsert}
              onProfileDeleted={handleProfileDelete}
              canEdit={canEditReleaseProfiles}
            />
          ) : null}
          {activeTab === "retention" ? (
            <RetentionRulesSection
              workflows={retentionWorkflows}
              restClient={restClient}
              onWorkflowSaved={handleRetentionUpsert}
              onWorkflowDeleted={handleRetentionDelete}
              canEdit={canEditRetention}
            />
          ) : null}
          {activeTab === "overrides" ? (
            <MetadataOverridesSection
              restClient={restClient}
              libraryEntities={libraryEntities}
              canEdit={canEditOverrides}
            />
          ) : null}
        </div>
      </DataPanel>
    </div>
  );
}

interface ReleaseProfilesSectionProps {
  profiles: ReleaseProfile[];
  restClient: RestApiClient;
  onProfileSaved: (profile: ReleaseProfile) => void;
  onProfileDeleted: (profileID: string) => void;
  canEdit: boolean;
}

function ReleaseProfilesSection({
  profiles,
  restClient,
  onProfileSaved,
  onProfileDeleted,
  canEdit,
}: ReleaseProfilesSectionProps) {
  const [isCreating, setIsCreating] = useState(false);
  const [activeProfileId, setActiveProfileId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [defaultScore, setDefaultScore] = useState("0");
  const [minFormatScore, setMinFormatScore] = useState("");
  const [cutoffFormatScore, setCutoffFormatScore] = useState("");
  const [minUpgradeFormatScore, setMinUpgradeFormatScore] = useState("");
  const [formatsText, setFormatsText] = useState("[]");
  const [preferencesText, setPreferencesText] = useState("[]");
  const [blocklistText, setBlocklistText] = useState("[]");
  const [submitting, setSubmitting] = useState(false);
  const [feedback, setFeedback] = useState<
    | { kind: "success" | "error"; message: string }
    | null
  >(null);

  const activeProfile = useMemo(() => {
    if (isCreating) {
      return null;
    }
    if (!activeProfileId) {
      return null;
    }
    return profiles.find((profile) => profile.id === activeProfileId) ?? null;
  }, [activeProfileId, isCreating, profiles]);

  useEffect(() => {
    if (isCreating) {
      return;
    }
    if (activeProfileId && profiles.some((profile) => profile.id === activeProfileId)) {
      return;
    }
    const nextId = profiles[0]?.id ?? null;
    setActiveProfileId(nextId);
  }, [activeProfileId, isCreating, profiles]);

  const resetToProfile = useCallback(
    (profile: ReleaseProfile | null, creating: boolean) => {
      if (creating || !profile) {
        setName("");
        setDescription("");
        setDefaultScore("0");
        setMinFormatScore("");
        setCutoffFormatScore("");
        setMinUpgradeFormatScore("");
        setFormatsText("[]");
        setPreferencesText("[]");
        setBlocklistText("[]");
      } else {
        setName(profile.name);
        setDescription(profile.description ?? "");
        setDefaultScore(String(profile.defaultScore ?? 0));
        setMinFormatScore(
          profile.minFormatScore !== undefined && profile.minFormatScore !== null
            ? String(profile.minFormatScore)
            : "",
        );
        setCutoffFormatScore(
          profile.cutoffFormatScore !== undefined && profile.cutoffFormatScore !== null
            ? String(profile.cutoffFormatScore)
            : "",
        );
        setMinUpgradeFormatScore(
          profile.minUpgradeFormatScore !== undefined &&
            profile.minUpgradeFormatScore !== null
            ? String(profile.minUpgradeFormatScore)
            : "",
        );
        setFormatsText(JSON.stringify(profile.formats ?? [], null, 2));
        setPreferencesText(JSON.stringify(profile.preferences ?? [], null, 2));
        setBlocklistText(JSON.stringify(profile.blocklist ?? [], null, 2));
      }
      setFeedback(null);
    },
    [],
  );

  useEffect(() => {
    resetToProfile(activeProfile, isCreating);
  }, [activeProfile, isCreating, resetToProfile]);

  const handleCreateNew = useCallback(() => {
    setIsCreating(true);
    setActiveProfileId(null);
    resetToProfile(null, true);
  }, [resetToProfile]);

  const handleSelectProfile = useCallback(
    (event: FormEvent<HTMLSelectElement>) => {
      const selected = event.currentTarget.value;
      setIsCreating(false);
      setActiveProfileId(selected || null);
    },
    [],
  );

  const handleReset = useCallback(() => {
    resetToProfile(activeProfile, isCreating);
  }, [activeProfile, isCreating, resetToProfile]);

  const parseNumberField = useCallback(
    (value: string, label: string, allowEmpty = true): number | null | undefined => {
      const trimmed = value.trim();
      if (!trimmed) {
        return allowEmpty ? null : 0;
      }
      const parsed = Number(trimmed);
      if (Number.isNaN(parsed)) {
        throw new Error(`${label} must be a valid number`);
      }
      return parsed;
    },
    [],
  );

  const parseArrayField = useCallback((value: string, label: string): unknown[] => {
    const trimmed = value.trim();
    if (!trimmed) {
      return [];
    }
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (!Array.isArray(parsed)) {
        throw new Error(`${label} must be a JSON array`);
      }
      return parsed;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : `Invalid JSON for ${label}`;
      throw new Error(message);
    }
  }, []);

  const formId = useId();

  const handleSave = useCallback(async () => {
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to update release profiles.",
      });
      return;
    }

    const trimmedName = name.trim();
    if (!trimmedName) {
      setFeedback({ kind: "error", message: "Profile name is required." });
      return;
    }

    let parsedDefaultScore: number;
    try {
      parsedDefaultScore = Number(defaultScore.trim() || "0");
      if (Number.isNaN(parsedDefaultScore)) {
        throw new Error("Default score must be a valid number.");
      }
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Invalid default score.";
      setFeedback({ kind: "error", message });
      return;
    }

    let parsedMinFormatScore: number | null | undefined;
    let parsedCutoffFormatScore: number | null | undefined;
    let parsedMinUpgradeFormatScore: number | null | undefined;
    try {
      parsedMinFormatScore = parseNumberField(
        minFormatScore,
        "Minimum format score",
      );
      parsedCutoffFormatScore = parseNumberField(
        cutoffFormatScore,
        "Cutoff format score",
      );
      parsedMinUpgradeFormatScore = parseNumberField(
        minUpgradeFormatScore,
        "Upgrade threshold score",
      );
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Invalid numeric field.";
      setFeedback({ kind: "error", message });
      return;
    }

    let parsedFormats: unknown[];
    let parsedPreferences: unknown[];
    let parsedBlocklist: unknown[];
    try {
      parsedFormats = parseArrayField(formatsText, "Custom formats");
      parsedPreferences = parseArrayField(preferencesText, "Preferences");
      parsedBlocklist = parseArrayField(blocklistText, "Blocklist");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Invalid policy structure.";
      setFeedback({ kind: "error", message });
      return;
    }

    const payload: UpsertReleaseProfileInput = {
      name: trimmedName,
      description: description.trim() || undefined,
      defaultScore: parsedDefaultScore,
      minFormatScore: parsedMinFormatScore ?? undefined,
      cutoffFormatScore: parsedCutoffFormatScore ?? undefined,
      minUpgradeFormatScore: parsedMinUpgradeFormatScore ?? undefined,
      formats: parsedFormats,
      preferences: parsedPreferences,
      blocklist: parsedBlocklist,
    };

    setSubmitting(true);
    setFeedback(null);
    try {
      let saved: ReleaseProfile;
      if (isCreating || !activeProfile) {
        saved = await restClient.createReleaseProfile(payload);
        setActiveProfileId(saved.id);
        setIsCreating(false);
      } else {
        saved = await restClient.updateReleaseProfile(activeProfile.id, payload);
        setActiveProfileId(saved.id);
      }
      onProfileSaved(saved);
      setFeedback({
        kind: "success",
        message: `Saved policy “${saved.name}”.`,
      });
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to save release profile.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [
    activeProfile,
    blocklistText,
    canEdit,
    cutoffFormatScore,
    defaultScore,
    description,
    formatsText,
    isCreating,
    minFormatScore,
    minUpgradeFormatScore,
    name,
    onProfileSaved,
    parseArrayField,
    parseNumberField,
    preferencesText,
    restClient,
  ]);

  const handleDelete = useCallback(async () => {
    if (!activeProfile) {
      return;
    }
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to delete release profiles.",
      });
      return;
    }

    setSubmitting(true);
    setFeedback(null);
    try {
      await restClient.deleteReleaseProfile(activeProfile.id);
      onProfileDeleted(activeProfile.id);
      setFeedback({
        kind: "success",
        message: `Deleted policy “${activeProfile.name}”.`,
      });
      setActiveProfileId(null);
      setIsCreating(false);
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to delete release profile.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [activeProfile, canEdit, onProfileDeleted, restClient]);

  return (
    <section className={styles.section} aria-labelledby={`${formId}-heading`}>
      <div className={styles.sectionHeader}>
        <div>
          <h3 id={`${formId}-heading`} className={styles.sectionTitle}>
            Release profiles
          </h3>
          <p className={styles.sectionDescription}>
            Tune automation scoring with reusable custom formats, preference weights, and block rules.
          </p>
        </div>
        <div className={styles.sectionControls}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-profile-select`}>
            Active profile
          </label>
          <div className={styles.inlineControls}>
            <select
              id={`${formId}-profile-select`}
              className={styles.select}
              value={isCreating ? "" : activeProfileId ?? ""}
              onChange={handleSelectProfile}
              disabled={!profiles.length}
            >
              {profiles.length ? null : (
                <option value="">No profiles available</option>
              )}
              {profiles.map((profile) => (
                <option key={profile.id} value={profile.id}>
                  {profile.name}
                </option>
              ))}
            </select>
            <button
              type="button"
              className={styles.secondaryButton}
              onClick={handleCreateNew}
              disabled={submitting}
            >
              New profile
            </button>
          </div>
        </div>
      </div>
      <form
        className={styles.formGrid}
        onSubmit={(event) => {
          event.preventDefault();
          void handleSave();
        }}
      >
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-name`}>
            Name
          </label>
          <input
            id={`${formId}-name`}
            className={styles.textInput}
            type="text"
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
            disabled={!canEdit}
            placeholder="Remux-first"
            required
          />
        </div>
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-description`}>
            Description
          </label>
          <textarea
            id={`${formId}-description`}
            className={styles.textArea}
            rows={3}
            value={description}
            onChange={(event) => setDescription(event.currentTarget.value)}
            disabled={!canEdit}
            placeholder="Document how the policy should behave"
          />
        </div>
        <div className={styles.fieldRow}>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-default-score`}>
              Default score
            </label>
            <input
              id={`${formId}-default-score`}
              className={styles.textInput}
              type="number"
              step="any"
              value={defaultScore}
              onChange={(event) => setDefaultScore(event.currentTarget.value)}
              disabled={!canEdit}
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-min-format`}>
              Minimum format score
            </label>
            <input
              id={`${formId}-min-format`}
              className={styles.textInput}
              type="number"
              step="any"
              value={minFormatScore}
              onChange={(event) => setMinFormatScore(event.currentTarget.value)}
              disabled={!canEdit}
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-cutoff-format`}>
              Cutoff format score
            </label>
            <input
              id={`${formId}-cutoff-format`}
              className={styles.textInput}
              type="number"
              step="any"
              value={cutoffFormatScore}
              onChange={(event) =>
                setCutoffFormatScore(event.currentTarget.value)
              }
              disabled={!canEdit}
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-upgrade-score`}>
              Upgrade threshold score
            </label>
            <input
              id={`${formId}-upgrade-score`}
              className={styles.textInput}
              type="number"
              step="any"
              value={minUpgradeFormatScore}
              onChange={(event) =>
                setMinUpgradeFormatScore(event.currentTarget.value)
              }
              disabled={!canEdit}
            />
          </div>
        </div>
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-formats`}>
            Custom formats JSON
          </label>
          <textarea
            id={`${formId}-formats`}
            className={`${styles.textArea} ${styles.monospace}`}
            rows={8}
            value={formatsText}
            onChange={(event) => setFormatsText(event.currentTarget.value)}
            disabled={!canEdit}
          />
        </div>
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-preferences`}>
            Preferences JSON
          </label>
          <textarea
            id={`${formId}-preferences`}
            className={`${styles.textArea} ${styles.monospace}`}
            rows={6}
            value={preferencesText}
            onChange={(event) => setPreferencesText(event.currentTarget.value)}
            disabled={!canEdit}
          />
        </div>
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-blocklist`}>
            Blocklist JSON
          </label>
          <textarea
            id={`${formId}-blocklist`}
            className={`${styles.textArea} ${styles.monospace}`}
            rows={4}
            value={blocklistText}
            onChange={(event) => setBlocklistText(event.currentTarget.value)}
            disabled={!canEdit}
          />
        </div>
      </form>
      <div className={styles.actionsRow}>
        <button
          type="button"
          className={styles.secondaryButton}
          onClick={handleReset}
          disabled={submitting}
        >
          Reset
        </button>
        <div className={styles.actionsSpacer} />
        <button
          type="button"
          className={styles.dangerButton}
          onClick={handleDelete}
          disabled={submitting || !activeProfile}
        >
          Delete
        </button>
        <button
          type="button"
          className={styles.primaryButton}
          onClick={handleSave}
          disabled={submitting}
        >
          {submitting ? "Saving…" : "Save profile"}
        </button>
      </div>
      {feedback ? (
        <div
          className={
            feedback.kind === "success"
              ? `${styles.feedback} ${styles.feedbackSuccess}`
              : `${styles.feedback} ${styles.feedbackError}`
          }
          role={feedback.kind === "error" ? "alert" : undefined}
        >
          {feedback.message}
        </div>
      ) : null}
    </section>
  );
}

interface RetentionRulesSectionProps {
  workflows: RetentionWorkflow[];
  restClient: RestApiClient;
  onWorkflowSaved: (workflow: RetentionWorkflow) => void;
  onWorkflowDeleted: (workflowName: string) => void;
  canEdit: boolean;
}

type RetentionOverrideDraft = {
  key: string;
  target: string;
  description: string;
  maxVersions: string;
  maxAge: string;
};

function RetentionRulesSection({
  workflows,
  restClient,
  onWorkflowSaved,
  onWorkflowDeleted,
  canEdit,
}: RetentionRulesSectionProps) {
  const [isCreating, setIsCreating] = useState(false);
  const [activeWorkflowName, setActiveWorkflowName] = useState<string | null>(
    null,
  );
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [defaultMaxVersions, setDefaultMaxVersions] = useState("");
  const [defaultMaxAge, setDefaultMaxAge] = useState("");
  const [overrides, setOverrides] = useState<RetentionOverrideDraft[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [feedback, setFeedback] = useState<
    | { kind: "success" | "error"; message: string }
    | null
  >(null);

  const activeWorkflow = useMemo(() => {
    if (isCreating) {
      return null;
    }
    if (!activeWorkflowName) {
      return null;
    }
    return (
      workflows.find((workflow) => workflow.name === activeWorkflowName) ?? null
    );
  }, [activeWorkflowName, isCreating, workflows]);

  useEffect(() => {
    if (isCreating) {
      return;
    }
    if (
      activeWorkflowName &&
      workflows.some((workflow) => workflow.name === activeWorkflowName)
    ) {
      return;
    }
    const nextName = workflows[0]?.name ?? null;
    setActiveWorkflowName(nextName);
  }, [activeWorkflowName, isCreating, workflows]);

  const createOverrideDraft = useCallback(
    (override: RetentionTargetOverride, index: number): RetentionOverrideDraft => {
      return {
        key: `${override.target || "override"}-${index}`,
        target: override.target,
        description: override.description ?? "",
        maxVersions:
          override.policy.maxVersions !== undefined &&
          override.policy.maxVersions !== null
            ? String(override.policy.maxVersions)
            : "",
        maxAge: override.policy.maxAge ?? "",
      };
    },
    [],
  );

  const resetWorkflow = useCallback(
    (workflow: RetentionWorkflow | null, creating: boolean) => {
      if (creating || !workflow) {
        setName("");
        setDescription("");
        setDefaultMaxVersions("");
        setDefaultMaxAge("");
        setOverrides([]);
      } else {
        setName(workflow.name);
        setDescription(workflow.description ?? "");
        setDefaultMaxVersions(
          workflow.defaultPolicy.maxVersions !== undefined &&
            workflow.defaultPolicy.maxVersions !== null
            ? String(workflow.defaultPolicy.maxVersions)
            : "",
        );
        setDefaultMaxAge(workflow.defaultPolicy.maxAge ?? "");
        setOverrides(
          workflow.overrides.map((override, index) =>
            createOverrideDraft(override, index),
          ),
        );
      }
      setFeedback(null);
    },
    [createOverrideDraft],
  );

  useEffect(() => {
    resetWorkflow(activeWorkflow, isCreating);
  }, [activeWorkflow, isCreating, resetWorkflow]);

  const handleSelectWorkflow = useCallback(
    (event: FormEvent<HTMLSelectElement>) => {
      const selected = event.currentTarget.value;
      setIsCreating(false);
      setActiveWorkflowName(selected || null);
    },
    [],
  );

  const handleCreateWorkflow = useCallback(() => {
    setIsCreating(true);
    setActiveWorkflowName(null);
    resetWorkflow(null, true);
  }, [resetWorkflow]);

  const handleReset = useCallback(() => {
    resetWorkflow(activeWorkflow, isCreating);
  }, [activeWorkflow, isCreating, resetWorkflow]);

  const formId = useId();

  const updateOverrideField = useCallback(
    (key: string, field: keyof RetentionOverrideDraft, value: string) => {
      setOverrides((current) =>
        current.map((override) =>
          override.key === key ? { ...override, [field]: value } : override,
        ),
      );
    },
    [],
  );

  const handleAddOverride = useCallback(() => {
    setOverrides((current) => [
      ...current,
      {
        key: `override-${Date.now()}-${current.length}`,
        target: "",
        description: "",
        maxVersions: "",
        maxAge: "",
      },
    ]);
  }, []);

  const handleRemoveOverride = useCallback((key: string) => {
    setOverrides((current) => current.filter((override) => override.key !== key));
  }, []);

  const parsePolicyNumber = useCallback(
    (value: string, label: string): number | null | undefined => {
      const trimmed = value.trim();
      if (!trimmed) {
        return null;
      }
      const parsed = Number(trimmed);
      if (Number.isNaN(parsed)) {
        throw new Error(`${label} must be a valid number`);
      }
      return parsed;
    },
    [],
  );

  const handleSave = useCallback(async () => {
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to update retention policies.",
      });
      return;
    }

    const trimmedName = name.trim();
    if (!trimmedName) {
      setFeedback({ kind: "error", message: "Workflow name is required." });
      return;
    }

    let defaultMaxVersionsValue: number | null | undefined;
    try {
      defaultMaxVersionsValue = parsePolicyNumber(
        defaultMaxVersions,
        "Default max versions",
      );
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Invalid max versions.";
      setFeedback({ kind: "error", message });
      return;
    }

    const overridesPayload: RetentionTargetOverride[] = [];
    try {
      overrides.forEach((override, index) => {
        const target = override.target.trim();
        if (!target) {
          return;
        }
        const maxVersionsValue = parsePolicyNumber(
          override.maxVersions,
          `Max versions for override ${index + 1}`,
        );
        const maxAgeValue = override.maxAge.trim() || undefined;
        const policy: RetentionPolicy = {
          maxVersions: maxVersionsValue ?? undefined,
          maxAge: maxAgeValue ?? undefined,
        };
        overridesPayload.push({
          target,
          description: override.description.trim() || undefined,
          policy,
        });
      });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Invalid override settings.";
      setFeedback({ kind: "error", message });
      return;
    }

    const defaultPolicy: RetentionPolicy = {
      maxVersions: defaultMaxVersionsValue ?? undefined,
      maxAge: defaultMaxAge.trim() || undefined,
    };

    const payload: UpsertRetentionWorkflowInput = {
      name: trimmedName,
      description: description.trim() || undefined,
      defaultPolicy,
      overrides: overridesPayload,
    };

    setSubmitting(true);
    setFeedback(null);
    try {
      let saved: RetentionWorkflow;
      if (isCreating || !activeWorkflow) {
        saved = await restClient.createRetentionWorkflow(payload);
        setActiveWorkflowName(saved.name);
        setIsCreating(false);
      } else {
        saved = await restClient.updateRetentionWorkflow(
          activeWorkflow.name,
          payload,
        );
        setActiveWorkflowName(saved.name);
      }
      onWorkflowSaved(saved);
      setFeedback({
        kind: "success",
        message: `Saved retention workflow “${saved.name}”.`,
      });
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to save retention workflow.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [
    activeWorkflow,
    canEdit,
    defaultMaxAge,
    defaultMaxVersions,
    description,
    isCreating,
    name,
    onWorkflowSaved,
    overrides,
    parsePolicyNumber,
    restClient,
  ]);

  const handleDelete = useCallback(async () => {
    if (!activeWorkflow) {
      return;
    }
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to delete retention workflows.",
      });
      return;
    }
    setSubmitting(true);
    setFeedback(null);
    try {
      await restClient.deleteRetentionWorkflow(activeWorkflow.name);
      onWorkflowDeleted(activeWorkflow.name);
      setFeedback({
        kind: "success",
        message: `Deleted retention workflow “${activeWorkflow.name}”.`,
      });
      setActiveWorkflowName(null);
      setIsCreating(false);
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to delete retention workflow.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [activeWorkflow, canEdit, onWorkflowDeleted, restClient]);

  return (
    <section className={styles.section} aria-labelledby={`${formId}-retention-heading`}>
      <div className={styles.sectionHeader}>
        <div>
          <h3 id={`${formId}-retention-heading`} className={styles.sectionTitle}>
            Retention rules
          </h3>
          <p className={styles.sectionDescription}>
            Define how long replicated artifacts are retained globally and per target override.
          </p>
        </div>
        <div className={styles.sectionControls}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-workflow-select`}>
            Active workflow
          </label>
          <div className={styles.inlineControls}>
            <select
              id={`${formId}-workflow-select`}
              className={styles.select}
              value={isCreating ? "" : activeWorkflowName ?? ""}
              onChange={handleSelectWorkflow}
              disabled={!workflows.length}
            >
              {workflows.length ? null : (
                <option value="">No workflows available</option>
              )}
              {workflows.map((workflow) => (
                <option key={workflow.name} value={workflow.name}>
                  {workflow.name}
                </option>
              ))}
            </select>
            <button
              type="button"
              className={styles.secondaryButton}
              onClick={handleCreateWorkflow}
              disabled={submitting}
            >
              New workflow
            </button>
          </div>
        </div>
      </div>
      <form
        className={styles.formGrid}
        onSubmit={(event) => {
          event.preventDefault();
          void handleSave();
        }}
      >
        <div className={styles.fieldRow}>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-workflow-name`}>
              Name
            </label>
            <input
              id={`${formId}-workflow-name`}
              className={styles.textInput}
              type="text"
              value={name}
              onChange={(event) => setName(event.currentTarget.value)}
              disabled={!canEdit || (!isCreating && Boolean(activeWorkflow))}
              placeholder="global-media"
              required
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-default-max-versions`}>
              Default max versions
            </label>
            <input
              id={`${formId}-default-max-versions`}
              className={styles.textInput}
              type="number"
              value={defaultMaxVersions}
              onChange={(event) =>
                setDefaultMaxVersions(event.currentTarget.value)
              }
              disabled={!canEdit}
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel} htmlFor={`${formId}-default-max-age`}>
              Default max age (duration)
            </label>
            <input
              id={`${formId}-default-max-age`}
              className={styles.textInput}
              type="text"
              value={defaultMaxAge}
              onChange={(event) => setDefaultMaxAge(event.currentTarget.value)}
              disabled={!canEdit}
              placeholder="720h"
            />
          </div>
        </div>
        <div className={styles.fieldGroup}>
          <label className={styles.fieldLabel} htmlFor={`${formId}-workflow-description`}>
            Description
          </label>
          <textarea
            id={`${formId}-workflow-description`}
            className={styles.textArea}
            rows={3}
            value={description}
            onChange={(event) => setDescription(event.currentTarget.value)}
            disabled={!canEdit}
          />
        </div>
        <div className={styles.overrideHeader}>
          <div>
            <h4 className={styles.subsectionTitle}>Target overrides</h4>
            <p className={styles.subsectionDescription}>
              Override retention for specific replication targets when colder storage requires deeper history.
            </p>
          </div>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={handleAddOverride}
            disabled={!canEdit}
          >
            Add override
          </button>
        </div>
        <div className={styles.overrideList}>
          {overrides.length === 0 ? (
            <p className={styles.emptyState}>No overrides configured.</p>
          ) : (
            overrides.map((override, index) => (
              <div key={override.key} className={styles.overrideCard}>
                <div className={styles.overrideGrid}>
                  <div className={styles.fieldGroup}>
                    <label
                      className={styles.fieldLabel}
                      htmlFor={`${formId}-override-target-${override.key}`}
                    >
                      Target name
                    </label>
                    <input
                      id={`${formId}-override-target-${override.key}`}
                      className={styles.textInput}
                      type="text"
                      value={override.target}
                      onChange={(event) =>
                        updateOverrideField(
                          override.key,
                          "target",
                          event.currentTarget.value,
                        )
                      }
                      disabled={!canEdit}
                      placeholder="secondary-eu"
                    />
                  </div>
                  <div className={styles.fieldGroup}>
                    <label
                      className={styles.fieldLabel}
                      htmlFor={`${formId}-override-max-versions-${override.key}`}
                    >
                      Max versions
                    </label>
                    <input
                      id={`${formId}-override-max-versions-${override.key}`}
                      className={styles.textInput}
                      type="number"
                      value={override.maxVersions}
                      onChange={(event) =>
                        updateOverrideField(
                          override.key,
                          "maxVersions",
                          event.currentTarget.value,
                        )
                      }
                      disabled={!canEdit}
                    />
                  </div>
                  <div className={styles.fieldGroup}>
                    <label
                      className={styles.fieldLabel}
                      htmlFor={`${formId}-override-max-age-${override.key}`}
                    >
                      Max age (duration)
                    </label>
                    <input
                      id={`${formId}-override-max-age-${override.key}`}
                      className={styles.textInput}
                      type="text"
                      value={override.maxAge}
                      onChange={(event) =>
                        updateOverrideField(
                          override.key,
                          "maxAge",
                          event.currentTarget.value,
                        )
                      }
                      disabled={!canEdit}
                      placeholder="2160h"
                    />
                  </div>
                  <div className={styles.fieldGroup}>
                    <label
                      className={styles.fieldLabel}
                      htmlFor={`${formId}-override-description-${override.key}`}
                    >
                      Description
                    </label>
                    <textarea
                      id={`${formId}-override-description-${override.key}`}
                      className={styles.textArea}
                      rows={2}
                      value={override.description}
                      onChange={(event) =>
                        updateOverrideField(
                          override.key,
                          "description",
                          event.currentTarget.value,
                        )
                      }
                      disabled={!canEdit}
                    />
                  </div>
                </div>
                <div className={styles.overrideActions}>
                  <span className={styles.overrideIndex}>Override {index + 1}</span>
                  <button
                    type="button"
                    className={styles.dangerButton}
                    onClick={() => handleRemoveOverride(override.key)}
                    disabled={!canEdit}
                  >
                    Remove
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </form>
      <div className={styles.actionsRow}>
        <button
          type="button"
          className={styles.secondaryButton}
          onClick={handleReset}
          disabled={submitting}
        >
          Reset
        </button>
        <div className={styles.actionsSpacer} />
        <button
          type="button"
          className={styles.dangerButton}
          onClick={handleDelete}
          disabled={submitting || !activeWorkflow}
        >
          Delete
        </button>
        <button
          type="button"
          className={styles.primaryButton}
          onClick={handleSave}
          disabled={submitting}
        >
          {submitting ? "Saving…" : "Save workflow"}
        </button>
      </div>
      {feedback ? (
        <div
          className={
            feedback.kind === "success"
              ? `${styles.feedback} ${styles.feedbackSuccess}`
              : `${styles.feedback} ${styles.feedbackError}`
          }
          role={feedback.kind === "error" ? "alert" : undefined}
        >
          {feedback.message}
        </div>
      ) : null}
    </section>
  );
}

interface MetadataOverridesSectionProps {
  restClient: RestApiClient;
  libraryEntities?: LibraryEntity[];
  canEdit: boolean;
}

type EpisodeCutoverDraft = {
  key: string;
  episodeId: string;
  seasonNumber: string;
  episodeNumber: string;
  absoluteNumber: string;
  title: string;
  airDate: string;
  notes: string;
};

type AlbumEditionDraft = {
  key: string;
  editionId: string;
  name: string;
  releaseDate: string;
  isDefault: boolean;
  notes: string;
};

type TrackOverrideDraft = {
  key: string;
  trackId: string;
  discNumber: string;
  trackNumber: string;
  totalTracks: string;
  isrc: string;
  notes: string;
};

function MetadataOverridesSection({
  restClient,
  libraryEntities,
  canEdit,
}: MetadataOverridesSectionProps) {
  const baseUrl = restClient.getBaseUrl();
  const [selectedEntityId, setSelectedEntityId] = useState<string>("");
  const [episodeDrafts, setEpisodeDrafts] = useState<EpisodeCutoverDraft[]>([]);
  const [albumDrafts, setAlbumDrafts] = useState<AlbumEditionDraft[]>([]);
  const [trackDrafts, setTrackDrafts] = useState<TrackOverrideDraft[]>([]);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<string | undefined>(
    undefined,
  );
  const [submitting, setSubmitting] = useState(false);
  const [feedback, setFeedback] = useState<
    | { kind: "success" | "error"; message: string }
    | null
  >(null);

  useEffect(() => {
    if (selectedEntityId) {
      return;
    }
    const firstEntity = libraryEntities?.[0];
    if (firstEntity) {
      setSelectedEntityId(firstEntity.id);
    }
  }, [libraryEntities, selectedEntityId]);

  const queryKey = useMemo(
    () => ["library", "metadata-overrides", baseUrl, selectedEntityId],
    [baseUrl, selectedEntityId],
  );

  const overridesQuery = useQuery({
    queryKey,
    queryFn: () => restClient.fetchMetadataOverrides(selectedEntityId),
    enabled: Boolean(selectedEntityId),
  });

  const queryClient = useQueryClient();

  const loadError =
    overridesQuery.error instanceof Error
      ? overridesQuery.error.message
      : overridesQuery.error
        ? String(overridesQuery.error)
        : null;

  const toDateInput = useCallback((value?: string): string => {
    if (!value) {
      return "";
    }
    const trimmed = value.trim();
    if (!trimmed) {
      return "";
    }
    return trimmed.slice(0, 10);
  }, []);

  const createEpisodeDraft = useCallback(
    (override: EpisodeCutoverOverride, index: number): EpisodeCutoverDraft => {
      return {
        key: `${override.episodeId || "episode"}-${index}`,
        episodeId: override.episodeId,
        seasonNumber: String(override.seasonNumber ?? 0),
        episodeNumber: String(override.episodeNumber ?? 0),
        absoluteNumber:
          override.absoluteNumber !== undefined && override.absoluteNumber !== null
            ? String(override.absoluteNumber)
            : "",
        title: override.title ?? "",
        airDate: toDateInput(override.airDate),
        notes: override.notes ?? "",
      };
    },
    [toDateInput],
  );

  const createAlbumDraft = useCallback(
    (override: AlbumEditionOverride, index: number): AlbumEditionDraft => {
      return {
        key: `${override.editionId || "edition"}-${index}`,
        editionId: override.editionId,
        name: override.name,
        releaseDate: toDateInput(override.releaseDate),
        isDefault: Boolean(override.isDefault),
        notes: override.notes ?? "",
      };
    },
    [toDateInput],
  );

  const createTrackDraft = useCallback(
    (override: TrackNumberOverride, index: number): TrackOverrideDraft => {
      return {
        key: `${override.trackId || "track"}-${index}`,
        trackId: override.trackId,
        discNumber:
          override.discNumber !== undefined && override.discNumber !== null
            ? String(override.discNumber)
            : "",
        trackNumber: String(override.trackNumber ?? ""),
        totalTracks:
          override.totalTracks !== undefined && override.totalTracks !== null
            ? String(override.totalTracks)
            : "",
        isrc: override.isrc ?? "",
        notes: override.notes ?? "",
      };
    },
    [],
  );

  const applyOverrides = useCallback(
    (overrides: MetadataOverrides | null | undefined) => {
      if (!overrides) {
        setEpisodeDrafts([]);
        setAlbumDrafts([]);
        setTrackDrafts([]);
        setLastUpdatedAt(undefined);
        return;
      }
      setEpisodeDrafts(
        overrides.episodeCutovers.map((item, index) =>
          createEpisodeDraft(item, index),
        ),
      );
      setAlbumDrafts(
        overrides.albumEditions.map((item, index) =>
          createAlbumDraft(item, index),
        ),
      );
      setTrackDrafts(
        overrides.trackNumberOverrides.map((item, index) =>
          createTrackDraft(item, index),
        ),
      );
      setLastUpdatedAt(overrides.updatedAt);
    },
    [createAlbumDraft, createEpisodeDraft, createTrackDraft],
  );

  useEffect(() => {
    applyOverrides(overridesQuery.data ?? null);
    setFeedback(null);
  }, [applyOverrides, overridesQuery.data]);

  useEffect(() => {
    setFeedback(null);
  }, [selectedEntityId]);

  const handleEntityChange = useCallback(
    (event: FormEvent<HTMLSelectElement>) => {
      setSelectedEntityId(event.currentTarget.value);
    },
    [],
  );

  const updateEpisodeField = useCallback(
    (key: string, field: keyof EpisodeCutoverDraft, value: string) => {
      setEpisodeDrafts((current) =>
        current.map((draft) =>
          draft.key === key ? { ...draft, [field]: value } : draft,
        ),
      );
    },
    [],
  );

  const updateAlbumField = useCallback(
    (key: string, field: keyof AlbumEditionDraft, value: string | boolean) => {
      setAlbumDrafts((current) =>
        current.map((draft) =>
          draft.key === key ? { ...draft, [field]: value } : draft,
        ),
      );
    },
    [],
  );

  const updateTrackField = useCallback(
    (key: string, field: keyof TrackOverrideDraft, value: string) => {
      setTrackDrafts((current) =>
        current.map((draft) =>
          draft.key === key ? { ...draft, [field]: value } : draft,
        ),
      );
    },
    [],
  );

  const handleAddEpisode = useCallback(() => {
    setEpisodeDrafts((current) => [
      ...current,
      {
        key: `episode-${Date.now()}-${current.length}`,
        episodeId: "",
        seasonNumber: "0",
        episodeNumber: "0",
        absoluteNumber: "",
        title: "",
        airDate: "",
        notes: "",
      },
    ]);
  }, []);

  const handleRemoveEpisode = useCallback((key: string) => {
    setEpisodeDrafts((current) => current.filter((draft) => draft.key !== key));
  }, []);

  const handleAddAlbum = useCallback(() => {
    setAlbumDrafts((current) => [
      ...current,
      {
        key: `album-${Date.now()}-${current.length}`,
        editionId: "",
        name: "",
        releaseDate: "",
        isDefault: false,
        notes: "",
      },
    ]);
  }, []);

  const handleRemoveAlbum = useCallback((key: string) => {
    setAlbumDrafts((current) => current.filter((draft) => draft.key !== key));
  }, []);

  const handleAddTrack = useCallback(() => {
    setTrackDrafts((current) => [
      ...current,
      {
        key: `track-${Date.now()}-${current.length}`,
        trackId: "",
        discNumber: "",
        trackNumber: "",
        totalTracks: "",
        isrc: "",
        notes: "",
      },
    ]);
  }, []);

  const handleRemoveTrack = useCallback((key: string) => {
    setTrackDrafts((current) => current.filter((draft) => draft.key !== key));
  }, []);

  const toIsoDate = useCallback((value: string): string | undefined => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    const iso = new Date(`${trimmed}T00:00:00Z`).toISOString();
    return iso;
  }, []);

  const parseIntField = useCallback(
    (value: string, label: string, { allowZero = false } = {}): number => {
      const trimmed = value.trim();
      if (!trimmed) {
        throw new Error(`${label} is required.`);
      }
      const parsed = Number.parseInt(trimmed, 10);
      if (Number.isNaN(parsed)) {
        throw new Error(`${label} must be a whole number.`);
      }
      if (!allowZero && parsed <= 0) {
        throw new Error(`${label} must be positive.`);
      }
      return parsed;
    },
    [],
  );

  const safeNumberField = useCallback((value: string): number | undefined => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    const parsed = Number(trimmed);
    return Number.isNaN(parsed) ? undefined : parsed;
  }, []);

  const handleSave = useCallback(async () => {
    if (!selectedEntityId) {
      setFeedback({ kind: "error", message: "Select an entity to update overrides." });
      return;
    }
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to update metadata overrides.",
      });
      return;
    }

    try {
      const episodePayload = episodeDrafts
        .filter((draft) => draft.episodeId.trim())
        .map((draft, index) => {
          const season = parseIntField(draft.seasonNumber, `Season number for episode ${index + 1}`, { allowZero: true });
          const episode = parseIntField(draft.episodeNumber, `Episode number for episode ${index + 1}`, { allowZero: true });
          const absolute = draft.absoluteNumber.trim()
            ? safeNumberField(draft.absoluteNumber)
            : undefined;
          return {
            episodeId: draft.episodeId.trim(),
            seasonNumber: season,
            episodeNumber: episode,
            absoluteNumber: absolute,
            title: draft.title.trim() || undefined,
            airDate: toIsoDate(draft.airDate),
            notes: draft.notes.trim() || undefined,
          } satisfies EpisodeCutoverOverride;
        });

      const albumPayload = albumDrafts
        .filter((draft) => draft.editionId.trim() && draft.name.trim())
        .map((draft) => ({
          editionId: draft.editionId.trim(),
          name: draft.name.trim(),
          releaseDate: toIsoDate(draft.releaseDate),
          isDefault: draft.isDefault,
          notes: draft.notes.trim() || undefined,
        } satisfies AlbumEditionOverride));

      const trackPayload = trackDrafts
        .filter((draft) => draft.trackId.trim())
        .map((draft, index) => ({
          trackId: draft.trackId.trim(),
          discNumber: safeNumberField(draft.discNumber),
          trackNumber: parseIntField(draft.trackNumber, `Track number for override ${index + 1}`),
          totalTracks: safeNumberField(draft.totalTracks),
          isrc: draft.isrc.trim() || undefined,
          notes: draft.notes.trim() || undefined,
        } satisfies TrackNumberOverride));

      const payload: MetadataOverridesInput = {
        episodeCutovers: episodePayload,
        albumEditions: albumPayload,
        trackNumberOverrides: trackPayload,
      };

      setSubmitting(true);
      setFeedback(null);
      const saved = await restClient.setMetadataOverrides(
        selectedEntityId,
        payload,
      );
      applyOverrides(saved);
      queryClient.setQueryData<MetadataOverrides | null>(queryKey, saved);
      setFeedback({
        kind: "success",
        message: "Metadata overrides saved.",
      });
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to save metadata overrides.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [
    albumDrafts,
    applyOverrides,
    canEdit,
    episodeDrafts,
    parseIntField,
    queryClient,
    queryKey,
    restClient,
    safeNumberField,
    selectedEntityId,
    toIsoDate,
    trackDrafts,
  ]);

  const handleClear = useCallback(async () => {
    if (!selectedEntityId) {
      setFeedback({ kind: "error", message: "Select an entity to clear overrides." });
      return;
    }
    if (!canEdit) {
      setFeedback({
        kind: "error",
        message: "You do not have permission to clear metadata overrides.",
      });
      return;
    }
    setSubmitting(true);
    setFeedback(null);
    try {
      await restClient.clearMetadataOverrides(selectedEntityId);
      applyOverrides(null);
      queryClient.setQueryData<MetadataOverrides | null>(queryKey, null);
      setFeedback({ kind: "success", message: "Metadata overrides cleared." });
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to clear metadata overrides.";
      setFeedback({ kind: "error", message });
    } finally {
      setSubmitting(false);
    }
  }, [
    applyOverrides,
    canEdit,
    queryClient,
    queryKey,
    restClient,
    selectedEntityId,
  ]);

  const handleReset = useCallback(() => {
    applyOverrides(overridesQuery.data ?? null);
    setFeedback(null);
  }, [applyOverrides, overridesQuery.data]);

  const hasSelection = Boolean(selectedEntityId);

  const { refetch: refetchOverrides } = overridesQuery;

  const handleReload = useCallback(() => {
    if (!hasSelection) {
      return;
    }
    setFeedback(null);
    void refetchOverrides();
  }, [hasSelection, refetchOverrides]);

  const isLoadingOverrides = overridesQuery.isPending || overridesQuery.isFetching;

  const isEditingDisabled = !canEdit || !hasSelection || isLoadingOverrides;

  const isSaveDisabled = submitting || isEditingDisabled || Boolean(loadError);

  const isResetDisabled = submitting || isLoadingOverrides || !hasSelection;

  return (
    <section className={styles.section} aria-labelledby="metadata-overrides-heading">
      <div className={styles.sectionHeader}>
        <div>
          <h3 id="metadata-overrides-heading" className={styles.sectionTitle}>
            Metadata overrides
          </h3>
          <p className={styles.sectionDescription}>
            Adjust provider metadata for edge cases without waiting on upstream corrections.
          </p>
        </div>
        <div className={styles.sectionControls}>
          <label className={styles.fieldLabel} htmlFor="metadata-entity-select">
            Library entity
          </label>
          <div className={styles.inlineControls}>
            <select
              id="metadata-entity-select"
              className={styles.select}
              value={selectedEntityId}
              onChange={handleEntityChange}
              disabled={!libraryEntities?.length}
            >
              {libraryEntities?.length ? null : (
                <option value="">No entities available</option>
              )}
              {libraryEntities?.map((entity) => (
                <option key={entity.id} value={entity.id}>
                  {entity.title} · {entity.entityType}
                </option>
              ))}
            </select>
            <button
              type="button"
              className={styles.secondaryButton}
              onClick={handleClear}
              disabled={submitting || isEditingDisabled}
            >
              Clear overrides
            </button>
          </div>
        </div>
      </div>
      {lastUpdatedAt ? (
        <p className={styles.updatedAt}>Last updated {lastUpdatedAt}</p>
      ) : null}
      {!libraryEntities?.length ? (
        <p className={styles.emptyState}>No entities available.</p>
      ) : loadError ? (
        <div className={styles.errorState} role="alert">
          <p className={styles.errorMessage}>Failed to load metadata overrides.</p>
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={handleReload}
            disabled={isLoadingOverrides || !hasSelection}
          >
            Retry
          </button>
        </div>
      ) : overridesQuery.isPending ? (
        <p className={styles.emptyState}>Loading overrides…</p>
      ) : (
        <>
          <div className={styles.subsection}>
            <div className={styles.subsectionHeader}>
              <h4 className={styles.subsectionTitle}>Episode cutovers</h4>
              <button
                type="button"
                className={styles.secondaryButton}
                onClick={handleAddEpisode}
                disabled={isEditingDisabled}
              >
                Add episode cutover
              </button>
            </div>
            {episodeDrafts.length === 0 ? (
              <p className={styles.emptyState}>No episode overrides configured.</p>
            ) : (
              <div className={styles.overrideList}>
                {episodeDrafts.map((draft, index) => (
                  <div key={draft.key} className={styles.overrideCard}>
                    <div className={styles.overrideGrid}>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`episode-id-${draft.key}`}
                        >
                          Episode ID
                        </label>
                        <input
                          id={`episode-id-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.episodeId}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "episodeId",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                          placeholder="s01e01"
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`season-number-${draft.key}`}
                        >
                          Season number
                        </label>
                        <input
                          id={`season-number-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.seasonNumber}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "seasonNumber",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`episode-number-${draft.key}`}
                        >
                          Episode number
                        </label>
                        <input
                          id={`episode-number-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.episodeNumber}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "episodeNumber",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`absolute-number-${draft.key}`}
                        >
                          Absolute number
                        </label>
                        <input
                          id={`absolute-number-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.absoluteNumber}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "absoluteNumber",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`episode-title-${draft.key}`}
                        >
                          Title override
                        </label>
                        <input
                          id={`episode-title-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.title}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "title",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`episode-air-date-${draft.key}`}
                        >
                          Air date
                        </label>
                        <input
                          id={`episode-air-date-${draft.key}`}
                          className={styles.textInput}
                          type="date"
                          value={draft.airDate}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "airDate",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`episode-notes-${draft.key}`}
                        >
                          Notes
                        </label>
                        <textarea
                          id={`episode-notes-${draft.key}`}
                          className={styles.textArea}
                          rows={2}
                          value={draft.notes}
                          onChange={(event) =>
                            updateEpisodeField(
                              draft.key,
                              "notes",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                    </div>
                    <div className={styles.overrideActions}>
                      <span className={styles.overrideIndex}>Episode override {index + 1}</span>
                      <button
                        type="button"
                        className={styles.dangerButton}
                        onClick={() => handleRemoveEpisode(draft.key)}
                        disabled={isEditingDisabled}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          <div className={styles.subsection}>
            <div className={styles.subsectionHeader}>
              <h4 className={styles.subsectionTitle}>Album editions</h4>
              <button
                type="button"
                className={styles.secondaryButton}
                onClick={handleAddAlbum}
                disabled={isEditingDisabled}
              >
                Add album edition
              </button>
            </div>
            {albumDrafts.length === 0 ? (
              <p className={styles.emptyState}>No album edition overrides configured.</p>
            ) : (
              <div className={styles.overrideList}>
                {albumDrafts.map((draft, index) => (
                  <div key={draft.key} className={styles.overrideCard}>
                    <div className={styles.overrideGrid}>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`edition-id-${draft.key}`}
                        >
                          Edition ID
                        </label>
                        <input
                          id={`edition-id-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.editionId}
                          onChange={(event) =>
                            updateAlbumField(
                              draft.key,
                              "editionId",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                          placeholder="deluxe"
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`edition-name-${draft.key}`}
                        >
                          Name
                        </label>
                        <input
                          id={`edition-name-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.name}
                          onChange={(event) =>
                            updateAlbumField(
                              draft.key,
                              "name",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`edition-release-${draft.key}`}
                        >
                          Release date
                        </label>
                        <input
                          id={`edition-release-${draft.key}`}
                          className={styles.textInput}
                          type="date"
                          value={draft.releaseDate}
                          onChange={(event) =>
                            updateAlbumField(
                              draft.key,
                              "releaseDate",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.checkboxGroup}>
                        <label className={styles.checkboxLabel}>
                          <input
                            type="checkbox"
                            checked={draft.isDefault}
                            onChange={(event) =>
                              updateAlbumField(
                                draft.key,
                                "isDefault",
                                event.currentTarget.checked,
                              )
                            }
                            disabled={isEditingDisabled}
                          />
                          Mark as default edition
                        </label>
                      </div>
                      <div className={styles.fieldGroup}>
                        <label
                          className={styles.fieldLabel}
                          htmlFor={`edition-notes-${draft.key}`}
                        >
                          Notes
                        </label>
                        <textarea
                          id={`edition-notes-${draft.key}`}
                          className={styles.textArea}
                          rows={2}
                          value={draft.notes}
                          onChange={(event) =>
                            updateAlbumField(
                              draft.key,
                              "notes",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                    </div>
                    <div className={styles.overrideActions}>
                      <span className={styles.overrideIndex}>Album edition {index + 1}</span>
                      <button
                        type="button"
                        className={styles.dangerButton}
                        onClick={() => handleRemoveAlbum(draft.key)}
                        disabled={isEditingDisabled}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          <div className={styles.subsection}>
            <div className={styles.subsectionHeader}>
              <h4 className={styles.subsectionTitle}>Track numbering</h4>
              <button
                type="button"
                className={styles.secondaryButton}
                onClick={handleAddTrack}
                disabled={isEditingDisabled}
              >
                Add track override
              </button>
            </div>
            {trackDrafts.length === 0 ? (
              <p className={styles.emptyState}>No track overrides configured.</p>
            ) : (
              <div className={styles.overrideList}>
                {trackDrafts.map((draft, index) => (
                  <div key={draft.key} className={styles.overrideCard}>
                    <div className={styles.overrideGrid}>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`track-id-${draft.key}`}>
                          Track ID
                        </label>
                        <input
                          id={`track-id-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.trackId}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "trackId",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                          placeholder="track-1"
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`disc-number-${draft.key}`}>
                          Disc number
                        </label>
                        <input
                          id={`disc-number-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.discNumber}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "discNumber",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`track-number-${draft.key}`}>
                          Track number
                        </label>
                        <input
                          id={`track-number-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.trackNumber}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "trackNumber",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`total-tracks-${draft.key}`}>
                          Total tracks
                        </label>
                        <input
                          id={`total-tracks-${draft.key}`}
                          className={styles.textInput}
                          type="number"
                          value={draft.totalTracks}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "totalTracks",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`track-isrc-${draft.key}`}>
                          ISRC
                        </label>
                        <input
                          id={`track-isrc-${draft.key}`}
                          className={styles.textInput}
                          type="text"
                          value={draft.isrc}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "isrc",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                      <div className={styles.fieldGroup}>
                        <label className={styles.fieldLabel} htmlFor={`track-notes-${draft.key}`}>
                          Notes
                        </label>
                        <textarea
                          id={`track-notes-${draft.key}`}
                          className={styles.textArea}
                          rows={2}
                          value={draft.notes}
                          onChange={(event) =>
                            updateTrackField(
                              draft.key,
                              "notes",
                              event.currentTarget.value,
                            )
                          }
                          disabled={isEditingDisabled}
                        />
                      </div>
                    </div>
                    <div className={styles.overrideActions}>
                      <span className={styles.overrideIndex}>Track override {index + 1}</span>
                      <button
                        type="button"
                        className={styles.dangerButton}
                        onClick={() => handleRemoveTrack(draft.key)}
                        disabled={isEditingDisabled}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
      <div className={styles.actionsRow}>
        <button
          type="button"
          className={styles.secondaryButton}
          onClick={handleReset}
          disabled={isResetDisabled}
        >
          Reset
        </button>
        <button
          type="button"
          className={styles.secondaryButton}
          onClick={handleReload}
          disabled={isLoadingOverrides || !hasSelection}
        >
          Reload
        </button>
        <div className={styles.actionsSpacer} />
        <button
          type="button"
          className={styles.primaryButton}
          onClick={() => {
            void handleSave();
          }}
          disabled={isSaveDisabled}
        >
          {submitting ? "Saving…" : "Save overrides"}
        </button>
      </div>
      {feedback ? (
        <div
          className={
            feedback.kind === "success"
              ? `${styles.feedback} ${styles.feedbackSuccess}`
              : `${styles.feedback} ${styles.feedbackError}`
          }
          role={feedback.kind === "error" ? "alert" : undefined}
        >
          {feedback.message}
        </div>
      ) : null}
    </section>
  );
}

