import { defineConfig, devices } from "@playwright/test";
import { config as loadDotenv } from "dotenv";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");

// Load e2e/.env.e2e then optionally map BOOTSTRAP_* from repo .env (no secrets committed).
loadDotenv({ path: path.join(__dirname, ".env.e2e"), override: true });
const rootEnvPath = path.join(root, ".env");
if (fs.existsSync(rootEnvPath)) {
  loadDotenv({ path: rootEnvPath, override: false });
}
if (!process.env.E2E_ADMIN_USERNAME && process.env.BOOTSTRAP_ADMIN_USERNAME) {
  process.env.E2E_ADMIN_USERNAME = process.env.BOOTSTRAP_ADMIN_USERNAME;
}
if (!process.env.E2E_ADMIN_PASSWORD && process.env.BOOTSTRAP_ADMIN_PASSWORD) {
  process.env.E2E_ADMIN_PASSWORD = process.env.BOOTSTRAP_ADMIN_PASSWORD;
}

const WEB_URL = process.env.E2E_WEB_URL || "http://127.0.0.1:5173";
const ADMIN_URL = process.env.E2E_ADMIN_URL || "http://127.0.0.1:5174";
const API_URL = process.env.E2E_API_URL || "http://127.0.0.1:18080";
const MOCK_LLM = (process.env.E2E_MOCK_LLM || "1") !== "0";
const MOCK_PORT = process.env.E2E_MOCK_LLM_PORT || "18099";

process.env.E2E_WEB_URL = WEB_URL;
process.env.E2E_ADMIN_URL = ADMIN_URL;
process.env.E2E_API_URL = API_URL;
process.env.E2E_MOCK_LLM_URL = process.env.E2E_MOCK_LLM_URL || `http://127.0.0.1:${MOCK_PORT}/v1`;

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 20_000 },
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]],
  outputDir: "test-results",
  globalSetup: MOCK_LLM ? "./helpers/global-setup.ts" : undefined,
  globalTeardown: MOCK_LLM ? "./helpers/global-teardown.ts" : undefined,
  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "off",
    locale: "zh-CN",
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "web",
      testDir: "./tests/web",
      use: { ...devices["Desktop Chrome"], baseURL: WEB_URL },
    },
    {
      name: "admin",
      testDir: "./tests/admin",
      use: { ...devices["Desktop Chrome"], baseURL: ADMIN_URL },
    },
  ],
});
