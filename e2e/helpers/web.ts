import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";

export async function openAgentByName(page: Page, name: string): Promise<void> {
  const item = page.locator(".agent-list .agent-item", { hasText: name }).first();
  await expect(item).toBeVisible({ timeout: 15_000 });
  await item.click();
  await expect(page.locator(".composer textarea")).toBeVisible();
}

export async function sendChatMessage(page: Page, text: string): Promise<void> {
  const ta = page.locator(".composer textarea");
  await ta.fill(text);
  await page.locator(".composer-send:not(.composer-stop)").click();
}

export async function waitForAssistantText(page: Page, substring: string, timeout = 60_000): Promise<void> {
  await expect(page.locator(".bubble-assistant").filter({ hasText: substring }).first()).toBeVisible({
    timeout,
  });
}

export async function waitForUserText(page: Page, substring: string): Promise<void> {
  await expect(page.locator(".bubble-user").filter({ hasText: substring }).first()).toBeVisible();
}

export async function clickStop(page: Page): Promise<void> {
  const stop = page.locator('button.composer-stop[aria-label="停止生成"]');
  await expect(stop).toBeVisible({ timeout: 30_000 });
  await stop.click();
}

export async function openSettings(page: Page, tab?: string): Promise<void> {
  await page.locator("button.settings-btn", { hasText: "设置" }).click();
  await expect(page.locator(".modal h3", { hasText: "设置" })).toBeVisible();
  if (tab) {
    await page.locator(".settings-tabs button", { hasText: tab }).click();
  }
}

export async function closeSettings(page: Page): Promise<void> {
  await page.locator(".modal-head button.ghost", { hasText: "关闭" }).click();
}

export async function createBotViaUI(page: Page, name: string): Promise<void> {
  await page.locator('button.icon-btn[title="新建聊天"]').click();
  await expect(page.locator(".new-chat-action", { hasText: "创建新 Bot" })).toBeVisible();
  await page.locator(".new-chat-action", { hasText: "创建新 Bot" }).click();
  await page.locator('input[placeholder="Bot 名称（必填）"]').fill(name);
  await page.locator("button.new-chat-group-submit", { hasText: "创建并开始聊天" }).click();
  await expect(page.locator(".agent-list .agent-item", { hasText: name }).first()).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.locator(".composer textarea")).toBeVisible({ timeout: 15_000 });
}

export async function createGroupViaUI(page: Page, name: string, memberNames: string[]): Promise<void> {
  await page.locator('button.icon-btn[title="新建聊天"]').click();
  await page.locator(".new-chat-action", { hasText: "创建群聊" }).click();
  await page.locator('input[placeholder="群聊名称"]').fill(name);
  for (const m of memberNames) {
    await page.getByRole("checkbox", { name: m }).check();
  }
  await page.locator("button.new-chat-group-submit", { hasText: "创建群聊" }).click();
  await expect(page.locator(".channel-list .channel-item", { hasText: name }).first()).toBeVisible({
    timeout: 15_000,
  });
}
