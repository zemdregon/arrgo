import { test, expect } from "@playwright/test";
import { setupDashboardMocks } from "../utils/mockApi";

test.describe("Library ingest journey", () => {
  test("operators can run manual searches and queue downloads", async ({ page }) => {
    await setupDashboardMocks(page);

    await page.goto("/");

    await expect(page.getByRole("heading", { name: "Manual indexer search" })).toBeVisible();

    const manualPanel = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: "Manual indexer search" }) });

    await manualPanel.getByLabel("Search query").fill("4K restoration");
    await manualPanel.getByRole("button", { name: "Search indexers" }).click();

    const feedback = manualPanel.getByRole("status");
    await expect(feedback).toContainText("Search returned 1 release");

    const grabButton = manualPanel.getByRole("button", { name: "Grab" }).first();
    await grabButton.click();

    await expect(feedback).toContainText(
      "Marked \"Foundation S01E01 1080p WEB\" as grabbed."
    );

    const resultRow = manualPanel.getByRole("row", {
      name: /Foundation S01E01 1080p WEB/
    });
    await expect(resultRow.getByText("grabbed", { exact: true })).toBeVisible();
  });
});
