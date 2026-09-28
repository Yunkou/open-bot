import { test, expect } from "@playwright/test";
import { loginAdminUI } from "../../helpers/auth";
import { expectAdminPageTitle } from "../../helpers/admin";

const pages: { path: string; title: string }[] = [
  { path: "/users", title: "用户管理" },
  { path: "/bots", title: "Bot 管理" },
  { path: "/traces", title: "调用追踪" },
  { path: "/members", title: "成员与角色" },
  { path: "/llm", title: "默认模型" },
  { path: "/decision", title: "决策模型" },
  { path: "/usage", title: "用量" },
  { path: "/flags", title: "功能开关" },
  { path: "/audit", title: "审计日志" },
];

test.describe("admin pages smoke navigate", () => {
  test.beforeEach(async ({ page }) => {
    await loginAdminUI(page);
  });

  for (const p of pages) {
    test(`navigate ${p.path}`, async ({ page }) => {
      await page.goto(p.path);
      await expectAdminPageTitle(page, p.title);
    });
  }

  test("members invite form present", async ({ page }) => {
    await page.goto("/members");
    await expectAdminPageTitle(page, "成员与角色");
    await expect(page.locator(".ant-pro-table, form, .ant-table").first()).toBeVisible();
  });

  test("usage stats cards render", async ({ page }) => {
    await page.goto("/usage");
    await expect(page.getByText("成员数").first()).toBeVisible();
    await expect(page.getByText("会话数").first()).toBeVisible();
  });

  test("flags page shows JSON editor", async ({ page }) => {
    await page.goto("/flags");
    await expectAdminPageTitle(page, "功能开关");
    await expect(page.locator("textarea")).toBeVisible();
  });

  test("llm page shows model form", async ({ page }) => {
    await page.goto("/llm");
    await expectAdminPageTitle(page, "默认模型");
    await expect(page.getByLabel("模型").or(page.locator("#model")).first()).toBeVisible();
  });
});
