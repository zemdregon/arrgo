"use client";

import { ChangeEvent, useId } from "react";
import { useEnvironment } from "./EnvironmentContext";
import styles from "./EnvironmentSwitcher.module.css";

function formatMetaLabel(apiUrl: string, description?: string): string {
  if (description) {
    return description;
  }
  try {
    const url = new URL(apiUrl);
    return url.host;
  } catch (error) {
    return apiUrl;
  }
}

export function EnvironmentSwitcher() {
  const { environments, current, setEnvironment, isHydrated } = useEnvironment();
  const selectId = useId();
  const labelId = `${selectId}-label`;

  const handleChange = (event: ChangeEvent<HTMLSelectElement>) => {
    setEnvironment(event.target.value);
  };

  const metaLabel = formatMetaLabel(current.apiUrl, current.description);
  const disabled = !isHydrated || environments.length <= 1;

  return (
    <div className={styles.root}>
      <span className={styles.label} id={labelId}>
        Environment
      </span>
      <select
        id={selectId}
        aria-labelledby={labelId}
        className={styles.select}
        value={current.id}
        onChange={handleChange}
        disabled={disabled}
        title={metaLabel}
      >
        {environments.map((environment) => (
          <option key={environment.id} value={environment.id}>
            {environment.label}
          </option>
        ))}
      </select>
      <span className={styles.meta} aria-live="polite">
        {metaLabel}
      </span>
    </div>
  );
}
