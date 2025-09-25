import { defineConfig, devices } from "@playwright/test";

const STORYBOOK_PORT = Number(process.env.STORYBOOK_PORT ?? 6006);

export default defineConfig({
  testDir: "./tests/visual",
  timeout: 45_000,
  expect: {
    toHaveScreenshot: {
      animations: "disabled",
      maxDiffPixelRatio: 0.02,
    },
  },
  reporter: [["list"]],
  use: {
    ...devices["Desktop Chrome"],
  },
  webServer: {
    command: `npm run storybook -- --ci --port ${STORYBOOK_PORT}`,
    url: `http://127.0.0.1:${STORYBOOK_PORT}`,
    reuseExistingServer: !process.env.CI,
    stdout: "pipe",
    stderr: "pipe",
    timeout: 120_000,
  },
});
