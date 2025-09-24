"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import styles from "./SidebarNavigation.module.css";

export type SidebarNavItem = {
  href: string;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
};

type SidebarNavigationProps = {
  primaryItems: SidebarNavItem[];
  secondaryItems?: SidebarNavItem[];
};

export function SidebarNavigation({
  primaryItems,
  secondaryItems = []
}: SidebarNavigationProps) {
  const pathname = usePathname();

  return (
    <nav className={styles.root} aria-label="Primary">
      <div className={styles.section}>
        <h2 className={styles.sectionLabel}>Navigation</h2>
        <ul className={styles.list}>
          {primaryItems.map((item) => {
            const isActive = pathname === item.href;
            const content = (
              <span className={styles.linkContent}>
                {item.icon ? <span className={styles.icon}>{item.icon}</span> : null}
                <span>{item.label}</span>
              </span>
            );

            return (
              <li key={item.href}>
                {item.disabled ? (
                  <span className={styles.link} data-disabled>
                    {content}
                  </span>
                ) : (
                  <Link
                    className={styles.link}
                    data-active={isActive ? "true" : undefined}
                    href={item.href}
                    prefetch={!item.href.startsWith("http")}
                  >
                    {content}
                  </Link>
                )}
              </li>
            );
          })}
        </ul>
      </div>
      {secondaryItems.length ? (
        <div className={styles.section}>
          <h2 className={styles.sectionLabel}>Resources</h2>
          <ul className={styles.list}>
            {secondaryItems.map((item) => {
              const content = (
                <span className={styles.linkContent}>
                  {item.icon ? <span className={styles.icon}>{item.icon}</span> : null}
                  <span>{item.label}</span>
                </span>
              );

              return (
                <li key={item.href}>
                  {item.disabled ? (
                    <span className={styles.link} data-disabled>
                      {content}
                    </span>
                  ) : item.href.startsWith("http") ? (
                    <a
                      className={styles.link}
                      href={item.href}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {content}
                    </a>
                  ) : (
                    <Link className={styles.link} href={item.href} prefetch>
                      {content}
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
    </nav>
  );
}
