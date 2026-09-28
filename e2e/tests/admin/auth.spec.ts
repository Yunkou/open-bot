import { test, expect } from "@playwright/test";
import { adminLogoutUI, loginAdminUI, uniqueUsername, DEFAULT_USER_PASSWORD } from "../../helpers/auth";
import { apiRegister } from "../../helpers/api";
import { expectAdminPageTitle } from "../../helpers/admin";

test.describe("admin auth", () => {
  test("platform_admin can login", async ({ page }) => {
    await loginAdminUI(page);
    await expectAdminPageTitle(page, "用户管理");
    await expect(page).toHaveURL(/\/users/);
  });

  test("member is denied admin access", async ({ page }) => {
    const username = uniqueUsername("memdeny");
    await apiRegister(username, DEFAULT_USER_PASSWORD);
    await page.goto("/login");
    await page.getByRole("textbox", { name: "用户名" }).fill(username);
    await page.getByRole("textbox", { name: "密码" }).fill(DEFAULT_USER_PASSWORD);
    await page.getByRole("button", { name: /登\s*录/ }).click();
    await expect(page.getByText(/无管理端权限|没有权限|无权|拒绝/)).toBeVisible({ timeout: 15_000 });
  });

  test("logout returns to login", async ({ page }) => {
    await loginAdminUI(page);
    await adminLogoutUI(page);
    await expect(page.getByText("open-bot 管理端")).toBeVisible();
  });
});
