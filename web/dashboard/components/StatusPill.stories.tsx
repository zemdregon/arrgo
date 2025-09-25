import type { Meta, StoryObj } from "@storybook/react";
import { StatusPill } from "./StatusPill";

const meta: Meta<typeof StatusPill> = {
  title: "Components/StatusPill",
  component: StatusPill,
  args: {
    label: "Neutral",
    tone: "neutral",
  },
};

export default meta;

type Story = StoryObj<typeof StatusPill>;

export const Neutral: Story = {};

export const Success: Story = {
  args: {
    label: "Healthy",
    tone: "success",
  },
};

export const Warning: Story = {
  args: {
    label: "Attention",
    tone: "warning",
  },
};

export const Danger: Story = {
  args: {
    label: "Critical",
    tone: "danger",
  },
};
