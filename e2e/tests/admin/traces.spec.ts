import { test, expect } from "@playwright/test";
import { loginAdminUI } from "../../helpers/auth";
import { expectAdminPageTitle } from "../../helpers/admin";

test.describe("admin traces", () => {
  test("traces list or empty-state / Langfuse status", async ({ page }) => {
    await loginAdminUI(page);
    await page.goto("/traces");
    await expectAdminPageTitle(page, "调用追踪");

    const empty = page.getByText(/追踪未启用或不可用|无数据|暂无/);
    const table = page.locator(".ant-table, .ant-pro-table");
    const detailLink = page.getByText("详情");

    await expect(empty.or(table).first()).toBeVisible({ timeout: 20_000 });

    if (await detailLink.count()) {
      await detailLink.first().click();
      await expect(page.locator(".ant-drawer")).toBeVisible();
      const lf = page.locator('a[href*="traces"], a[href*="langfuse"], a[href*="/project/"]');
      if (await lf.count()) {
        const href = await lf.first().getAttribute("href");
        expect(href || "").toMatch(/trace|project/i);
      }
    }
  });
});
