import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";

/** PageContainer title (avoids sidebar menu duplicate text). */
export async function expectAdminPageTitle(page: Page, title: string): Promise<void> {
  await expect(page.getByTitle(title)).toBeVisible({ timeout: 15_000 });
}
