import { test, expect } from "@playwright/test";
import { apiCreateAgent } from "../../helpers/api";
import { injectWebSession, registerFreshWebUser } from "../../helpers/auth";
import { closeSettings, openAgentByName, openSettings } from "../../helpers/web";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

test.describe("web settings & attachments", () => {
  test("settings tabs smoke", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "webset", withMockLLM: true });
    await injectWebSession(page, session);
    await page.goto("/");
    await openSettings(page, "LLM");
    await expect(page.locator(".settings-tabs button.active", { hasText: "LLM" })).toBeVisible();

    for (const tab of ["Skills", "MCP", "压缩", "Routines", "运行环境（高级）", "我的电脑"]) {
      const btn = page.locator(".settings-tabs button", { hasText: tab });
      await btn.click();
      await expect(btn).toHaveClass(/active/);
    }
    await closeSettings(page);
    await expect(page.locator(".modal h3", { hasText: "设置" })).toHaveCount(0);
  });

  test("attachment chip appears before send", async ({ page }) => {
    const { session } = await registerFreshWebUser({ prefix: "webatt", withMockLLM: true });
    const agent = await apiCreateAgent(session.token, { name: "Attach Bot" });
    await injectWebSession(page, session);
    await page.goto("/");
    await openAgentByName(page, agent.name);

    const fixture = path.join(__dirname, "../../fixtures/hello.txt");
    await page.locator("input.composer-file-input").setInputFiles(fixture);
    await expect(page.locator(".composer-chip", { hasText: "hello.txt" })).toBeVisible();
  });
});
