"use client";

import { useToast, type ToastVariant } from "./ToastProvider";
import styles from "./ToastViewport.module.css";

const VARIANT_CLASS: Record<ToastVariant, string> = {
  info: styles.info,
  success: styles.success,
  warning: styles.warning,
  error: styles.error,
};

function resolveRole(variant: ToastVariant): "alert" | "status" {
  return variant === "error" || variant === "warning" ? "alert" : "status";
}

export function ToastViewport() {
  const { toasts, dismiss } = useToast();

  if (toasts.length === 0) {
    return null;
  }

  return (
    <div className={styles.viewport} role="region" aria-live="polite" aria-label="Notifications">
      {toasts.map((toast) => {
        const variantClass = VARIANT_CLASS[toast.variant];
        const role = resolveRole(toast.variant);
        return (
          <div
            key={toast.id}
            className={[styles.toast, variantClass].filter(Boolean).join(" ")}
            role={role}
            tabIndex={0}
          >
            <div className={styles.content}>
              {toast.title ? <p className={styles.title}>{toast.title}</p> : null}
              {toast.description ? (
                <p className={styles.description}>{toast.description}</p>
              ) : null}
            </div>
            <button
              type="button"
              className={styles.dismiss}
              aria-label="Dismiss notification"
              onClick={() => dismiss(toast.id)}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
