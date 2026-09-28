export function webURL(): string {
  return process.env.E2E_WEB_URL || "http://127.0.0.1:5173";
}

export function adminURL(): string {
  return process.env.E2E_ADMIN_URL || "http://127.0.0.1:5174";
}

export function apiURL(): string {
  return process.env.E2E_API_URL || "http://127.0.0.1:18080";
}

export function mockLLMEnabled(): boolean {
  return (process.env.E2E_MOCK_LLM || "1") !== "0";
}

export function mockLLMURL(): string {
  return process.env.E2E_MOCK_LLM_URL || "http://127.0.0.1:18099/v1";
}

export function adminCredentials(): { username: string; password: string } {
  const username = process.env.E2E_ADMIN_USERNAME || "";
  const password = process.env.E2E_ADMIN_PASSWORD || "";
  if (!username || !password) {
    throw new Error(
      "Missing E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD (or BOOTSTRAP_ADMIN_* in repo .env). See e2e/.env.e2e.example",
    );
  }
  return { username, password };
}

export function uniqueUsername(prefix = "e2e"): string {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`;
}

export const DEFAULT_USER_PASSWORD = "e2e-pass-1234";
