import type { ReactNode } from "react";
import styles from "./AppShell.module.css";

type AppShellProps = {
  header?: ReactNode;
  sidebar?: ReactNode;
  children: ReactNode;
};

export function AppShell({ header, sidebar, children }: AppShellProps) {
  return (
    <div className={styles.root}>
      {sidebar ? <aside className={styles.sidebar}>{sidebar}</aside> : null}
      <div className={styles.contentArea}>
        {header ? <header className={styles.header}>{header}</header> : null}
        <main className={styles.main}>{children}</main>
      </div>
    </div>
  );
}
