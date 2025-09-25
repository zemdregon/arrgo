import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { setupDashboardMocks } from "../utils/mockApi";

test.describe("Accessibility and keyboard navigation", () => {
  test("dashboard overview meets WCAG expectations", async ({ page }) => {
    await setupDashboardMocks(page);
    await page.goto("/");

    await expect(page.getByRole("heading", { name: "Manual indexer search" })).toBeVisible();

    const results = await new AxeBuilder({ page }).analyze();
    expect(results.violations).toEqual([]);
  });

  test("request workflows support accessible keyboard navigation", async ({ page }) => {
    await setupDashboardMocks(page);
    await page.goto("/requests");

    await expect(page.getByRole("heading", { name: "Request management" })).toBeVisible();

    const requestResults = await new AxeBuilder({ page }).analyze();
    expect(requestResults.violations).toEqual([]);

    await page.keyboard.press("Control+K");
    const palette = page.getByRole("dialog", { name: "Command menu" });
    await expect(palette).toBeVisible();

    await page.keyboard.type("Overview");
    await page.keyboard.press("Enter");

    await page.waitForURL("**/");
    await expect(page.getByRole("heading", { name: "Manual indexer search" })).toBeVisible();
  });
});
