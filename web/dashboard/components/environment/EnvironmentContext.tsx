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

export type EnvironmentOption = {
  id: string;
  label: string;
  apiUrl: string;
  description?: string;
};

type EnvironmentContextValue = {
  environments: EnvironmentOption[];
  current: EnvironmentOption;
  setEnvironment: (id: string) => void;
  isHydrated: boolean;
};

const STORAGE_KEY = "arrgo:dashboard:environment";
const FALLBACK_API_URL = process.env.NEXT_PUBLIC_ARRGO_API_URL ?? "http://localhost:8080";

function normalizeId(value: string): string {
  return value.trim().toLowerCase();
}

function resolveDefaultEnvironments(): EnvironmentOption[] {
  const defaults: EnvironmentOption[] = [
    {
      id: "local",
      label: "Local",
      description: "Local workstation",
      apiUrl: FALLBACK_API_URL,
    },
    {
      id: "development",
      label: "Development",
      description: "Shared dev cluster",
      apiUrl: "https://dev.api.arrgo.dev",
    },
    {
      id: "staging",
      label: "Staging",
      description: "Pre-production",
      apiUrl: "https://staging.api.arrgo.dev",
    },
    {
      id: "production",
      label: "Production",
      description: "Live traffic",
      apiUrl: "https://api.arrgo.dev",
    },
  ];
  const raw = process.env.NEXT_PUBLIC_ARRGO_ENVIRONMENTS;
  if (!raw) {
    return defaults.map((env) => ({ ...env, id: normalizeId(env.id) }));
  }

  try {
    const parsed = JSON.parse(raw) as Array<Partial<EnvironmentOption>>;
    const sanitized: EnvironmentOption[] = parsed
      .map((item) => {
        const id = typeof item.id === "string" ? normalizeId(item.id) : undefined;
        const label = typeof item.label === "string" ? item.label.trim() : undefined;
        const apiUrl = typeof item.apiUrl === "string" ? item.apiUrl.trim() : undefined;
        const description =
          typeof item.description === "string" && item.description.trim()
            ? item.description.trim()
            : undefined;
        if (!id || !label || !apiUrl) {
          return undefined;
        }
        return { id, label, apiUrl, description };
      })
      .filter((value): value is EnvironmentOption => Boolean(value));

    if (sanitized.length) {
      return sanitized;
    }
  } catch (error) {
    // Ignore malformed configuration and fall back to defaults.
  }

  return defaults.map((env) => ({ ...env, id: normalizeId(env.id) }));
}

const ENVIRONMENT_OPTIONS = resolveDefaultEnvironments();
const DEFAULT_ENVIRONMENT_ID = (() => {
  const requested = process.env.NEXT_PUBLIC_ARRGO_DEFAULT_ENVIRONMENT;
  if (!requested) {
    return ENVIRONMENT_OPTIONS[0]?.id ?? "local";
  }
  const normalized = normalizeId(requested);
  const match = ENVIRONMENT_OPTIONS.find((option) => option.id === normalized);
  return match ? match.id : ENVIRONMENT_OPTIONS[0]?.id ?? "local";
})();

const EnvironmentContext = createContext<EnvironmentContextValue | undefined>(
  undefined,
);

export function EnvironmentProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [selectedId, setSelectedId] = useState<string>(DEFAULT_ENVIRONMENT_ID);
  const [isHydrated, setHydrated] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const normalized = normalizeId(stored);
        if (ENVIRONMENT_OPTIONS.some((option) => option.id === normalized)) {
          setSelectedId(normalized);
        }
      }
    } catch (error) {
      // localStorage might be unavailable; ignore and rely on defaults.
    }
    setHydrated(true);
  }, []);

  const setEnvironment = useCallback((id: string) => {
    const normalized = normalizeId(id);
    if (!ENVIRONMENT_OPTIONS.some((option) => option.id === normalized)) {
      return;
    }
    setSelectedId(normalized);
    if (typeof window !== "undefined") {
      try {
        window.localStorage.setItem(STORAGE_KEY, normalized);
      } catch (error) {
        // Ignore storage failures (e.g., private mode).
      }
    }
  }, []);

  const environmentMap = useMemo(() => {
    return new Map(ENVIRONMENT_OPTIONS.map((option) => [option.id, option]));
  }, []);

  const current = environmentMap.get(selectedId) ?? ENVIRONMENT_OPTIONS[0];

  const value = useMemo<EnvironmentContextValue>(
    () => ({
      environments: ENVIRONMENT_OPTIONS,
      current,
      setEnvironment,
      isHydrated,
    }),
    [current, isHydrated, setEnvironment],
  );

  return (
    <EnvironmentContext.Provider value={value}>
      {children}
    </EnvironmentContext.Provider>
  );
}

export function useEnvironment(): EnvironmentContextValue {
  const context = useContext(EnvironmentContext);
  if (!context) {
    throw new Error("useEnvironment must be used within an EnvironmentProvider");
  }
  return context;
}
