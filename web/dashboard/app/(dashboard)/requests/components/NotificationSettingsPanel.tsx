"use client";

import { FormEvent, useState } from "react";
import { DataPanel } from "../../../components/DataPanel";
import { StatusPill } from "../../../components/StatusPill";
import styles from "./NotificationSettingsPanel.module.css";

type NotificationSettings = {
  emailEnabled: boolean;
  emailRecipients: string;
  webhookEnabled: boolean;
  webhookUrl: string;
  webhookSecret: string;
  pushEnabled: boolean;
  pushProvider: "ntfy" | "gotify" | "pushover";
  pushTopic: string;
};

const DEFAULT_SETTINGS: NotificationSettings = {
  emailEnabled: true,
  emailRecipients: "ops@arrgo.dev, guild-leads@arrgo.dev",
  webhookEnabled: true,
  webhookUrl: "https://hooks.arrgo.dev/v1/request-outcomes",
  webhookSecret: "arrgo-signature",
  pushEnabled: false,
  pushProvider: "ntfy",
  pushTopic: "",
};

interface NotificationSettingsPanelProps {
  className?: string;
}

export function NotificationSettingsPanel({
  className,
}: NotificationSettingsPanelProps) {
  const [settings, setSettings] = useState<NotificationSettings>(DEFAULT_SETTINGS);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSaving(true);
    setFeedback(null);
    window.setTimeout(() => {
      setSaving(false);
      setFeedback("Notification preferences saved. Integrations queued for redeploy.");
    }, 450);
  };

  return (
    <div className={className}>
      <DataPanel
        title="Notification policies"
        description="Control who gets notified when requests are approved, denied, or escalated through automations."
      >
        <div className={styles.content}>
          <form className={styles.form} onSubmit={handleSubmit}>
            <section className={styles.section} aria-labelledby="notification-email">
              <div className={styles.sectionHeader}>
                <div>
                  <h3 id="notification-email" className={styles.sectionTitle}>
                    Email summaries
                  </h3>
                  <p className={styles.sectionDescription}>
                    Nightly digest and immediate notifications for approver and requester updates.
                  </p>
                </div>
                <StatusPill
                  label={settings.emailEnabled ? "Active" : "Disabled"}
                  tone={settings.emailEnabled ? "success" : "danger"}
                />
              </div>
              <div className={styles.toggleRow}>
                <label htmlFor="email-toggle">Send emails</label>
                <input
                  id="email-toggle"
                  type="checkbox"
                  checked={settings.emailEnabled}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      emailEnabled: event.target.checked,
                    }))
                  }
                />
              </div>
              <div className={styles.inputGroup}>
                <label htmlFor="email-recipients">Distribution list</label>
                <textarea
                  id="email-recipients"
                  value={settings.emailRecipients}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      emailRecipients: event.target.value,
                    }))
                  }
                  disabled={!settings.emailEnabled}
                  placeholder="people@arrgo.dev"
                />
              </div>
            </section>

            <section className={styles.section} aria-labelledby="notification-webhook">
              <div className={styles.sectionHeader}>
                <div>
                  <h3 id="notification-webhook" className={styles.sectionTitle}>
                    Webhook delivery
                  </h3>
                  <p className={styles.sectionDescription}>
                    Send structured JSON payloads to downstream automations whenever request status changes.
                  </p>
                </div>
                <StatusPill
                  label={settings.webhookEnabled ? "Active" : "Disabled"}
                  tone={settings.webhookEnabled ? "success" : "danger"}
                />
              </div>
              <div className={styles.toggleRow}>
                <label htmlFor="webhook-toggle">Enable webhook</label>
                <input
                  id="webhook-toggle"
                  type="checkbox"
                  checked={settings.webhookEnabled}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      webhookEnabled: event.target.checked,
                    }))
                  }
                />
              </div>
              <div className={styles.inputGroup}>
                <label htmlFor="webhook-url">Webhook URL</label>
                <input
                  id="webhook-url"
                  type="url"
                  value={settings.webhookUrl}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      webhookUrl: event.target.value,
                    }))
                  }
                  disabled={!settings.webhookEnabled}
                  placeholder="https://hooks.arrgo.dev/v1/request-outcomes"
                />
              </div>
              <div className={styles.inputGroup}>
                <label htmlFor="webhook-secret">Signing secret</label>
                <input
                  id="webhook-secret"
                  type="text"
                  value={settings.webhookSecret}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      webhookSecret: event.target.value,
                    }))
                  }
                  disabled={!settings.webhookEnabled}
                  placeholder="Shared secret used to validate requests"
                />
              </div>
            </section>

            <section className={styles.section} aria-labelledby="notification-push">
              <div className={styles.sectionHeader}>
                <div>
                  <h3 id="notification-push" className={styles.sectionTitle}>
                    Push integrations
                  </h3>
                  <p className={styles.sectionDescription}>
                    Send real-time alerts to mobile and chat channels with provider-specific configuration.
                  </p>
                </div>
                <StatusPill
                  label={settings.pushEnabled ? "Active" : "Disabled"}
                  tone={settings.pushEnabled ? "success" : "neutral"}
                />
              </div>
              <div className={styles.toggleRow}>
                <label htmlFor="push-toggle">Enable push alerts</label>
                <input
                  id="push-toggle"
                  type="checkbox"
                  checked={settings.pushEnabled}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      pushEnabled: event.target.checked,
                    }))
                  }
                />
              </div>
              <div className={styles.inputGroup}>
                <label htmlFor="push-provider">Provider</label>
                <select
                  id="push-provider"
                  value={settings.pushProvider}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      pushProvider: event.target.value as NotificationSettings["pushProvider"],
                    }))
                  }
                  disabled={!settings.pushEnabled}
                >
                  <option value="ntfy">ntfy</option>
                  <option value="gotify">Gotify</option>
                  <option value="pushover">Pushover</option>
                </select>
              </div>
              <div className={styles.inputGroup}>
                <label htmlFor="push-topic">Topic or channel</label>
                <input
                  id="push-topic"
                  type="text"
                  value={settings.pushTopic}
                  onChange={(event) =>
                    setSettings((current) => ({
                      ...current,
                      pushTopic: event.target.value,
                    }))
                  }
                  disabled={!settings.pushEnabled}
                  placeholder="arrgo/requests"
                />
              </div>
            </section>

            <div className={styles.actions}>
              <button className={styles.saveButton} type="submit" disabled={saving}>
                {saving ? "Saving…" : "Save preferences"}
              </button>
            </div>
            {feedback ? <div className={styles.feedback}>{feedback}</div> : null}
          </form>
        </div>
      </DataPanel>
    </div>
  );
}
