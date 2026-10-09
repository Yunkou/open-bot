import { chromium } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { config as loadDotenv } from "dotenv";

const root = "/Users/tangxin/Workprojects/open-bot";
loadDotenv({ path: path.join(root, "e2e/.env.e2e"), override: true });
loadDotenv({ path: path.join(root, ".env"), override: false });
const WEB = "http://127.0.0.1:5173";
const OUT = path.join(root, ".merge-packs/smoke-shots-multi-p0");
const user = process.env.E2E_ADMIN_USERNAME || process.env.BOOTSTRAP_ADMIN_USERNAME || process.env.E2E_USERNAME;
const pass = process.env.E2E_ADMIN_PASSWORD || process.env.BOOTSTRAP_ADMIN_PASSWORD || process.env.E2E_PASSWORD;
fs.mkdirSync(OUT, { recursive: true });
const results = [];
const rec = (id, status, detail) => { results.push({ id, status, detail }); console.log(`${id} ${status}: ${detail}`); };

async function ensureLoggedIn(page) {
  await page.goto(WEB, { waitUntil: "domcontentloaded", timeout: 45000 });
  await page.waitForTimeout(800);
  const pwd = page.locator('input[type="password"]').first();
  if (await pwd.count()) {
    await page.locator("input").nth(0).fill(user || "");
    await pwd.fill(pass || "");
    await page.locator('button[type="submit"]').filter({ hasText: /登/ }).first().click();
    await page.waitForTimeout(2500);
  }
}

async function forceCoarse(page) {
  await page.addInitScript(() => {
    const orig = window.matchMedia.bind(window);
    window.matchMedia = (q) => {
      const qs = String(q);
      if (qs.includes("pointer: coarse")) {
        return {
          matches: true, media: qs, onchange: null,
          addListener() {}, removeListener() {},
          addEventListener() {}, removeEventListener() {},
          dispatchEvent() { return false; },
        };
      }
      return orig(q);
    };
  });
}

async function longPressPointer(page, locator) {
  const handle = await locator.elementHandle();
  if (!handle) return false;
  await page.evaluate(async (el) => {
    const r = el.getBoundingClientRect();
    const x = r.left + Math.min(48, r.width / 2);
    const y = r.top + r.height / 2;
    const opts = { bubbles: true, cancelable: true, pointerType: "touch", clientX: x, clientY: y, pointerId: 1 };
    el.dispatchEvent(new PointerEvent("pointerdown", opts));
    await new Promise((res) => setTimeout(res, 550));
    el.dispatchEvent(new PointerEvent("pointerup", opts));
  }, handle);
  await page.waitForTimeout(400);
  return true;
}

const browser = await chromium.launch({ headless: true });

// Shared: create one bot if needed (narrow sheet path = p0-3)
{
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
  const page = await context.newPage();
  await ensureLoggedIn(page);
  await page.waitForTimeout(800);
  await page.getByTitle("新建聊天").click();
  await page.waitForTimeout(400);
  await page.locator(".new-chat-row, button, [role='button']").filter({ hasText: "创建新 Bot" }).first().click({ force: true });
  await page.waitForTimeout(700);
  const prefer = await page.getByText("优先电脑").isVisible().catch(() => false);
  const sticky = await page.getByRole("button", { name: "创建并开始聊天" }).isVisible().catch(() => false);
  const sheetMeta = await page.evaluate(() => {
    const el = document.querySelector(".new-chat-popover, .new-chat-sheet, .sheet-form") || document.querySelector("[class*='new-chat']");
    if (!el) return null;
    const s = getComputedStyle(el);
    const g = el.querySelector(".new-chat-shape-grid, .shape-grid, [class*='shape-grid']");
    const cols = g ? getComputedStyle(g).gridTemplateColumns.trim().split(/\s+/).length : -1;
    return {
      width: s.width, maxHeight: s.maxHeight, position: s.position, bottom: s.bottom,
      className: el.className, cols,
      isSheet: el.classList.contains("sheet") || el.classList.contains("new-chat-sheet") || /sheet/i.test(el.className) || s.position === "fixed",
    };
  });
  await page.screenshot({ path: path.join(OUT, "p0-3-create-bot.png") });
  const colsOk = !sheetMeta || sheetMeta.cols === -1 || sheetMeta.cols <= 4;
  rec("p0-3", sticky && prefer && colsOk ? "PASS" : "FAIL", `prefer=${prefer} sticky=${sticky} meta=${JSON.stringify(sheetMeta)}`);

  // fill name and create so later delete works
  const nameInput = page.locator('input[placeholder*="名称"], input[name="name"], .new-chat-popover input').first();
  if (await nameInput.count()) {
    await nameInput.fill(`P0触控冒烟${Date.now() % 10000}`);
  }
  if (sticky) {
    await page.getByRole("button", { name: "创建并开始聊天" }).click();
    await page.waitForTimeout(2500);
  }
  await page.screenshot({ path: path.join(OUT, "p0-3-after-create.png") });
  await context.close();
}

