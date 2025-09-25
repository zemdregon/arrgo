"use client";

import { DataPanel } from "../../../../components/DataPanel";
import { StatusPill } from "../../../../components/StatusPill";
import styles from "./SlaMetricsPanel.module.css";

type MetricTone = "success" | "warning" | "danger" | "neutral";

type Metric = {
  id: string;
  label: string;
  value: string;
  subtext: string;
  tone: MetricTone;
  delta: string;
};

const SLA_METRICS: Metric[] = [
  {
    id: "turnaround",
    label: "Average turnaround",
    value: "7.6h",
    subtext: "Target 12h",
    tone: "success",
    delta: "-0.8h vs last week",
  },
  {
    id: "backlog",
    label: "Backlog aging > 48h",
    value: "6",
    subtext: "Threshold 10",
    tone: "warning",
    delta: "-2 requests",
  },
  {
    id: "violations",
    label: "Policy violations (7d)",
    value: "3",
    subtext: "Escalated automatically",
    tone: "danger",
    delta: "+1 vs baseline",
  },
];

const BACKLOG_BREAKDOWN = [
  { label: "0-24h", value: "18 requests" },
  { label: "24-48h", value: "9 requests" },
  { label: "> 48h", value: "6 requests" },
];

const POLICY_BREACHES = [
  { label: "Quality downgrade", value: "2 (auto-waived)" },
  { label: "Quota override", value: "1 (manual approval)" },
  { label: "Region lock", value: "0" },
];

const TREND_SUMMARY = [
  { label: "On-time approvals", value: "92% (+4%)" },
  { label: "Median SLA", value: "5.8h" },
  { label: "Automation coverage", value: "68% of requests" },
];

interface SlaMetricsPanelProps {
  className?: string;
}

function toneForStatusPill(tone: MetricTone): "neutral" | "success" | "warning" | "danger" {
  switch (tone) {
    case "success":
      return "success";
    case "warning":
      return "warning";
    case "danger":
      return "danger";
    default:
      return "neutral";
  }
}

export function SlaMetricsPanel({ className }: SlaMetricsPanelProps) {
  return (
    <div className={className}>
      <DataPanel
        title="SLA performance"
        description="Track how quickly requests move through approvals, where backlogs accumulate, and which policies are being bypassed."
      >
        <div className={styles.content}>
          <div className={styles.metricsGrid}>
            {SLA_METRICS.map((metric) => (
              <article key={metric.id} className={styles.metricCard}>
                <div className={styles.metricHeader}>
                  <span className={styles.metricLabel}>{metric.label}</span>
                  <StatusPill label={metric.delta} tone={toneForStatusPill(metric.tone)} />
                </div>
                <span className={styles.metricValue}>{metric.value}</span>
                <span className={styles.metricSubtext}>{metric.subtext}</span>
              </article>
            ))}
          </div>

          <div className={styles.detailGrid}>
            <section className={styles.detailCard} aria-labelledby="sla-backlog-breakdown">
              <h3 id="sla-backlog-breakdown" className={styles.detailTitle}>
                Backlog aging
              </h3>
              <ul className={styles.detailList}>
                {BACKLOG_BREAKDOWN.map((item) => (
                  <li key={item.label} className={styles.detailItem}>
                    <span className={styles.detailLabel}>{item.label}</span>
                    <span className={styles.detailValue}>{item.value}</span>
                  </li>
                ))}
              </ul>
            </section>

            <section className={styles.detailCard} aria-labelledby="sla-policy-violations">
              <h3 id="sla-policy-violations" className={styles.detailTitle}>
                Policy violations
              </h3>
              <ul className={styles.detailList}>
                {POLICY_BREACHES.map((item) => (
                  <li key={item.label} className={styles.detailItem}>
                    <span className={styles.detailLabel}>{item.label}</span>
                    <span className={styles.detailValue}>{item.value}</span>
                  </li>
                ))}
              </ul>
            </section>

            <section className={styles.trendSummary} aria-labelledby="sla-trends">
              <h3 id="sla-trends" className={styles.trendTitle}>
                Trend highlights
              </h3>
              <ul className={styles.trendList}>
                {TREND_SUMMARY.map((trend) => (
                  <li key={trend.label}>
                    <span>{trend.value}</span> — {trend.label}
                  </li>
                ))}
              </ul>
            </section>
          </div>
        </div>
      </DataPanel>
    </div>
  );
}
