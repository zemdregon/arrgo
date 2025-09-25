import type { Meta, StoryObj } from "@storybook/react";
import { DataPanel } from "./DataPanel";
import { StatusPill } from "./StatusPill";

const meta: Meta<typeof DataPanel> = {
  title: "Components/DataPanel",
  component: DataPanel,
  parameters: {
    layout: "padded",
  },
  args: {
    title: "Library health",
    description:
      "Summaries that help operators understand ingest performance and queue conditions at a glance.",
  },
};

export default meta;

type Story = StoryObj<typeof DataPanel>;

export const Default: Story = {
  args: {
    children: (
      <div style={{ display: "grid", gap: "0.5rem" }}>
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <span>Pending imports</span>
          <strong>8</strong>
        </div>
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <span>Failed scans (24h)</span>
          <strong>2</strong>
        </div>
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <span>Automation coverage</span>
          <strong>86%</strong>
        </div>
      </div>
    ),
  },
};

export const Loading: Story = {
  args: {
    loading: true,
    children: <div style={{ height: "6rem" }} />,
  },
};

export const Error: Story = {
  args: {
    error: "Failed to reach arrgo API",
    onRetry: () => {
      // Intentionally empty for Storybook preview interactions.
    },
    children: null,
  },
};

export const WithActions: Story = {
  args: {
    actions: (
      <button type="button" style={{ padding: "0.5rem 0.75rem" }}>
        Refresh metrics
      </button>
    ),
    footer: (
      <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
        <StatusPill label="Healthy" tone="success" />
        <span>All services responding in under 350ms.</span>
      </div>
    ),
    children: (
      <p style={{ margin: 0 }}>
        The arrgo ingest pipeline is running nominally. The last import completed 12 minutes ago and
        no workers are reporting errors.
      </p>
    ),
  },
};