// p0-1 wide + coarse
{
  const context = await browser.newContext({ viewport: { width: 1100, height: 800 }, hasTouch: true });
  const page = await context.newPage();
  await forceCoarse(page);
  await ensureLoggedIn(page);
  await page.waitForTimeout(1000);
  const hasTouchClass = await page.evaluate(() => document.documentElement.classList.contains("touch-ui"));
  // open an agent chat if any
  const agent = page.locator(".agent-item.conv-item").first();
  if (await agent.count()) await agent.click();
  await page.waitForTimeout(800);
  const bubble = page.locator(".msg .md, .markdown-body, .msg-body, .chat-msg").first();
  if (await bubble.count()) await longPressPointer(page, bubble);
  const sheet1 = await page.getByText("复制").first().isVisible().catch(() => false)
    || await page.locator(".msg-action-sheet").first().isVisible().catch(() => false);
  const hoverHidden = await page.evaluate(() => {
    const el = document.querySelector(".msg-hover-bar");
    if (!el) return true;
    return getComputedStyle(el).display === "none";
  });
  await page.screenshot({ path: path.join(OUT, "p0-1-wide-touch.png") });
  rec("p0-1", hasTouchClass && hoverHidden ? "PASS" : "FAIL", `touch-ui=${hasTouchClass} sheet=${sheet1} hoverHidden=${hoverHidden}`);
  await context.close();
}

// p0-2 delete sheet via long-press agent row
{
  const context = await browser.newContext({ viewport: { width: 1100, height: 800 }, hasTouch: true });
  const page = await context.newPage();
  await forceCoarse(page);
  await ensureLoggedIn(page);
  await page.waitForTimeout(1000);
  // if no agents, create one quickly on this wide layout
  let agents = await page.locator(".agent-item.conv-item").count();
  if (agents === 0) {
    await page.getByTitle("新建聊天").click();
    await page.waitForTimeout(300);
    await page.locator("button, [role='button'], .new-chat-row").filter({ hasText: "创建新 Bot" }).first().click({ force: true });
    await page.waitForTimeout(500);
    const nameInput = page.locator(".new-chat-popover input, input").first();
    if (await nameInput.count()) await nameInput.fill(`P0删测${Date.now() % 1000}`);
    await page.getByRole("button", { name: "创建并开始聊天" }).click();
    await page.waitForTimeout(2500);
    agents = await page.locator(".agent-item.conv-item").count();
  }
  const row = page.locator(".agent-item.conv-item").first();
  await longPressPointer(page, row);
  const sheet2 = await page.getByText("删除会话").isVisible().catch(() => false);
  await page.screenshot({ path: path.join(OUT, "p0-2-conv-delete.png") });
  let confirm = false;
  if (sheet2) {
    await page.getByText("删除会话").click();
    await page.waitForTimeout(500);
    confirm = await page.getByText("删除后无法恢复").isVisible().catch(() => false);
    await page.screenshot({ path: path.join(OUT, "p0-2-confirm.png") });
    const cancel = page.getByRole("button", { name: "取消" });
    if (await cancel.count()) await cancel.first().click();
  }
  rec("p0-2", sheet2 && confirm ? "PASS" : "FAIL", `agents=${agents} sheet=${sheet2} confirm=${confirm}`);
  await context.close();
}

fs.writeFileSync(path.join(OUT, "summary.json"), JSON.stringify(results, null, 2));
console.log("SUMMARY\n" + JSON.stringify(results, null, 2));
await browser.close();
