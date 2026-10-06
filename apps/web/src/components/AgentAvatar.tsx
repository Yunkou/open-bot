import {
  hashString,
  normalizePresenceStatus,
  resolveAvatarColor,
  resolveAvatarShape,
  type BotPresenceStatus,
} from "./avatarColor";
import { AVATAR_BODY_PATHS, AVATAR_EYE } from "./avatarShapes";

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

/** Unknown status → working (v2.1 product rule); empty → idle. */
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
  const seed = id || name;
  const bg = resolveAvatarColor(seed, color);
  const sh = resolveAvatarShape(seed, shape);
  const st = normalizeStatus(status);
  const interactive = Boolean(onClick);
  const bodyPath = AVATAR_BODY_PATHS[sh];
  const eyeBoost = size <= 32 ? 1.06 : 1;
  const blinkDelayMs = (hashString(seed) % 2800) + 200; // 0.2–3.0s desync
  const noBreath = size <= 24;
  const dotPx = Math.max(6, Math.min(8, Math.round(size * 0.22)));

  const face = (
    <>
      <svg
        className="agent-avatar-face"
        data-status={st}
        viewBox="0 0 32 32"
        width="100%"
        height="100%"
        aria-hidden
        style={{ ["--avatar-blink-delay" as string]: `${blinkDelayMs}ms` }}
      >
        <path className="agent-avatar-body" d={bodyPath} fill={bg} />
        {/* Outer g: optional small-size eye boost; inner g: CSS expression animations */}
        <g
          fill="#fff"
          style={
            eyeBoost !== 1
              ? { transformOrigin: "15.4px 13px", transform: `scale(${eyeBoost})` }
              : undefined
          }
        >
          <g className="agent-avatar-eyes">
            <ellipse
              className="agent-avatar-eye agent-avatar-eye-l"
              cx={AVATAR_EYE.left.cx}
              cy={AVATAR_EYE.left.cy}
              rx={AVATAR_EYE.left.rx}
              ry={AVATAR_EYE.left.ry}
              transform={`rotate(${AVATAR_EYE.left.rotate} ${AVATAR_EYE.left.cx} ${AVATAR_EYE.left.cy})`}
            />
            <ellipse
              className="agent-avatar-eye agent-avatar-eye-r"
              cx={AVATAR_EYE.right.cx}
              cy={AVATAR_EYE.right.cy}
              rx={AVATAR_EYE.right.rx}
              ry={AVATAR_EYE.right.ry}
              transform={`rotate(${AVATAR_EYE.right.rotate} ${AVATAR_EYE.right.cx} ${AVATAR_EYE.right.cy})`}
            />
            <g className="agent-avatar-spirals" fill="none" stroke="#fff" strokeWidth="1.4">
              <circle className="agent-avatar-spiral agent-avatar-spiral-l" cx="12.2" cy="13.4" r="2.45" />
              <circle className="agent-avatar-spiral agent-avatar-spiral-r" cx="18.6" cy="12.6" r="2.45" />
            </g>
            <g className="agent-avatar-spiral-dots" fill="#fff">
              <circle cx="12.2" cy="13.4" r="0.75" />
              <circle cx="18.6" cy="12.6" r="0.75" />
            </g>
          </g>
        </g>
      </svg>
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
    noBreath ? "agent-avatar-wrap--no-breath" : "",
    interactive ? "clickable" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  const wrapStyle = { width: size, height: size };

  if (interactive) {
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
