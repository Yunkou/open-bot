import {
  avatarInitials,
  resolveAvatarColor,
  resolveAvatarShape,
  type AvatarShape,
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
  onClick?: () => void;
  title?: string;
};

function shapeClass(shape: AvatarShape): string {
  return `shape-${shape}`;
}

export function AgentAvatar({
  id,
  name,
  size = 28,
  className = "",
  shape,
  color,
  status = "idle",
  onClick,
  title,
}: Props) {
  const seed = id || name;
  const bg = resolveAvatarColor(seed, color);
  const sh = resolveAvatarShape(seed, shape);
  const darkText = bg.toLowerCase() === "#fee440";
  const st = (status || "idle") as string;
  const interactive = Boolean(onClick);

  const inner = (
    <>
      <span
        className={`agent-avatar ${shapeClass(sh)}`.trim()}
        style={{
          width: size,
          height: size,
          background: bg,
          color: darkText ? "#1a1a1b" : "#fff",
          fontSize: Math.max(10, Math.round(size * 0.38)),
        }}
        aria-hidden
      >
        {avatarInitials(name)}
      </span>
      {st !== "idle" ? <span className={`agent-avatar-ring status-${st}`} aria-hidden /> : null}
    </>
  );

  const wrapClass = `agent-avatar-wrap status-${st}${interactive ? " clickable" : ""} ${className}`.trim();
  const wrapStyle = { width: size, height: size };

  if (interactive) {
    return (
      <button
        type="button"
        className={wrapClass}
        style={wrapStyle}
        onClick={(e) => {
          // Avatar opens settings; don't also trigger the parent row's select.
          e.stopPropagation();
          onClick?.();
        }}
        onKeyDown={(e) => e.stopPropagation()}
        title={title || `设置 ${name}`}
        aria-label={`设置 ${name} 形象`}
      >
        {inner}
      </button>
    );
  }

  return (
    <span className={wrapClass} style={wrapStyle} title={title || name} aria-hidden>
      {inner}
    </span>
  );
}
