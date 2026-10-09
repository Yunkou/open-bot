import { chromium } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { config as loadDotenv } from "dotenv";

const root = "/Users/tangxin/Workprojects/open-bot";
loadDotenv({ path: path.join(root, "e2e/.env.e2e"), override: true });
loadDotenv({ path: path.join(root, ".env"), override: false });
const user = process.env.E2E_ADMIN_USERNAME || process.env.BOOTSTRAP_ADMIN_USERNAME;
const pass = process.env.E2E_ADMIN_PASSWORD || process.env.BOOTSTRAP_ADMIN_PASSWORD;
const ADMIN = "http://127.0.0.1:5174";
const WEB = "http://127.0.0.1:5173";
const API = "http://127.0.0.1:18080";
const OUT = path.join(root, ".merge-packs/smoke-shots");
fs.mkdirSync(OUT, { recursive: true });
const results = [];
const rec = (id, status, detail, shot, extra = {}) => {
  results.push({ id, status, detail, shot, ...extra });
  console.log(`${id} ${status}: ${detail}`);
};

async function loginAdmin(page) {
  await page.goto(ADMIN + "/login", { waitUntil: "domcontentloaded" });
  await page.getByRole("textbox", { name: "用户名" }).fill(user);
  await page.getByRole("textbox", { name: "密码" }).fill(pass);
  await page.getByRole("button", { name: /登\s*录/ }).click();
  await page.waitForURL(/\/(users|bots|members|traces|llm|usage|flags|audit|skills)/, { timeout: 30000 });
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: "zh-CN" });
  const page = await context.newPage();
  page.setDefaultNavigationTimeout(60000);

  await loginAdmin(page);
  await page.goto(ADMIN + "/bots", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(600);
  const topLevelSkills = await page.locator(".ant-menu-item").filter({ hasText: /^技能$/ }).count();
  const nestedSkills = await page.locator(".ant-menu-submenu").filter({ hasText: /^Bot$/ }).locator(".ant-menu-item").filter({ hasText: /技能/ }).count();
  const shot1 = path.join(OUT, "admin-skills-pm1-sidebar.png");
  await page.screenshot({ path: shot1 });
  rec("PM1", topLevelSkills >= 1 && nestedSkills === 0 ? "PASS" : "FAIL", `topLevel=${topLevelSkills} nested=${nestedSkills}`, shot1);

  await page.goto(ADMIN + "/skills", { waitUntil: "domcontentloaded" });
  await page.waitForSelector(".ant-table-row", { timeout: 20000 });
  const tableRows = await page.locator(".ant-table-row").count();
  const hasSource = await page.getByText(/内置|自建/).count();
  const shot2a = path.join(OUT, "admin-skills-pm2-list.png");
  await page.screenshot({ path: shot2a });

  const link = page.locator(".ant-table-tbody a").first();
  await link.click();
  await page.waitForURL(/\/skills\/.+/, { timeout: 15000 });
  await page.waitForSelector(".skill-editor, .skill-tree-row, .skill-textarea, .skill-cm, .cm-editor", { timeout: 20000 });
  try {
    await page.waitForSelector(".cm-editor, .skill-cm", { timeout: 8000 });
  } catch {}
  await page.waitForTimeout(500);
  const url = page.url();
  const hasTree = await page.locator(".skill-tree-row, .skill-editor .skill-tree-name").count();
  const hasSkillMd = await page.getByText("SKILL.md").count();
  const hasCM = await page.locator(".cm-editor, .skill-cm").count();
  const hasTextarea = await page.locator("textarea.skill-textarea").count();
  const hasEditorShell = await page.locator(".skill-editor").count();
  const noOldModal = (await page.locator(".ant-modal").filter({ hasText: /编辑技能/ }).count()) === 0;
  const shot2b = path.join(OUT, "admin-skills-pm2-editor.png");
  await page.screenshot({ path: shot2b });
  const pm2 = /\/skills\//.test(url) && hasEditorShell > 0 && (hasTree > 0 || hasSkillMd > 0) && (hasCM > 0 || hasTextarea > 0) && noOldModal && tableRows > 0;
  rec("PM2", pm2 ? "PASS" : "FAIL", `rows=${tableRows} sourcePills=${hasSource} url=${url} tree=${hasTree} skillMd=${hasSkillMd} cm=${hasCM} ta=${hasTextarea} shell=${hasEditorShell}`, shot2b, { shot_list: shot2a });

  await page.goto(ADMIN + "/bots", { waitUntil: "domcontentloaded" });
  await page.waitForSelector(".ant-table-row", { timeout: 20000 });
  await page.locator(".ant-table-row").first().getByText("编辑").click();
  await page.waitForSelector(".ant-modal", { timeout: 15000 });
  try {
    await page.waitForSelector(".ant-modal .ant-checkbox", { timeout: 15000 });
  } catch {}
  await page.waitForTimeout(800);
  const modal = page.locator(".ant-modal").filter({ hasText: /启用的技能|编辑/ }).last();
  const hasEnableLabel = await modal.getByText("启用的技能").count();
  const checkboxes = await modal.locator(".ant-checkbox").count();
  const skillLinks = await modal.locator('a[href*="/skills/"]').count();
  const cmInModal = await modal.locator(".cm-editor, textarea.skill-textarea").count();
  const shot3 = path.join(OUT, "admin-skills-pm3-bot-edit.png");
  await page.screenshot({ path: shot3 });
  const pm3 = hasEnableLabel > 0 && checkboxes > 0 && skillLinks > 0 && cmInModal === 0;
  rec("PM3", pm3 ? "PASS" : "FAIL", `label=${hasEnableLabel} cbs=${checkboxes} links=${skillLinks} bodyEditor=${cmInModal}`, shot3);
  await page.keyboard.press("Escape");
  await page.waitForTimeout(300);

  const webLogin = await fetch(`${API}/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: user, password: pass }),
  });
  if (!webLogin.ok) throw new Error("web login failed " + webLogin.status + " " + await webLogin.text());
  const sess = await webLogin.json();
  const webPage = await context.newPage();
  await webPage.addInitScript(({ token, u }) => {
    localStorage.setItem("openbot_token", token);
    localStorage.setItem("openbot_user", JSON.stringify(u));
  }, { token: sess.token, u: sess.user });
  await webPage.goto(WEB + "/", { waitUntil: "domcontentloaded" });
  await webPage.waitForSelector(".sidebar", { timeout: 30000 });
  const settingsCandidates = [
    webPage.locator('button[title="设置"]'),
    webPage.locator('button[aria-label="设置"]'),
    webPage.getByRole("button", { name: /设置/ }),
  ];
  let opened = false;
  for (const c of settingsCandidates) {
    if (await c.count()) {
      await c.first().click();
      opened = true;
      break;
    }
  }
  if (!opened) {
    const titles = await webPage.locator("button").evaluateAll((els) =>
      els.map((e) => ({ title: e.getAttribute("title"), aria: e.getAttribute("aria-label"), text: (e.textContent || "").slice(0, 40) }))
        .filter((x) => (x.title || "").includes("设置") || (x.aria || "").includes("设置") || (x.text || "").includes("设置"))
    );
    console.log("settings candidates", titles);
    // try clicking any button with gear title containing Settings-like
    const all = await webPage.locator("button[title]").evaluateAll((els) => els.map((e) => e.getAttribute("title")));
    console.log("titled buttons", all);
    throw new Error("no settings button");
  }
  await webPage.waitForSelector(".settings-dialog, .modal.settings-dialog, .settings-nav", { timeout: 15000 });
  const skillsNav = webPage.locator(".settings-nav button, .settings-nav a").filter({ hasText: /^技能$/ });
  if (await skillsNav.count()) await skillsNav.first().click();
  else await webPage.locator(".settings-nav").getByText("技能").click();
  await webPage.waitForTimeout(500);
  const entryVisible = await webPage.getByText("打开管理端 · 技能").isVisible().catch(() => false);
  const fullEditor = await webPage.locator(".cm-editor, .skill-editor, .skill-tree-row, textarea.skill-textarea").count();
  const shot4 = path.join(OUT, "admin-skills-pm4-web-entry.png");
  await webPage.screenshot({ path: shot4 });
  rec("PM4", entryVisible && fullEditor === 0 ? "PASS" : "FAIL", `entry=${entryVisible} fullEditor=${fullEditor}`, shot4);

  await browser.close();
  const summary = { at: new Date().toISOString(), results, allPass: results.every((r) => r.status === "PASS") };
  fs.writeFileSync(path.join(OUT, "admin-skills-smoke-summary.json"), JSON.stringify(summary, null, 2));
  console.log(JSON.stringify(summary, null, 2));
  process.exit(summary.allPass ? 0 : 2);
}
main().catch((e) => { console.error(e); process.exit(1); });
