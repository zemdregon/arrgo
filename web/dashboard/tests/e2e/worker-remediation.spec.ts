import { test, expect } from "@playwright/test";
import { setupDashboardMocks } from "../utils/mockApi";

test.describe("Worker remediation journey", () => {
  test("operators can tune worker pool thresholds", async ({ page }) => {
    const mocks = await setupDashboardMocks(page);

    await page.goto("/");

    await expect(page.getByRole("heading", { name: "Worker pool configuration" })).toBeVisible();

    const workerPanel = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: "Worker pool configuration" }) });

    await workerPanel.getByRole("button", { name: /default automation/i }).click();

    await workerPanel.getByLabel("Heartbeat interval (s)").fill("25");
    await workerPanel.getByLabel("Stale after (s)").fill("90");
    await workerPanel.getByLabel("Max concurrent jobs").fill("8");

    await workerPanel.getByRole("button", { name: "Save configuration" }).click();

    const updatedPool = mocks.state.workerPools.pools.find(
      (pool) => pool.pool === "default"
    );
    expect(updatedPool?.heartbeat_interval_seconds).toBe(25);
    expect(updatedPool?.stale_after_seconds).toBe(90);
    expect(updatedPool?.max_concurrent_jobs).toBe(8);
  });
});
