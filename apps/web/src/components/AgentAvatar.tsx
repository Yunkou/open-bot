import { avatarColor, avatarInitials } from "./avatarColor";

type Props = {
  id?: string;
  name: string;
  size?: number;
  className?: string;
};

export function AgentAvatar({ id, name, size = 28, className = "" }: Props) {
  const bg = avatarColor(id || name);
  const darkText = bg === "#fee440";
  return (
    <span
      className={`agent-avatar ${className}`.trim()}
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
  );
}
