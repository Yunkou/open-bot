import { test, expect, type Page } from "@playwright/test";
import { loginAdminUI, uniqueUsername, DEFAULT_USER_PASSWORD } from "../../helpers/auth";
import { expectAdminPageTitle } from "../../helpers/admin";

async function expectTextInPagedTable(page: Page, text: string): Promise<void> {
  const match = page.getByText(text, { exact: false });
  if (await match.count()) {
    await expect(match.first()).toBeVisible();
    return;
  }
  const last = page.locator(".ant-pagination-item").last();
  if (await last.count()) {
    await last.click();
  }
  await expect(page.getByText(text).first()).toBeVisible({ timeout: 15_000 });
}

test.describe("admin users & bots CRUD smoke", () => {
  test.beforeEach(async ({ page }) => {
    await loginAdminUI(page);
  });

  test("users list loads and create user modal works", async ({ page }) => {
    await page.goto("/users");
    await expectAdminPageTitle(page, "用户管理");
    await expect(page.getByRole("button", { name: /新\s*建\s*用\s*户/ })).toBeVisible();

    const uname = uniqueUsername("admuser");
    await page.getByRole("button", { name: /新\s*建\s*用\s*户/ }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    await dialog.locator("#username").fill(uname);
    await dialog.locator("#password").fill(DEFAULT_USER_PASSWORD);

    const createResp = page.waitForResponse(
      (r) => r.url().includes("/v1/admin/users") && r.request().method() === "POST",
      { timeout: 20_000 },
    );
    await dialog.getByRole("button", { name: /确\s*定/ }).click();
    const resp = await createResp;
    expect(resp.status(), await resp.text()).toBe(201);

    await expect(dialog).toBeHidden({ timeout: 10_000 });
    await expectTextInPagedTable(page, uname);
  });

  test("bots page create bot smoke", async ({ page }) => {
    await page.goto("/bots");
    await expectAdminPageTitle(page, "Bot 管理");
    await expect(page.getByRole("button", { name: /新\s*建\s*Bot/ })).toBeVisible();

    await page.getByRole("button", { name: /新\s*建\s*Bot/ }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    await dialog.getByLabel("所属用户").click();
    await page.locator(".ant-select-dropdown:visible .ant-select-item-option").first().click();

    const botName = `E2E Bot ${Date.now().toString(36)}`;
    await dialog.locator("#name").fill(botName);
    await dialog.locator("#description").fill("created by e2e");

    const createResp = page.waitForResponse(
      (r) => r.url().includes("/v1/admin/bots") && r.request().method() === "POST",
      { timeout: 20_000 },
    );
    await dialog.getByRole("button", { name: /确\s*定/ }).click();
    const resp = await createResp;
    expect(resp.status(), await resp.text()).toBeLessThan(300);

    await expect(dialog).toBeHidden({ timeout: 10_000 });
    await expectTextInPagedTable(page, botName);
  });
});
