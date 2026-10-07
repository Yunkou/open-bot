import type { Agent } from "../api";
import { AgentAvatar } from "./AgentAvatar";

export type SettingsHubNav =
  | "account"
  | "mcp"
  | "bot"
  | "machines"
  | "llm"
  | "skills"
  | "sandbox"
  | "compact"
  | "routines"
  | "secrets"
  | "general";

type Props = {
  username: string;
  email?: string;
  accountInitials: string;
  focusBot?: Agent | null;
  preferredMachineLabel?: string;
  onClose: () => void;
  onNavigate: (tab: SettingsHubNav) => void;
  onOpenBotAvatar?: () => void;
  onLogout: () => void;
};

function Chevron() {
  return (
    <svg className="settings-hub-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M9 6l6 6-6 6" />
    </svg>
  );
}

/**
 * Grok-style grouped settings hub for touch-ui / layout-narrow (C-2 → #7 v2).
 * Fullscreen gray+white cards; drill-downs use existing settings tabs / sheets.
 */
export function MobileSettingsHub({
  username,
  email,
  accountInitials,
  focusBot,
  preferredMachineLabel,
  onClose,
  onNavigate,
  onOpenBotAvatar,
  onLogout,
}: Props) {
  return (
    <div className="settings-hub settings-hub-v2">
      <div className="settings-hub-top">
        <h2 className="settings-hub-title">设置</h2>
        <button type="button" className="settings-hub-close" aria-label="关闭" onClick={onClose}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
            <path d="M6 6l12 12M18 6 6 18" />
          </svg>
        </button>
      </div>

      <div className="settings-hub-scroll">
        <section className="settings-hub-card">
          <button type="button" className="settings-hub-row settings-hub-row-account" onClick={() => onNavigate("account")}>
            <span className="settings-hub-avatar" aria-hidden>
              {accountInitials}
            </span>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">{username || "账户"}</span>
              {email ? <span className="settings-hub-row-sub">{email}</span> : null}
            </span>
            <Chevron />
          </button>
        </section>

        <section className="settings-hub-card">
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("mcp")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">插件</span>
              <span className="settings-hub-row-sub">扩展能力与连接</span>
            </span>
            <Chevron />
          </button>
        </section>

        <div className="settings-hub-section-label">Bot</div>
        <section className="settings-hub-card">
          <button
            type="button"
            className="settings-hub-row"
            onClick={() => {
              if (onOpenBotAvatar && focusBot) onOpenBotAvatar();
              else onNavigate("bot");
            }}
          >
            {focusBot ? (
              <AgentAvatar
                id={focusBot.id}
                name={focusBot.name}
                size={36}
                shape={focusBot.avatar_shape}
                color={focusBot.avatar_color}
                online={focusBot.online === true}
              />
            ) : (
              <span className="settings-hub-avatar settings-hub-avatar-sm" aria-hidden>
                ?
              </span>
            )}
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">当前 Bot / 形象</span>
              <span className="settings-hub-row-sub">{focusBot?.name || "未选择"}</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("machines")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">优先电脑</span>
            </span>
            <span className="settings-hub-row-value">{preferredMachineLabel || "未指定"}</span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("bot")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">Bot 设置</span>
              <span className="settings-hub-row-sub">岗位描述与本 Bot 能力</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("general")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">审核与时区</span>
            </span>
            <Chevron />
          </button>
        </section>

        <div className="settings-hub-section-label">通用</div>
        <section className="settings-hub-card">
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("llm")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">模型</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("skills")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">扩展能力包</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("sandbox")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">运行环境</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("routines")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">例行任务</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("compact")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">数据与压缩</span>
            </span>
            <Chevron />
          </button>
          <button type="button" className="settings-hub-row" onClick={() => onNavigate("secrets")}>
            <span className="settings-hub-row-main">
              <span className="settings-hub-row-title">密钥</span>
            </span>
            <Chevron />
          </button>
        </section>

        <section className="settings-hub-card">
          <button type="button" className="settings-hub-row settings-hub-row-logout" onClick={onLogout}>
            退出登录
          </button>
        </section>
      </div>
    </div>
  );
}
