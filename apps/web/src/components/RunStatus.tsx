import { AgentAvatar } from "./AgentAvatar";
import type { BotPresenceStatus } from "./avatarColor";

type Props = {
  label: string;
  /** @deprecated Prefer shape/color/status via AgentAvatar props; kept for tip compat. */
  color?: string;
  agentId?: string;
  agentName?: string;
  shape?: string | null;
  avatarColor?: string | null;
  /** Product presence (five-only). Drives RobotHead via same map as list/header avatars. */
  status?: BotPresenceStatus | string;
  /** §8: working → searching animation when search-tool signal this turn. */
  searchingHint?: boolean;
};

/**
 * In-chat thinking / tool row — RobotHead synced to avatar presence
 * (not a separate spinner blob aesthetic).
 */
export function RunStatus({
  label,
  color,
  agentId,
  agentName = "助手",
  shape,
  avatarColor,
  status = "thinking",
  searchingHint = false,
}: Props) {
  return (
    <div className="chat-row chat-row-assistant run-status-row" aria-live="polite">
      <div className="run-status">
        <AgentAvatar
          id={agentId}
          name={agentName}
          size={28}
          shape={shape}
          color={avatarColor ?? color}
          status={status}
          searchingHint={searchingHint}
        />
        <span className="run-status-label">{label}</span>
      </div>
    </div>
  );
}
