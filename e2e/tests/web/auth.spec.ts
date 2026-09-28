import { test, expect } from "@playwright/test";
import { loginWebUI, registerWebUI, uniqueUsername, DEFAULT_USER_PASSWORD } from "../../helpers/auth";

test.describe("web auth", () => {
  test("register then logout then login", async ({ page }) => {
    const username = uniqueUsername("webauth");
    const password = DEFAULT_USER_PASSWORD;

    await registerWebUI(page, username, password);
    await expect(page.locator("button.settings-btn.ghost", { hasText: "退出登录" })).toContainText(username);

    await page.locator("button.settings-btn.ghost", { hasText: "退出登录" }).click();
    await expect(page.locator(".auth-page")).toBeVisible();
    await expect(page.locator(".brand-title")).toHaveText("open-bot");

    await loginWebUI(page, username, password);
    await expect(page.locator(".sidebar")).toBeVisible();
    await expect(page.locator("button.settings-btn.ghost", { hasText: "退出登录" })).toContainText(username);
  });

  test("login with wrong password shows error", async ({ page }) => {
    await page.goto("/");
    await page.locator(".auth-tabs button", { hasText: "登录" }).click();
    await page.locator('input[autocomplete="username"]').fill("definitely_not_a_user_xyz");
    await page.locator('input[type="password"]').fill("wrong-pass");
    await page.locator('button.primary[type="submit"]').click();
    await expect(page.locator(".auth-error")).toBeVisible({ timeout: 10_000 });
  });
});
