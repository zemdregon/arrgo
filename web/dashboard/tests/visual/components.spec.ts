import { test, expect } from "@playwright/test";

const storybookPort = Number(process.env.STORYBOOK_PORT ?? 6006);
const storybookBase = `http://127.0.0.1:${storybookPort}`;

const stories: Array<{ id: string; snapshot: string }> = [
  { id: "components-datapanel--default", snapshot: "data-panel-default.png" },
  { id: "components-datapanel--loading", snapshot: "data-panel-loading.png" },
  { id: "components-datapanel--error", snapshot: "data-panel-error.png" },
  { id: "components-datapanel--with-actions", snapshot: "data-panel-with-actions.png" },
  { id: "components-statuspill--neutral", snapshot: "status-pill-neutral.png" },
  { id: "components-statuspill--success", snapshot: "status-pill-success.png" },
  { id: "components-statuspill--warning", snapshot: "status-pill-warning.png" },
  { id: "components-statuspill--danger", snapshot: "status-pill-danger.png" },
];

for (const story of stories) {
  test(`visual regression for ${story.id}`, async ({ page }) => {
    await page.goto(`${storybookBase}/iframe.html?id=${story.id}&viewMode=story`);
    await page.waitForSelector("#storybook-root", { state: "visible" });
    await page.waitForTimeout(200);
    await expect(page.locator("#storybook-root")).toHaveScreenshot(story.snapshot);
  });
}
