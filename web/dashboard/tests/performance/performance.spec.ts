import { test, expect } from "@playwright/test";
import { setupDashboardMocks } from "../utils/mockApi";

test.describe("Performance budgets", () => {
  test("initial load, navigation, and API hydration stay within targets", async ({ page }) => {
    const { metrics } = await setupDashboardMocks(page);

    await page.goto("/");

    const initialTiming = await page.evaluate(() => {
      const [navigation] = performance.getEntriesByType("navigation") as PerformanceNavigationTiming[];
      return {
        domContentLoaded: navigation?.domContentLoadedEventEnd ?? Number.POSITIVE_INFINITY,
        loadEventEnd: navigation?.loadEventEnd ?? Number.POSITIVE_INFINITY,
      };
    });

    expect(initialTiming.domContentLoaded).toBeLessThanOrEqual(1_500);
    expect(initialTiming.loadEventEnd).toBeLessThanOrEqual(2_500);

    await page.getByRole("link", { name: "Requests" }).click();
    await page.waitForURL("**/requests");

    const navigationTiming = await page.evaluate(() => {
      const entries = performance.getEntriesByType("navigation") as PerformanceNavigationTiming[];
      const current = entries[entries.length - 1];
      return {
        domContentLoaded: current?.domContentLoadedEventEnd ?? Number.POSITIVE_INFINITY,
        responseStart: current?.responseStart ?? Number.POSITIVE_INFINITY,
      };
    });

    expect(navigationTiming.domContentLoaded).toBeLessThanOrEqual(1_200);
    expect(navigationTiming.responseStart).toBeLessThanOrEqual(300);

    expect(metrics.responses.length).toBeGreaterThan(0);
    for (const record of metrics.responses) {
      expect(record.duration).toBeLessThanOrEqual(200);
    }
  });
});
