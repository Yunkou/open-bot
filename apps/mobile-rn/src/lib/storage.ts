/**
 * 存储分层。
 *
 * 分工是社区通行的做法，也和这个项目的数据性质对得上：
 *
 * | 数据            | 存储                | 理由                                          |
 * | --------------- | ------------------- | --------------------------------------------- |
 * | JWT             | `expo-secure-store` | Keychain / Keystore，不进明文存储，且不可备份 |
 * | 偏好（折叠、引导标记、上次打开、当前 Bot） | `react-native-mmkv` | 高频读写的小 JSON，MMKV 同步且比 AsyncStorage 快一个量级 |
 *
 * 早期版本把偏好也塞进了 SecureStore，那是对的 SecureStore 位置被浪费了：
 * 它每次读写都走系统加密存储，为「助手列表要不要折叠」这种数据付出
 * Keychain 往返的代价不划算，而且偏好数据本来也不需要加密。
 *
 * MMKV 是**同步** API，所以这一层的读取不用 await —— 调用点少一次微任务，
 * 状态也能在同一次 render 里拿到，不用来回闪一次。
 */

import { createMMKV } from "react-native-mmkv";

/** 偏好库单独一个 id，和将来的其它 MMKV 用途隔开，便于整体迁移或清空。 */
const prefs = createMMKV({ id: "openbot-prefs" });

/** 读取并反序列化。解析失败返回兜底值，不抛 —— 坏数据不该让设置页白屏。 */
export function readJson<T>(key: string, fallback: T): T {
  const raw = prefs.getString(key);
  if (raw == null) return fallback;
  try {
    const parsed = JSON.parse(raw) as T;
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}

export function writeJson(key: string, value: unknown): void {
  try {
    prefs.set(key, JSON.stringify(value));
  } catch {
    /* 写不进去最多是「偏好不生效」，不该把调用方一起拖崩 */
  }
}

export function removeKey(key: string): void {
  try {
    // v4 的删除方法叫 remove，不是 v2 的 delete
    prefs.remove(key);
  } catch {
    /* 同上 */
  }
}

/** 供需要直接操作偏好库的模块使用。 */
export { prefs as mmkvPrefs };
