"use client";

import {
  createContext,
  PropsWithChildren,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

export type ToastVariant = "info" | "success" | "warning" | "error";

export type ToastOptions = {
  id?: string;
  title?: string;
  description?: string;
  variant?: ToastVariant;
  duration?: number;
};

export type ToastInstance = {
  id: string;
  title?: string;
  description?: string;
  variant: ToastVariant;
  duration: number;
  createdAt: number;
};

type ToastContextValue = {
  toasts: ToastInstance[];
  push: (toast: ToastOptions) => string;
  dismiss: (id: string) => void;
  clear: () => void;
};

const DEFAULT_DURATION = 6000;

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

function generateToastId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `toast-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function sanitizeDuration(duration?: number): number {
  if (!duration || Number.isNaN(duration)) {
    return DEFAULT_DURATION;
  }
  return Math.max(0, duration);
}

export function ToastProvider({ children }: PropsWithChildren) {
  const [toasts, setToasts] = useState<ToastInstance[]>([]);
  const timersRef = useRef<Map<string, number>>(new Map());

  const removeTimer = useCallback((id: string) => {
    const timer = timersRef.current.get(id);
    if (timer !== undefined) {
      if (typeof window !== "undefined") {
        window.clearTimeout(timer);
      } else {
        clearTimeout(timer);
      }
      timersRef.current.delete(id);
    }
  }, []);

  const dismiss = useCallback(
    (id: string) => {
      removeTimer(id);
      setToasts((current) => current.filter((toast) => toast.id !== id));
    },
    [removeTimer],
  );

  const scheduleRemoval = useCallback(
    (id: string, duration: number) => {
      removeTimer(id);
      if (duration <= 0 || typeof window === "undefined") {
        return;
      }
      const timer = window.setTimeout(() => {
        setToasts((current) => current.filter((toast) => toast.id !== id));
        timersRef.current.delete(id);
      }, duration);
      timersRef.current.set(id, timer);
    },
    [removeTimer],
  );

  const push = useCallback(
    (toast: ToastOptions) => {
      const id = toast.id?.trim() || generateToastId();
      const duration = sanitizeDuration(toast.duration);
      const instance: ToastInstance = {
        id,
        title: toast.title?.trim() || undefined,
        description: toast.description?.trim() || undefined,
        variant: toast.variant ?? "info",
        duration,
        createdAt: Date.now(),
      };
      setToasts((current) => {
        const existingIndex = current.findIndex((item) => item.id === id);
        if (existingIndex >= 0) {
          const next = [...current];
          next[existingIndex] = instance;
          return next;
        }
        return [instance, ...current];
      });
      scheduleRemoval(id, duration);
      return id;
    },
    [scheduleRemoval],
  );

  const clear = useCallback(() => {
    for (const id of Array.from(timersRef.current.keys())) {
      removeTimer(id);
    }
    setToasts([]);
  }, [removeTimer]);

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const handle of Array.from(timers.values())) {
        if (typeof window !== "undefined") {
          window.clearTimeout(handle);
        } else {
          clearTimeout(handle);
        }
      }
      timers.clear();
    };
  }, []);

  const value = useMemo<ToastContextValue>(
    () => ({
      toasts,
      push,
      dismiss,
      clear,
    }),
    [clear, dismiss, push, toasts],
  );

  return <ToastContext.Provider value={value}>{children}</ToastContext.Provider>;
}

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
}
