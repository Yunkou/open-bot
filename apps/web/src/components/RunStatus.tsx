type Props = {
  label: string;
  color?: string;
};

/** Animated thinking / tool-running row (OpenClaw-style blob + gray label). */
export function RunStatus({ label, color = "#e85d4c" }: Props) {
  return (
    <div className="chat-row chat-row-assistant run-status-row" aria-live="polite">
      <div className="run-status">
        <span className="run-status-blob" style={{ color }} aria-hidden>
          <span className="run-status-blob-a" />
          <span className="run-status-blob-b" />
        </span>
        <span className="run-status-label">{label}</span>
      </div>
    </div>
  );
}
