"use client";

import { ReactNode, useMemo } from "react";
import { ApiKeySelector } from "./ApiKeySelector";
import { useAuth } from "./AuthContext";
import styles from "./AuthGuard.module.css";

type AuthGuardProps = {
  requiredPermissions?: string[];
  children: ReactNode;
};

export function AuthGuard({
  requiredPermissions = [],
  children,
}: AuthGuardProps) {
  const {
    isHydrated,
    status,
    activeKey,
    error,
    refresh,
    clearError,
    hasPermission,
  } = useAuth();

  const permissions = useMemo(
    () =>
      requiredPermissions
        .map((permission) => permission.trim())
        .filter((permission) => permission.length > 0),
    [requiredPermissions],
  );

  const missingPermissions = useMemo(
    () => permissions.filter((permission) => !hasPermission(permission)),
    [hasPermission, permissions],
  );

  if (!isHydrated) {
    return (
      <div className={styles.guard}>
        <div className={styles.card}>
          <h2 className={styles.title}>Preparing dashboard</h2>
          <p className={styles.message}>
            Loading environment configuration and authentication state…
          </p>
        </div>
      </div>
    );
  }

  if (!activeKey) {
    return (
      <div className={styles.guard}>
        <div className={styles.card}>
          <h2 className={styles.title}>Connect an arrgo API key</h2>
          <p className={styles.message}>
            Provide an API key with the required permissions to access this
            dashboard view. Saved keys are scoped per environment and stored in
            your browser.
          </p>
          <div className={styles.selectorContainer}>
            <ApiKeySelector variant="inline" />
          </div>
        </div>
      </div>
    );
  }

  if (status === "loading") {
    return (
      <div className={styles.guard}>
        <div className={styles.card}>
          <h2 className={styles.title}>Authenticating API key…</h2>
          <p className={styles.message}>
            Verifying permissions and loading your access profile.
          </p>
        </div>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className={styles.guard}>
        <div className={styles.card}>
          <h2 className={styles.title}>Authentication failed</h2>
          <p className={styles.message}>{error ?? "Unable to verify API key."}</p>
          <div className={styles.actions}>
            <button
              type="button"
              className={styles.primaryButton}
              onClick={() => {
                clearError();
                refresh();
              }}
            >
              Retry
            </button>
          </div>
          <div className={styles.selectorContainer}>
            <ApiKeySelector variant="inline" />
          </div>
        </div>
      </div>
    );
  }

  if (missingPermissions.length > 0) {
    return (
      <div className={styles.guard}>
        <div className={styles.card}>
          <h2 className={styles.title}>Insufficient permissions</h2>
          <p className={styles.message}>
            The active API key does not grant the permissions required for this
            dashboard section.
          </p>
          <ul className={styles.permissionList}>
            {missingPermissions.map((permission) => (
              <li key={permission} className={styles.permissionItem}>
                {permission}
              </li>
            ))}
          </ul>
          <div className={styles.selectorContainer}>
            <ApiKeySelector variant="inline" />
          </div>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
