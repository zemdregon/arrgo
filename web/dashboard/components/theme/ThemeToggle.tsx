"use client";

import { useTheme } from "./ThemeProvider";
import styles from "./ThemeToggle.module.css";

type ThemeToggleProps = {
  className?: string;
};

function SunIcon() {
  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      className={styles.icon}
      focusable="false"
    >
      <path
        d="M12 4.75a.75.75 0 0 1 .75-.75h.5a.75.75 0 0 1 .75.75v1a.75.75 0 0 1-.75.75h-.5A.75.75 0 0 1 12 5.75v-1zm-7 6.25a.75.75 0 0 0-.75-.75h-1a.75.75 0 0 0 0 1.5h1A.75.75 0 0 0 5 11zm14.75-.75a.75.75 0 0 0 0 1.5h1a.75.75 0 0 0 0-1.5h-1zM12 18.25a.75.75 0 0 0-.75.75v1a.75.75 0 1 0 1.5 0v-1A.75.75 0 0 0 12 18.25zm-6.01 1.071a.75.75 0 1 0-1.06 1.06l.707.708a.75.75 0 0 0 1.061-1.06l-.708-.708zm12.02 0-.708.708a.75.75 0 0 0 1.061 1.06l.708-.707a.75.75 0 1 0-1.06-1.061zM6.343 5.282a.75.75 0 1 0-1.06-1.06L4.575 4.93a.75.75 0 1 0 1.06 1.06l.708-.707zm11.314 0 .708.707a.75.75 0 0 0 1.06-1.06l-.707-.708a.75.75 0 1 0-1.061 1.06zM12 7.25a4.75 4.75 0 1 1 0 9.5 4.75 4.75 0 0 1 0-9.5zm0 1.5a3.25 3.25 0 1 0 0 6.5 3.25 3.25 0 0 0 0-6.5z"
        fill="currentColor"
      />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      className={styles.icon}
      focusable="false"
    >
      <path
        d="M11.748 2.029a.75.75 0 0 1 .759-.019A9.251 9.251 0 0 0 21.99 12.49a.75.75 0 0 1-1.154.63 6.752 6.752 0 0 1-8.975-8.976.75.75 0 0 1-.112-2.115z"
        fill="currentColor"
      />
      <path
        d="M12.25 4.04A7.751 7.751 0 1 0 19.96 11.75a8.3 8.3 0 0 1-7.71-7.71z"
        fill="currentColor"
        opacity="0.6"
      />
    </svg>
  );
}

export function ThemeToggle({ className }: ThemeToggleProps) {
  const { theme, toggleTheme } = useTheme();
  const isDark = theme === "dark";
  const label = isDark ? "Switch to light theme" : "Switch to dark theme";

  return (
    <button
      type="button"
      className={[styles.button, className].filter(Boolean).join(" ")}
      onClick={toggleTheme}
      aria-label={label}
    >
      <span className={styles.iconSlot}>{isDark ? <MoonIcon /> : <SunIcon />}</span>
      <span className={styles.label}>{isDark ? "Dark" : "Light"}</span>
    </button>
  );
}
