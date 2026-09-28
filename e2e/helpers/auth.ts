import type { Page } from "@playwright/test";
import {
  apiEnsureMockLLM,
  apiLogin,
  apiRegister,
  type AuthSession,
} from "./api";
import {
  DEFAULT_USER_PASSWORD,
  adminCredentials,
  mockLLMEnabled,
  uniqueUsername,
} from "./env";

/** Inject web chat session into localStorage (via addInitScript before navigation). */
export async function injectWebSession(page: Page, session: AuthSession): Promise<void> {
  await page.addInitScript(
    ({ token, user }) => {
      localStorage.setItem("openbot_token", token);
      localStorage.setItem("openbot_user", JSON.stringify(user));
    },
    { token: session.token, user: session.user },
  );
}

export async function injectAdminSession(page: Page, session: AuthSession): Promise<void> {
  await page.addInitScript(
    ({ token, user }) => {
      localStorage.setItem("openbot_admin_token", token);
      localStorage.setItem("openbot_admin_user", JSON.stringify(user));
    },
    { token: session.token, user: session.user },
  );
}

export async function registerFreshWebUser(opts?: {
  prefix?: string;
  withMockLLM?: boolean;
}): Promise<{ session: AuthSession; username: string; password: string }> {
  const username = process.env.E2E_USER_USERNAME || uniqueUsername(opts?.prefix || "e2e");
  const password = process.env.E2E_USER_PASSWORD || DEFAULT_USER_PASSWORD;
  let session: AuthSession;
  if (process.env.E2E_USER_USERNAME && process.env.E2E_USER_PASSWORD) {
    session = await apiLogin(username, password);
  } else {
    session = await apiRegister(username, password);
  }
  const wantMock = opts?.withMockLLM ?? mockLLMEnabled();
  if (wantMock) {
    await apiEnsureMockLLM(session.token);
  }
  return { session, username, password };
}

export async function loginWebUI(page: Page, username: string, password: string): Promise<void> {
  await page.goto("/");
  await page.locator(".auth-tabs button", { hasText: "登录" }).click();
  await page.locator('input[autocomplete="username"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('button.primary[type="submit"]').click();
  await page.locator(".sidebar").waitFor({ state: "visible" });
}

export async function registerWebUI(page: Page, username: string, password: string): Promise<void> {
  await page.goto("/");
  await page.locator(".auth-tabs button", { hasText: "注册" }).click();
  await page.locator('input[autocomplete="username"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('button.primary[type="submit"]').click();
  await page.locator(".sidebar").waitFor({ state: "visible" });
}

export async function loginAdminUI(page: Page, username?: string, password?: string): Promise<void> {
  const creds = username && password ? { username, password } : adminCredentials();
  await page.goto("/login");
  await page.getByRole("textbox", { name: "用户名" }).fill(creds.username);
  await page.getByRole("textbox", { name: "密码" }).fill(creds.password);
  // Ant Design zh_CN inserts spaces in button labels: "登 录"
  await page.getByRole("button", { name: /登\s*录/ }).click();
  await page.waitForURL(/\/(users|bots|members|traces|llm|usage|flags|audit)/);
}

export async function adminLogoutUI(page: Page): Promise<void> {
  // ProLayout avatar dropdown (username next to user icon in header)
  const avatar = page.locator(".ant-pro-global-header .ant-dropdown-trigger").last();
  if (await avatar.count()) {
    await avatar.click();
  } else {
    await page.getByRole("img", { name: "user" }).last().click();
  }
  // Menu may render as menuitem or plain text in overlay
  const item = page.getByText(/退\s*出\s*登\s*录/).last();
  await item.click();
  await page.waitForURL(/\/login/);
}

export { adminCredentials, uniqueUsername, DEFAULT_USER_PASSWORD };
