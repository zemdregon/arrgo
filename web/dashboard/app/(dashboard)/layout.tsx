import type { ReactNode } from "react";
import { AppShell } from "../../components/layout/AppShell";
import { SidebarNavigation } from "../../components/layout/SidebarNavigation";
import { EnvironmentSwitcher } from "../../components/environment/EnvironmentSwitcher";
import { WorkerStatusIndicator } from "../../components/workers/WorkerStatusIndicator";
import { ThemeToggle } from "../../components/theme/ThemeToggle";
import { ApiKeySelector } from "../../components/auth/ApiKeySelector";
import { AuthGuard } from "../../components/auth/AuthGuard";
import styles from "./layout.module.css";

const NAVIGATION_ITEMS = [
  { href: "/", label: "Overview" },
  { href: "/queue", label: "Queue analytics" },
  { href: "/library", label: "Library", disabled: true },
  { href: "/requests", label: "Requests" },
  { href: "/workers", label: "Workers", disabled: true },
  { href: "/automation", label: "Automation", disabled: true }
];

const SECONDARY_LINKS = [
  { href: "https://docs.arrgo.dev", label: "Documentation" },
  { href: "https://status.arrgo.dev", label: "Status", disabled: true },
  { href: "https://github.com/arrgo", label: "GitHub" }
];

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return (
    <AppShell
      sidebar={
        <SidebarNavigation
          primaryItems={NAVIGATION_ITEMS}
          secondaryItems={SECONDARY_LINKS}
        />
      }
      header={
        <div className={styles.headerBar}>
          <div className={styles.brandGroup}>
            <span className={styles.brand}>arrgo</span>
            <span className={styles.product}>dashboard</span>
          </div>
          <div className={styles.headerControls}>
            <WorkerStatusIndicator />
            <ApiKeySelector className={styles.apiKeySelector} />
            <EnvironmentSwitcher />
            <ThemeToggle className={styles.themeToggle} />
          </div>
        </div>
      }
    >
      <AuthGuard requiredPermissions={["library:read", "worker:read"]}>
        {children}
      </AuthGuard>
    </AppShell>
  );
}
