/**
 * 列表页的轻量偏好：上次打开的助手 / 群聊，以及各分区的折叠状态。
 *
 * 用 SecureStore 而不是 AsyncStorage —— 这个项目里 token、onboarding 标记都走
 * SecureStore，偏好项跟着放一起，卸载即清空，不给「换人用同一台设备」留残留。
 *
 * 读失败一律回退到默认值：偏好读不出来最多是「不记得上次」，
 * 绝不该让首页白屏。
 */

import * as SecureStore from "expo-secure-store";

const LAST_KEY = "openbot_last_opened";
const COLLAPSE_KEY = "openbot_list_collapsed";

export type LastOpened = {
  kind: "agent" | "channel";
  id: string;
  name: string;
  conversation_id?: string;
};

async function readJson<T>(key: string, fallback: T): Promise<T> {
  try {
    const raw = await SecureStore.getItemAsync(key);
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as T;
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}

async function writeJson(key: string, value: unknown): Promise<void> {
  try {
    await SecureStore.setItemAsync(key, JSON.stringify(value));
  } catch {
    /* 存不进去只是下次不记得，不阻断流程 */
  }
}

export async function getLastOpened(): Promise<LastOpened | null> {
  const v = await readJson<LastOpened | null>(LAST_KEY, null);
  if (!v || (v.kind !== "agent" && v.kind !== "channel") || !v.id) return null;
  return v;
}

export async function setLastOpened(value: LastOpened): Promise<void> {
  await writeJson(LAST_KEY, value);
}

export async function getCollapsed(): Promise<Record<string, boolean>> {
  return readJson<Record<string, boolean>>(COLLAPSE_KEY, {});
}

export async function setCollapsed(map: Record<string, boolean>): Promise<void> {
  await writeJson(COLLAPSE_KEY, map);
}
