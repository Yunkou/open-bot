import { useCallback, useEffect, useState } from "react";
import {
  DEFAULT_API_BASE,
  formatApiHealthProbe,
  getApiBase,
  probeApiHealth,
  setApiBase,
  type ApiHealthProbeResult,
} from "../api";

type Props = {
  /** Compact for login card; full for settings. */
  variant?: "login" | "settings";
  /** Called after a successful apply (health ok + setApiBase). */
  onApplied?: (url: string) => void;
};

/**
 * Runtime API base editor + /healthz probe (P1-4).
 * Copy: switch server / pairing — same LAN or tunnel as the desktop API.
 */
export function ApiServerField({ variant = "login", onApplied }: Props) {
  const [draft, setDraft] = useState(() => getApiBase());
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<ApiHealthProbeResult | null>(null);

  useEffect(() => {
    setDraft(getApiBase());
  }, []);

  const runProbe = useCallback(async (url: string) => {
    setBusy(true);
    setProbe(null);
    try {
      const result = await probeApiHealth(url);
      setProbe(result);
      return result;
    } finally {
      setBusy(false);
    }
  }, []);

  const apply = useCallback(async () => {
    const next = draft.trim() || DEFAULT_API_BASE;
    const result = await runProbe(next);
    if (!result.ok) return;
    const applied = setApiBase(next);
    setDraft(applied);
    onApplied?.(applied);
  }, [draft, onApplied, runProbe]);

  const resetDefault = useCallback(() => {
    setDraft(DEFAULT_API_BASE);
    setProbe(null);
  }, []);

  const help =
    variant === "settings"
      ? "手机与电脑连同一台 open-bot 服务。改地址后先「检测连接」，通过再保存；切换服务器会保留本机登录态，若连不上请退出后重登。"
      : "填写电脑上的 API 地址以配对（局域网如 http://192.168.x.x:18080，或 ngrok https 隧道）。先检测连接，通过后再登录。";

  const statusClass =
    probe == null ? "" : probe.ok ? "api-server-status is-ok" : "api-server-status is-bad";

  return (
    <div className={`api-server-field api-server-field-${variant}`}>
      <label className="api-server-label">
        服务器地址
        <input
          type="url"
          inputMode="url"
          autoComplete="url"
          placeholder={DEFAULT_API_BASE}
          value={draft}
          disabled={busy}
          onChange={(e) => {
            setDraft(e.target.value);
            setProbe(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void apply();
            }
          }}
        />
      </label>
      <p className="muted small api-server-help">{help}</p>
      <div className="api-server-actions">
        <button type="button" className="settings-btn" disabled={busy} onClick={() => void runProbe(draft)}>
          {busy ? "检测中…" : "检测连接"}
        </button>
        <button type="button" className="primary" disabled={busy} onClick={() => void apply()}>
          {variant === "settings" ? "保存并使用" : "使用此服务器"}
        </button>
        {draft.trim().replace(/\/$/, "") !== DEFAULT_API_BASE ? (
          <button type="button" className="settings-btn" disabled={busy} onClick={resetDefault}>
            恢复默认
          </button>
        ) : null}
      </div>
      {probe ? (
        <p className={statusClass} role="status">
          {formatApiHealthProbe(probe)}
        </p>
      ) : (
        <p className="muted small api-server-current">当前：{getApiBase()}</p>
      )}
    </div>
  );
}
