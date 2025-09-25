import { test, expect } from "@playwright/test";
import { setupDashboardMocks } from "../utils/mockApi";

test.describe("Request approval journey", () => {
  test("request approvers can triage and queue requests", async ({ page }) => {
    await setupDashboardMocks(page);

    await page.goto("/requests");

    await expect(page.getByRole("heading", { name: "Request management" })).toBeVisible();

    await page.getByRole("button", { name: "Private / staff" }).click();
    await expect(page.getByText("Quota exhausted").first()).not.toBeVisible();
    await expect(page.getByText("requests remaining", { exact: false })).toBeVisible();

    await page.getByLabel("Search media").fill("Foundation");
    await page.getByRole("button", { name: "Search", exact: true }).click();

    const requestButton = page.getByRole("button", { name: "Request" }).first();
    await requestButton.click();

    await expect(
      page.getByText("Foundation queued for staff review.", { exact: true })
    ).toBeVisible();
  });
});
