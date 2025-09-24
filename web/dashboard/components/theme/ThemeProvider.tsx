"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

const STORAGE_KEY = "arrgo:theme";

type ThemeMode = "light" | "dark";

type ThemeContextValue = {
  theme: ThemeMode;
  setTheme: (mode: ThemeMode) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

function applyTheme(mode: ThemeMode) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  root.dataset.theme = mode;
}

function resolveSystemPreference(): ThemeMode {
  if (typeof window === "undefined") {
    return "dark";
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

type ThemeProviderProps = {
  children: ReactNode;
};

export function ThemeProvider({ children }: ThemeProviderProps) {
  const hasUserPreference = useRef(false);
  const [initialized, setInitialized] = useState(false);
  const [theme, setThemeState] = useState<ThemeMode>(() => "dark");

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "dark" || stored === "light") {
      hasUserPreference.current = true;
      setThemeState(stored);
      applyTheme(stored);
    } else {
      const system = resolveSystemPreference();
      setThemeState(system);
      applyTheme(system);
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = (event: MediaQueryListEvent) => {
      if (!hasUserPreference.current) {
        const nextTheme: ThemeMode = event.matches ? "dark" : "light";
        setThemeState(nextTheme);
        applyTheme(nextTheme);
      }
    };
    media.addEventListener("change", listener);
    setInitialized(true);
    return () => media.removeEventListener("change", listener);
  }, []);

  useEffect(() => {
    if (!initialized || typeof window === "undefined") {
      return;
    }
    applyTheme(theme);
    if (hasUserPreference.current) {
      window.localStorage.setItem(STORAGE_KEY, theme);
    } else {
      window.localStorage.removeItem(STORAGE_KEY);
    }
  }, [theme, initialized]);

  const setTheme = useCallback((mode: ThemeMode) => {
    hasUserPreference.current = true;
    setThemeState(mode);
  }, []);

  const toggleTheme = useCallback(() => {
    setThemeState((current) => {
      const nextMode: ThemeMode = current === "dark" ? "light" : "dark";
      hasUserPreference.current = true;
      return nextMode;
    });
  }, []);

  const value = useMemo(
    () => ({
      theme,
      setTheme,
      toggleTheme,
    }),
    [theme, setTheme, toggleTheme],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
}
