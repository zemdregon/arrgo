import styles from "./StatusPill.module.css";

type Tone = "neutral" | "success" | "warning" | "danger";

interface StatusPillProps {
  label: string;
  tone?: Tone;
}

export function StatusPill({ label, tone = "neutral" }: StatusPillProps) {
  return <span className={`${styles.pill} ${styles[tone]}`}>{label}</span>;
}
