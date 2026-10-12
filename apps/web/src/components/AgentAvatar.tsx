import { RobotHead } from "robot-heads";
import {
  avatarSeed01,
  deriveScreenColor,
  deriveTrimColor,
  normalizePresenceStatus,
  presenceToRobotState,
  resolveAvatarColor,
  resolveAvatarShape,
  type BotPresenceStatus,
} from "./avatarColor";

type Props = {
  id?: string;
  name: string;
  size?: number;
  className?: string;
  shape?: string | null;
  color?: string | null;
  status?: BotPresenceStatus | string;
  /** Green online dot (bottom-right): server-computed bot online, independent of status face. */
  online?: boolean;
  onClick?: () => void;
  title?: string;
};

/** Unknown status → working (product rule); empty → idle. */
function normalizeStatus(status?: string | null): BotPresenceStatus {
  return normalizePresenceStatus(status);
}

export function AgentAvatar({
  id,
  name,
  size = 28,
  className = "",
  shape,
  color,
  status = "idle",
  online = false,
  onClick,
  title,
}: Props) {
  const seedKey = id || name;
  const bg = resolveAvatarColor(seedKey, color);
  const sh = resolveAvatarShape(seedKey, shape);
  const st = normalizeStatus(status);
  const robotState = presenceToRobotState(status);
  const clickable = Boolean(onClick);
  const seed = avatarSeed01(seedKey);
  const small = size <= 32;
  const trimColor = deriveTrimColor(bg);
  const screenColor = deriveScreenColor(bg);
  const dotPx = Math.max(6, Math.min(8, Math.round(size * 0.22)));

  const face = (
    <>
      <RobotHead
        shape={sh}
        state={robotState}
        size={size}
        color={bg}
        trimColor={trimColor}
        screenColor={screenColor}
        antenna={!small}
        floorShadow={!small}
        interactive={false}
        seed={seed}
        aria-hidden
      />
      {online ? (
        <span
          className="agent-avatar-online-dot"
          style={{ width: dotPx, height: dotPx }}
          aria-hidden
        />
      ) : null}
    </>
  );

  const wrapClass = [
    "agent-avatar-wrap",
    `status-${st}`,
    clickable ? "clickable" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  // Slightly taller than size so hops/antenna glow are less clipped by tight parents;
  // RobotHead's layout box stays `size`×`size` inside.
  const wrapStyle = { width: size, height: size };

  if (clickable) {
    return (
      <button
        type="button"
        className={wrapClass}
        style={wrapStyle}
        onClick={onClick}
        title={title || `设置 ${name}`}
        aria-label={`设置 ${name} 形象`}
      >
        {face}
      </button>
    );
  }

  return (
    <span className={wrapClass} style={wrapStyle} title={title || name} aria-hidden>
      {face}
    </span>
  );
}
