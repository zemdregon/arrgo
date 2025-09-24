import type { ReactNode } from "react";
import { AppShell } from "../../components/layout/AppShell";
import { SidebarNavigation } from "../../components/layout/SidebarNavigation";
import { ThemeToggle } from "../../components/theme/ThemeToggle";
import styles from "./layout.module.css";

const NAVIGATION_ITEMS = [
  { href: "/", label: "Overview" },
  { href: "/library", label: "Library", disabled: true },
  { href: "/requests", label: "Requests", disabled: true },
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
          <div className={styles.controls}>
            <div className={styles.environmentGroup}>
              <span className={styles.environmentLabel}>Environment</span>
              <span className={styles.environmentValue}>Local</span>
            </div>
            <ThemeToggle />
          </div>
        </div>
      }
    >
      {children}
    </AppShell>
  );
}
