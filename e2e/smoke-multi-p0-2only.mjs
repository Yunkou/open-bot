import { chromium } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { config as loadDotenv } from "dotenv";
const root = "/Users/tangxin/Workprojects/open-bot";
loadDotenv({ path: path.join(root, "e2e/.env.e2e"), override: true });
loadDotenv({ path: path.join(root, ".env"), override: false });
const WEB = "http://127.0.0.1:5173";
const OUT = path.join(root, ".merge-packs/smoke-shots-multi-p0");
const user = process.env.E2E_ADMIN_USERNAME || process.env.BOOTSTRAP_ADMIN_USERNAME;
const pass = process.env.E2E_ADMIN_PASSWORD || process.env.BOOTSTRAP_ADMIN_PASSWORD;

async function login(page) {
  await page.goto(WEB, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(800);
  const pwd = page.locator('input[type="password"]').first();
  if (await pwd.count()) {
    await page.locator("input").nth(0).fill(user || "");
    await pwd.fill(pass || "");
    await page.locator('button[type="submit"]').filter({ hasText: /登/ }).first().click();
    await page.waitForTimeout(2000);
  }
}
async function forceCoarse(page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (q) => {
      if (String(q).includes("pointer: coarse")) {
        return { matches: true, media: q, onchange: null, addListener(){}, removeListener(){}, addEventListener(){}, removeEventListener(){}, dispatchEvent(){return false;} };
      }
      return orig(q);
    };
  });
}
async function longPressPointer(page, locator) {
  const handle = await locator.elementHandle();
  await page.evaluate(async (el) => {
    const r = el.getBoundingClientRect();
    const x = r.left + Math.min(48, r.width / 2);
    const y = r.top + r.height / 2;
    const opts = { bubbles: true, cancelable: true, pointerType: "touch", clientX: x, clientY: y, pointerId: 1 };
    el.dispatchEvent(new PointerEvent("pointerdown", opts));
    await new Promise((res) => setTimeout(res, 550));
    el.dispatchEvent(new PointerEvent("pointerup", opts));
  }, handle);
  await page.waitForTimeout(800); // past 500ms swallow window
}

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1100, height: 800 }, hasTouch: true });
const page = await context.newPage();
await forceCoarse(page);
await login(page);
await page.waitForTimeout(800);
const row = page.locator(".agent-item.conv-item").first();
console.log("agents", await page.locator(".agent-item.conv-item").count());
await longPressPointer(page, row);
console.log("sheet", await page.getByText("删除会话").isVisible());
await page.screenshot({ path: path.join(OUT, "p0-2b-sheet.png") });
// click via role within sheet
await page.locator(".conv-action-sheet, .action-sheet, [class*='ActionSheet'], [class*='conv-action']").getByText("删除会话").click({ timeout: 3000 }).catch(async () => {
  await page.getByRole("button", { name: "删除会话" }).click();
});
await page.waitForTimeout(800);
const texts = await page.evaluate(() => Array.from(document.querySelectorAll("[role='alertdialog'], [data-radix-alert-dialog-content], .alert-dialog, [class*='AlertDialog']")).map(el => el.textContent));
console.log("dialogs", texts);
const hasConfirm = await page.getByText("删除后无法恢复").isVisible().catch(()=>false)
  || await page.getByText(/确定删除助手/).isVisible().catch(()=>false);
console.log("confirm", hasConfirm);
await page.screenshot({ path: path.join(OUT, "p0-2b-confirm.png") });
if (hasConfirm) {
  await page.getByRole("button", { name: "取消" }).last().click();
}
console.log(hasConfirm ? "PASS" : "FAIL");
await browser.close();
