"use client";

import {
  KeyboardEvent,
  MouseEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";

import type { SidebarNavItem } from "../layout/SidebarNavigation";
import { useEnvironment } from "../environment/EnvironmentContext";
import { useAuth } from "../auth/AuthContext";
import { useToast } from "../feedback/ToastProvider";
import { useClientLogger } from "../diagnostics/DiagnosticsProvider";
import { useRestQuery } from "../../hooks/useRestQuery";
import { useRestClient } from "../../hooks/useRestClient";
import type { JobSummary, LibraryEntity } from "../../lib/api";

import styles from "./CommandPalette.module.css";

type NavigationItem = {
  id: string;
  kind: "navigation";
  title: string;
  subtitle?: string;
  href: string;
  disabled?: boolean;
};

type EntityItem = {
  id: string;
  kind: "entity";
  title: string;
  subtitle?: string;
  href: string;
  entityId: string;
};

type ActionItem = {
  id: string;
  kind: "action";
  title: string;
  subtitle?: string;
  jobType: string;
  targetId: string;
  entityTitle: string;
  entityType: string;
};

type PaletteItem = NavigationItem | EntityItem | ActionItem;

type PaletteSection = {
  id: string;
  label: string;
  items: PaletteItem[];
};

type CommandPaletteProps = {
  navigationItems: SidebarNavItem[];
  className?: string;
};

type LibraryQueryKey = readonly ["library", "entities", string];

function normalizeQuery(value: string): string {
  return value.trim().toLowerCase();
}

function matchesQuery(item: PaletteItem, query: string): boolean {
  if (!query) {
    return true;
  }
  const haystack = [
    item.title,
    "subtitle" in item ? item.subtitle ?? "" : "",
    "entityId" in item ? item.entityId : "",
    "targetId" in item ? item.targetId : "",
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}

function detectShortcutLabel(): string {
  if (typeof navigator === "undefined") {
    return "Ctrl K";
  }
  const platform = navigator.userAgentData?.platform ?? navigator.platform ?? "";
  return /mac/i.test(platform) ? "⌘K" : "Ctrl K";
}

export function CommandPalette({
  navigationItems,
  className,
}: CommandPaletteProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { current } = useEnvironment();
  const { status: authStatus, hasPermission } = useAuth();
  const restClient = useRestClient();
  const toast = useToast();
  const logger = useClientLogger("command-palette");

  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const [shortcutLabel, setShortcutLabel] = useState(() => detectShortcutLabel());
  const [executingActionId, setExecutingActionId] = useState<string | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const apiBaseUrl = current.apiUrl;

  const libraryQueryKey = useMemo<LibraryQueryKey>(
    () => ["library", "entities", apiBaseUrl],
    [apiBaseUrl],
  );

  const cachedLibrary = queryClient.getQueryData<LibraryEntity[]>(libraryQueryKey);

  const {
    data: libraryEntities,
    isPending: libraryLoading,
    error: libraryError,
  } = useRestQuery(
    libraryQueryKey,
    (client) => client.fetchEntities(),
    {
      enabled: open,
      initialData: cachedLibrary,
      staleTime: 5 * 60_000,
      gcTime: 15 * 60_000,
    },
  );

  const canTriggerAutomation =
    authStatus === "authenticated" && hasPermission("library:write");

  const navigationResults = useMemo<NavigationItem[]>(
    () =>
      navigationItems.map((item) => ({
        id: `nav-${item.href}`,
        kind: "navigation" as const,
        title: item.label,
        subtitle: item.disabled ? "Coming soon" : "Navigate",
        href: item.href,
        disabled: item.disabled,
      })),
    [navigationItems],
  );

  const entityResults = useMemo<EntityItem[]>(() => {
    return (libraryEntities ?? []).map((entity) => ({
      id: `entity-${entity.id}`,
      kind: "entity" as const,
      title: entity.title || entity.id,
      subtitle: `Library • ${entity.entityType}`,
      href: `/?focusEntity=${encodeURIComponent(entity.id)}`,
      entityId: entity.id,
    }));
  }, [libraryEntities]);

  const actionResults = useMemo<ActionItem[]>(() => {
    return (libraryEntities ?? []).slice(0, 20).map((entity) => ({
      id: `action-scan-${entity.id}`,
      kind: "action" as const,
      title: `Trigger scan for ${entity.title || entity.id}`,
      subtitle: `Automation • Scan · ${entity.entityType}`,
      jobType: "scan",
      targetId: entity.id,
      entityTitle: entity.title || entity.id,
      entityType: entity.entityType,
    }));
  }, [libraryEntities]);

  const normalizedQuery = normalizeQuery(query);

  const sections = useMemo<PaletteSection[]>(() => {
    const filteredActions = actionResults.filter((item) =>
      matchesQuery(item, normalizedQuery),
    );
    const filteredNavigation = navigationResults.filter((item) =>
      matchesQuery(item, normalizedQuery),
    );
    const filteredEntities = entityResults
      .filter((item) => matchesQuery(item, normalizedQuery))
      .slice(0, normalizedQuery ? 15 : 8);

    const result: PaletteSection[] = [];
    if (filteredActions.length) {
      result.push({
        id: "actions",
        label: "Quick actions",
        items: filteredActions,
      });
    }
    if (filteredNavigation.length) {
      result.push({
        id: "navigation",
        label: "Navigation",
        items: filteredNavigation,
      });
    }
    if (filteredEntities.length) {
      result.push({
        id: "entities",
        label: "Library entities",
        items: filteredEntities,
      });
    }
    return result;
  }, [actionResults, entityResults, navigationResults, normalizedQuery]);

  const flatItems = useMemo(() => {
    return sections.flatMap((section) => section.items);
  }, [sections]);

  const closePalette = useCallback(() => {
    setOpen(false);
    setQuery("");
    setActiveIndex(0);
  }, []);

  const focusSearchInput = useCallback(() => {
    if (!searchInputRef.current) {
      return;
    }
    searchInputRef.current.focus();
    searchInputRef.current.select();
  }, []);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const isMeta = event.metaKey || event.ctrlKey;
      if ((event.key === "k" || event.key === "K") && isMeta) {
        event.preventDefault();
        setOpen((current) => !current);
        return;
      }
      if (event.key === "Escape") {
        setOpen(false);
      }
    };

    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
    };
  }, []);

  useEffect(() => {
    if (!open) {
      return;
    }
    const timer = window.setTimeout(() => {
      focusSearchInput();
    }, 0);
    return () => {
      window.clearTimeout(timer);
    };
  }, [focusSearchInput, open]);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setActiveIndex(0);
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    setActiveIndex(0);
  }, [normalizedQuery, open, sections.length]);

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }
    if (open) {
      const previous = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = previous;
      };
    }
    return undefined;
  }, [open]);

  useEffect(() => {
    setShortcutLabel(detectShortcutLabel());
  }, []);

  const handleSearchKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (!flatItems.length) {
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          event.preventDefault();
        }
        return;
      }
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActiveIndex((index) => Math.min(flatItems.length - 1, index + 1));
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setActiveIndex((index) => Math.max(0, index - 1));
        return;
      }
      if (event.key === "Enter") {
        event.preventDefault();
        const item = flatItems[activeIndex];
        if (item) {
          void handleSelect(item);
        }
      }
    },
    [activeIndex, flatItems, handleSelect],
  );

  const handleSelect = useCallback(
    async (item: PaletteItem) => {
      if (item.kind === "navigation") {
        if (item.disabled) {
          toast.push({
            variant: "warning",
            title: "Coming soon",
            description: `${item.title} isn’t available yet.`,
          });
          return;
        }
        closePalette();
        router.push(item.href);
        logger.info("Navigation command executed", {
          href: item.href,
          label: item.title,
        });
        return;
      }

      if (item.kind === "entity") {
        closePalette();
        router.push(item.href);
        logger.info("Entity navigation executed", {
          entityId: item.entityId,
          href: item.href,
        });
        return;
      }

      if (item.kind === "action") {
        if (!canTriggerAutomation) {
          toast.push({
            variant: "warning",
            title: "Permission required",
            description:
              "Automation workflows require a key with the library:write permission.",
          });
          logger.warn("Automation command blocked by permissions", {
            entityId: item.targetId,
            jobType: item.jobType,
          });
          return;
        }

        try {
          setExecutingActionId(item.id);
          const job: JobSummary = await restClient.triggerJob({
            jobType: item.jobType,
            targetID: item.targetId,
          });
          toast.push({
            variant: "success",
            title: "Workflow queued",
            description: `${item.entityTitle} · ${job.jobType} (${job.id})`,
          });
          logger.info("Automation command queued", {
            entityId: item.targetId,
            jobType: item.jobType,
            jobId: job.id,
          });
          closePalette();
        } catch (error) {
          const message =
            error instanceof Error ? error.message : "Unexpected error";
          toast.push({
            variant: "error",
            title: "Failed to trigger workflow",
            description: message,
          });
          logger.error(
            "Automation command failed",
            {
              entityId: item.targetId,
              jobType: item.jobType,
            },
            error,
          );
        } finally {
          setExecutingActionId(null);
        }
      }
    },
    [
      canTriggerAutomation,
      closePalette,
      logger,
      restClient,
      router,
      toast,
    ],
  );

  const handleBackdropClick = useCallback(
    (event: MouseEvent<HTMLDivElement>) => {
      if (event.target === event.currentTarget) {
        closePalette();
      }
    },
    [closePalette],
  );

  const computeFlatIndex = useCallback(
    (sectionId: string, offset: number) => {
      let total = 0;
      for (const section of sections) {
        if (section.id === sectionId) {
          return total + offset;
        }
        total += section.items.length;
      }
      return offset;
    },
    [sections],
  );

  const libraryErrorMessage =
    libraryError instanceof Error
      ? libraryError.message
      : libraryError
        ? String(libraryError)
        : null;

  const containerClassName = [styles.container, className]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={containerClassName}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <span className={styles.triggerPlaceholder}>Search or run a command…</span>
        <span className={styles.shortcut}>{shortcutLabel}</span>
      </button>
      {open ? (
        <div className={styles.backdrop} onMouseDown={handleBackdropClick}>
          <div
            className={styles.dialog}
            role="dialog"
            aria-modal="true"
            aria-label="Command menu"
          >
            <div className={styles.searchRow}>
              <span className={styles.searchIcon} aria-hidden>⌘</span>
              <input
                ref={searchInputRef}
                className={styles.searchInput}
                type="text"
                placeholder="Find an entity, navigate, or trigger a workflow"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={handleSearchKeyDown}
                autoComplete="off"
                autoCorrect="off"
                spellCheck={false}
              />
            </div>
            <div className={styles.results} role="listbox">
              {sections.length ? (
                sections.map((section) => (
                  <div key={section.id} className={styles.section}>
                    <p className={styles.sectionLabel}>{section.label}</p>
                    <ul className={styles.sectionList}>
                      {section.items.map((item, index) => {
                        const flatIndex = computeFlatIndex(section.id, index);
                        const isActive = flatIndex === activeIndex;
                        const isBusy =
                          item.kind === "action" && executingActionId === item.id;
                        const disabled =
                          (item.kind === "navigation" && item.disabled) || isBusy;

                        const itemClassName = [
                          styles.itemButton,
                          disabled ? styles.itemDisabled : "",
                        ]
                          .filter(Boolean)
                          .join(" ");

                        return (
                          <li key={item.id}>
                            <button
                              type="button"
                              className={itemClassName}
                              onMouseEnter={() => setActiveIndex(flatIndex)}
                              onMouseDown={(event) => event.preventDefault()}
                              onClick={() => {
                                if (!disabled) {
                                  void handleSelect(item);
                                }
                              }}
                              data-kind={item.kind}
                              data-active={isActive ? "true" : undefined}
                              disabled={disabled}
                            >
                              <div className={styles.itemBody}>
                                <span className={styles.itemTitle}>{item.title}</span>
                                {item.subtitle ? (
                                  <span className={styles.itemSubtitle}>
                                    {item.subtitle}
                                  </span>
                                ) : null}
                              </div>
                              <div className={styles.itemMeta}>
                                {item.kind === "action" ? (
                                  <span className={styles.badge}>Workflow</span>
                                ) : null}
                                {item.kind === "navigation" ? (
                                  <span className={styles.badge}>Navigate</span>
                                ) : null}
                                {isBusy ? (
                                  <span className={styles.itemStatus}>Running…</span>
                                ) : null}
                              </div>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                ))
              ) : (
                <div className={styles.emptyState}>
                  {libraryLoading ? (
                    <p>Loading library data…</p>
                  ) : normalizedQuery ? (
                    <p>No matches for “{query}”. Try a different search.</p>
                  ) : (
                    <p>
                      No commands available yet. Try adding media to your library to
                      unlock automation quick actions.
                    </p>
                  )}
                </div>
              )}
            </div>
            <div className={styles.footer}>
              <div className={styles.helper}>
                <kbd>Esc</kbd>
                <span>Close</span>
              </div>
              <div className={styles.helper}>
                <kbd>↑</kbd>
                <kbd>↓</kbd>
                <span>Navigate</span>
              </div>
              <div className={styles.helper}>
                <kbd>Enter</kbd>
                <span>Select</span>
              </div>
            </div>
            {libraryErrorMessage ? (
              <div className={styles.errorState} role="status">
                <p>Unable to load library entities: {libraryErrorMessage}</p>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
