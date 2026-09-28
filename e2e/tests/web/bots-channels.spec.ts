import { test, expect } from "@playwright/test";
import { apiCreateAgent } from "../../helpers/api";
import { injectWebSession, registerFreshWebUser } from "../../helpers/auth";
import { createBotViaUI, createGroupViaUI, openAgentByName } from "../../helpers/web";

test.describe("web bots & channels", () => {
  test("create bot via UI and open chat", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "webbot", withMockLLM: true });
    await injectWebSession(page, session);
    await page.goto("/");
    await createBotViaUI(page, "UI Created Bot");
    await expect(page.locator(".composer textarea")).toBeVisible();
  });

  test("create group chat with two members", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "webch", withMockLLM: true });
    const a = await apiCreateAgent(session.token, { name: "Group Member One" });
    const b = await apiCreateAgent(session.token, { name: "Group Member Two" });
    await injectWebSession(page, session);
    await page.goto("/");
    await expect(page.locator(".agent-list .agent-item", { hasText: a.name })).toBeVisible();
    await createGroupViaUI(page, "E2E Group", [a.name, b.name]);
    await expect(page.locator(".channel-list .channel-item", { hasText: "E2E Group" })).toBeVisible();
    await page.locator(".channel-list .channel-item", { hasText: "E2E Group" }).click();
    await expect(page.locator(".composer textarea")).toHaveAttribute(
      "placeholder",
      /群聊/,
    );
  });

  test("search filters assistants", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "websearch", withMockLLM: false });
    await apiCreateAgent(session.token, { name: "Alpha Finder" });
    await apiCreateAgent(session.token, { name: "Beta Finder" });
    await injectWebSession(page, session);
    await page.goto("/");
    await page.locator('input[aria-label="搜索助手或群聊"]').fill("Alpha");
    await expect(page.locator(".agent-list .agent-item", { hasText: "Alpha Finder" })).toBeVisible();
    await expect(page.locator(".agent-list .agent-item", { hasText: "Beta Finder" })).toHaveCount(0);
  });
});
