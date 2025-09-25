"use client";

import { PageHeader } from "../../../components/layout/PageHeader";
import { RequestIntakePanel } from "./components/RequestIntakePanel";
import { SlaMetricsPanel } from "./components/SlaMetricsPanel";
import { NotificationSettingsPanel } from "./components/NotificationSettingsPanel";
import { ApprovalWorkflowPanel } from "./components/ApprovalWorkflowPanel";
import styles from "./page.module.css";

export default function RequestsPage() {
  return (
    <div className={styles.page}>
      <PageHeader
        eyebrow="arrgo"
        title="Request management"
        description="Coordinate request intake, approvals, notifications, and service-level tracking without leaving the dashboard."
        actions={
          <button className={styles.primaryAction} type="button">
            Export SLA report
          </button>
        }
      />
      <div className={styles.grid}>
        <RequestIntakePanel />
        <SlaMetricsPanel />
        <NotificationSettingsPanel />
        <ApprovalWorkflowPanel className={styles.fullWidth} />
      </div>
    </div>
  );
}
