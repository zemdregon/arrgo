"use client";

import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useEnvironment } from "../environment/EnvironmentContext";
import {
  fetchPrincipal,
  Principal,
  setApiTokenProvider,
} from "../../lib/api";

const STORAGE_KEY = "arrgo:dashboard:auth";

type AuthStatus = "idle" | "loading" | "authenticated" | "error";

type StoredKey = {
  id: string;
  label: string;
  token: string;
  tokenId?: string;
  role?: string;
  permissions?: string[];
  lastValidatedAt?: string;
};

type EnvironmentAuthState = {
  keys: StoredKey[];
  activeKeyId?: string;
};

type AuthStore = Record<string, EnvironmentAuthState>;

type AddKeyInput = {
  label?: string;
  token: string;
};

type AuthContextValue = {
  status: AuthStatus;
  principal?: Principal;
  error?: string;
  keys: StoredKey[];
  activeKey?: StoredKey;
  environmentId: string;
  isHydrated: boolean;
  addKey: (input: AddKeyInput) => { ok: boolean; error?: string };
  selectKey: (id: string) => void;
  removeKey: (id: string) => void;
  clearActiveKey: () => void;
  clearError: () => void;
  refresh: () => void;
  hasPermission: (permission: string) => boolean;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

function safeTokenPrefix(token: string): string | undefined {
  const trimmed = token.trim();
  if (!trimmed) {
    return undefined;
  }
  const separator = trimmed.indexOf(".");
  if (separator <= 0) {
    return trimmed || undefined;
  }
  return trimmed.slice(0, separator);
}

function generateKeyId(token: string): string {
  const prefix = safeTokenPrefix(token);
  if (prefix) {
    return prefix;
  }
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `key-${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 8)}`;
}

function sanitizeStore(raw: unknown): AuthStore {
  if (!raw || typeof raw !== "object") {
    return {};
  }
  const environments =
    typeof (raw as { environments?: unknown }).environments === "object"
      ? ((raw as { environments: unknown }).environments as Record<string, unknown>)
      : {};

  const result: AuthStore = {};
  for (const [envId, value] of Object.entries(environments)) {
    if (!value || typeof value !== "object") {
      continue;
    }
    const keysValue = Array.isArray((value as { keys?: unknown }).keys)
      ? ((value as { keys: unknown[] }).keys as unknown[])
      : [];
    const keys = keysValue.flatMap((item) => {
      if (!item || typeof item !== "object") {
        return [];
      }
      const token = typeof (item as { token?: unknown }).token === "string"
        ? (item as { token: string }).token.trim()
        : "";
      if (!token) {
        return [];
      }
      const idRaw = typeof (item as { id?: unknown }).id === "string"
        ? (item as { id: string }).id.trim()
        : "";
      const id = idRaw || generateKeyId(token);
      const labelRaw = typeof (item as { label?: unknown }).label === "string"
        ? (item as { label: string }).label.trim()
        : "";
      const label = labelRaw || safeTokenPrefix(token) || id;
      const tokenIdRaw = typeof (item as { tokenId?: unknown }).tokenId === "string"
        ? (item as { tokenId: string }).tokenId.trim()
        : "";
      const roleRaw = typeof (item as { role?: unknown }).role === "string"
        ? (item as { role: string }).role.trim()
        : "";
      const permissionsValue = Array.isArray(
        (item as { permissions?: unknown }).permissions,
      )
        ? ((item as { permissions: unknown[] }).permissions as unknown[])
        : [];
      const permissions = permissionsValue
        .map((value) => (typeof value === "string" ? value.trim() : ""))
        .filter((value) => value.length > 0);
      const validatedRaw = typeof (item as { lastValidatedAt?: unknown })
        .lastValidatedAt === "string"
        ? (item as { lastValidatedAt: string }).lastValidatedAt
        : undefined;
      const entry: StoredKey = {
        id,
        label,
        token,
        tokenId: tokenIdRaw || safeTokenPrefix(token),
        role: roleRaw || undefined,
        permissions: permissions.length ? permissions : undefined,
        lastValidatedAt: validatedRaw,
      };
      return [entry];
    });

    let activeKeyId = typeof (value as { activeKeyId?: unknown }).activeKeyId === "string"
      ? (value as { activeKeyId: string }).activeKeyId.trim()
      : undefined;
    if (activeKeyId && !keys.some((key) => key.id === activeKeyId)) {
      activeKeyId = undefined;
    }
    result[envId] = {
      keys,
      activeKeyId: activeKeyId || keys[0]?.id,
    };
  }
  return result;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const { current, isHydrated: environmentHydrated } = useEnvironment();
  const environmentId = current.id;
  const apiBaseUrl = current.apiUrl;

  const [store, setStore] = useState<AuthStore>({});
  const [hydrated, setHydrated] = useState(false);
  const [status, setStatus] = useState<AuthStatus>("idle");
  const [principal, setPrincipal] = useState<Principal | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [refreshToken, setRefreshToken] = useState(0);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY);
      if (raw) {
        const parsed = JSON.parse(raw) as unknown;
        const sanitized = sanitizeStore(parsed);
        setStore(sanitized);
      }
    } catch (err) {
      // Ignore malformed storage data and continue with defaults.
    } finally {
      setHydrated(true);
    }
  }, []);

  useEffect(() => {
    if (!hydrated || typeof window === "undefined") {
      return;
    }
    const payload = JSON.stringify({ environments: store });
    try {
      window.localStorage.setItem(STORAGE_KEY, payload);
    } catch (err) {
      // Ignore storage persistence errors (e.g., private browsing).
    }
  }, [store, hydrated]);

  useEffect(() => {
    setStore((previous) => {
      const envState = previous[environmentId];
      if (!envState) {
        return {
          ...previous,
          [environmentId]: { keys: [], activeKeyId: undefined },
        };
      }
      if (!envState.activeKeyId && envState.keys.length > 0) {
        return {
          ...previous,
          [environmentId]: {
            ...envState,
            activeKeyId: envState.keys[0]?.id,
          },
        };
      }
      return previous;
    });
  }, [environmentId]);

  const environmentState = store[environmentId] ?? { keys: [], activeKeyId: undefined };
  const activeKey = useMemo(() => {
    if (!environmentState.activeKeyId) {
      return undefined;
    }
    return environmentState.keys.find((key) => key.id === environmentState.activeKeyId);
  }, [environmentState.activeKeyId, environmentState.keys]);

  const activeToken = activeKey?.token ?? "";

  useEffect(() => {
    if (activeToken) {
      setApiTokenProvider(() => activeToken);
    } else {
      setApiTokenProvider(undefined);
    }
  }, [activeToken]);

  useEffect(() => {
    if (!hydrated || !environmentHydrated) {
      return;
    }
    if (!activeToken) {
      setPrincipal(undefined);
      setStatus("idle");
      setError(undefined);
      return;
    }
    let cancelled = false;
    setStatus("loading");
    setError(undefined);
    fetchPrincipal(apiBaseUrl)
      .then((result) => {
        if (cancelled) {
          return;
        }
        setPrincipal(result);
        setStatus("authenticated");
        const now = new Date().toISOString();
        setStore((previous) => {
          const envState = previous[environmentId];
          if (!envState) {
            return previous;
          }
          const updatedKeys = envState.keys.map((key) => {
            if (key.id !== envState.activeKeyId) {
              return key;
            }
            return {
              ...key,
              label: result.key.label?.trim() || key.label,
              tokenId: result.key.id || key.tokenId,
              role: result.role.name || key.role,
              permissions: result.permissions.length
                ? result.permissions
                : key.permissions,
              lastValidatedAt: now,
            } satisfies StoredKey;
          });
          return {
            ...previous,
            [environmentId]: {
              ...envState,
              keys: updatedKeys,
            },
          };
        });
      })
      .catch((err) => {
        if (cancelled) {
          return;
        }
        const message = err instanceof Error ? err.message : String(err);
        setPrincipal(undefined);
        setStatus("error");
        setError(message);
      });
    return () => {
      cancelled = true;
    };
  }, [
    activeToken,
    apiBaseUrl,
    environmentId,
    environmentState.keys.length,
    environmentHydrated,
    hydrated,
    refreshToken,
  ]);

  const addKey = useCallback(
    (input: AddKeyInput) => {
      const token = input.token?.trim();
      if (!token) {
        return { ok: false, error: "API token is required" } as const;
      }
      const label = input.label?.trim() || safeTokenPrefix(token) || "Untitled key";
      setStore((previous) => {
        const envState = previous[environmentId] ?? { keys: [], activeKeyId: undefined };
        const id = generateKeyId(token);
        const filtered = envState.keys.filter((key) => key.id !== id && key.token !== token);
        const nextKeys = [
          ...filtered,
          {
            id,
            label,
            token,
            tokenId: safeTokenPrefix(token),
          } as StoredKey,
        ];
        return {
          ...previous,
          [environmentId]: {
            keys: nextKeys,
            activeKeyId: id,
          },
        };
      });
      setPrincipal(undefined);
      setStatus("loading");
      setError(undefined);
      setRefreshToken((value) => value + 1);
      return { ok: true } as const;
    },
    [environmentId],
  );

  const selectKey = useCallback(
    (id: string) => {
      setStore((previous) => {
        const envState = previous[environmentId];
        if (!envState || envState.activeKeyId === id) {
          return previous;
        }
        if (!envState.keys.some((key) => key.id === id)) {
          return previous;
        }
        return {
          ...previous,
          [environmentId]: {
            ...envState,
            activeKeyId: id,
          },
        };
      });
      setPrincipal(undefined);
      setStatus("loading");
      setError(undefined);
      setRefreshToken((value) => value + 1);
    },
    [environmentId],
  );

  const removeKey = useCallback(
    (id: string) => {
      setStore((previous) => {
        const envState = previous[environmentId];
        if (!envState) {
          return previous;
        }
        const nextKeys = envState.keys.filter((key) => key.id !== id);
        const isActive = envState.activeKeyId === id;
        return {
          ...previous,
          [environmentId]: {
            keys: nextKeys,
            activeKeyId: isActive ? nextKeys[0]?.id : envState.activeKeyId,
          },
        };
      });
      setPrincipal(undefined);
      setStatus("idle");
      setError(undefined);
      setRefreshToken((value) => value + 1);
    },
    [environmentId],
  );

  const clearActiveKey = useCallback(() => {
    setStore((previous) => {
      const envState = previous[environmentId];
      if (!envState || !envState.activeKeyId) {
        return previous;
      }
      return {
        ...previous,
        [environmentId]: {
          ...envState,
          activeKeyId: undefined,
        },
      };
    });
    setPrincipal(undefined);
    setStatus("idle");
    setError(undefined);
    setRefreshToken((value) => value + 1);
  }, [environmentId]);

  const clearError = useCallback(() => {
    setError(undefined);
  }, []);

  const refresh = useCallback(() => {
    if (!activeToken) {
      return;
    }
    setRefreshToken((value) => value + 1);
  }, [activeToken]);

  const hasPermission = useCallback(
    (permission: string) => {
      const normalized = permission.trim().toLowerCase();
      if (!normalized) {
        return false;
      }
      const pool = principal?.permissions ?? activeKey?.permissions ?? [];
      return pool.some((perm) => perm.toLowerCase() === normalized);
    },
    [activeKey?.permissions, principal?.permissions],
  );

  const value = useMemo<AuthContextValue>(
    () => ({
      status: activeToken ? status : "idle",
      principal,
      error,
      keys: environmentState.keys,
      activeKey,
      environmentId,
      isHydrated: hydrated && environmentHydrated,
      addKey,
      selectKey,
      removeKey,
      clearActiveKey,
      clearError,
      refresh,
      hasPermission,
    }),
    [
      activeKey,
      activeToken,
      addKey,
      clearActiveKey,
      clearError,
      environmentHydrated,
      environmentId,
      environmentState.keys,
      error,
      hasPermission,
      hydrated,
      principal,
      refresh,
      removeKey,
      selectKey,
      status,
    ],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
