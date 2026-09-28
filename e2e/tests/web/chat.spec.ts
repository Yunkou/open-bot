import { test, expect } from "@playwright/test";
import { apiCreateAgent } from "../../helpers/api";
import { injectWebSession, registerFreshWebUser } from "../../helpers/auth";
import {
  clickStop,
  openAgentByName,
  sendChatMessage,
  waitForAssistantText,
  waitForUserText,
} from "../../helpers/web";

test.describe("web chat", () => {
  test("send message and get assistant reply (mock LLM)", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "chatsend", withMockLLM: true });
    const agent = await apiCreateAgent(session.token, { name: "E2E Chat Bot" });
    await injectWebSession(page, session);
    await page.goto("/");
    await openAgentByName(page, agent.name);

    const msg = "[short] hello e2e";
    await sendChatMessage(page, msg);
    await waitForUserText(page, "hello e2e");
    await waitForAssistantText(page, "E2E_MOCK_OK");
  });

  test("stop button cancels in-flight generation", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "chatstop", withMockLLM: true });
    const agent = await apiCreateAgent(session.token, { name: "E2E Stop Bot" });
    await injectWebSession(page, session);
    await page.goto("/");
    await openAgentByName(page, agent.name);

    await sendChatMessage(page, "[long] please stream forever-ish");
    await clickStop(page);

    // Stop control goes away; optional partial assistant text or stopped marker
    await expect(page.locator('button.composer-stop[aria-label="停止生成"]')).toHaveCount(0, {
      timeout: 15_000,
    });
    await expect(page.locator(".composer-send:not(.composer-stop)")).toBeVisible();
  });

  test("interrupt-and-send keeps partial then starts next turn", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "chatint", withMockLLM: true });
    const agent = await apiCreateAgent(session.token, { name: "E2E Interrupt Bot" });
    await injectWebSession(page, session);
    await page.goto("/");
    await openAgentByName(page, agent.name);

    await sendChatMessage(page, "[long] first turn streaming");
    // Wait until some assistant tokens appear
    await expect(page.locator(".bubble-assistant").first()).toBeVisible({ timeout: 30_000 });

    // Typing + send while streaming should interrupt and keep partial
    await sendChatMessage(page, "[short] second turn");
    await waitForUserText(page, "second turn");
    await waitForAssistantText(page, "E2E_MOCK_OK");
  });

  test("refresh while run_active resumes stream", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "chatresume", withMockLLM: true });
    const agent = await apiCreateAgent(session.token, { name: "E2E Resume Bot" });
    await injectWebSession(page, session);
    await page.goto("/");
    await openAgentByName(page, agent.name);

    await sendChatMessage(page, "[long] resume after reload");
    await expect(page.locator(".bubble-assistant").first()).toBeVisible({ timeout: 30_000 });
    // Or run status row while streaming
    const streaming = page.locator(".bubble-assistant, .run-status-row").first();
    await expect(streaming).toBeVisible();

    await page.reload();
    await openAgentByName(page, agent.name);
    // After resume, either still streaming or completed mock long reply
    await expect(
      page.locator(".bubble-assistant, .run-status-row, button.composer-stop").first(),
    ).toBeVisible({ timeout: 30_000 });
    await waitForAssistantText(page, "E2E_MOCK", 90_000);
  });

  test("switch bot without stopping other run", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "chatswitch", withMockLLM: true });
    const a = await apiCreateAgent(session.token, { name: "E2E Bot A" });
    const b = await apiCreateAgent(session.token, { name: "E2E Bot B" });
    await injectWebSession(page, session);
    await page.goto("/");

    await openAgentByName(page, a.name);
    await sendChatMessage(page, "[long] run on A while switching");
    await expect(page.locator(".bubble-assistant, .run-status-row").first()).toBeVisible({
      timeout: 30_000,
    });

    await openAgentByName(page, b.name);
    // Bot A should still show busy indicator in sidebar
    const aItem = page.locator(".agent-list .agent-item", { hasText: a.name });
    await expect(aItem.locator(".run-busy-dot")).toBeVisible({ timeout: 15_000 });

    // Composer on B should be idle (send, not stop) unless we also started B
    await expect(page.locator(".composer-send:not(.composer-stop)")).toBeVisible();
  });
});
