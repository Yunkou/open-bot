import type { AttachmentMeta } from "../api";
import { stripThinkTags } from "../lib/stripThink";
import { ResultOrientedMessage } from "./ArtifactCards";
import { HostConfirmCard, parseHostConfirm } from "./HostConfirmCard";

export type ChatMessageData = {
  id: string;
  role: string;
  content: string;
  streaming?: boolean;
  attachments?: AttachmentMeta[];
  agent_id?: string;
  agent_name?: string;
  created_at?: string;
};

type Props = {
  message: ChatMessageData;
  /** Fallback selected bot when message.agent_id is missing. */
  agentId?: string;
  onHostDecide?: (ok: boolean) => void;
};

export function formatMessageTime(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const hm = d.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return hm;
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`;
}

function formatSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function ChatMessage({ message, agentId, onHostDecide }: Props) {
  const isUser = message.role === "user";
  const isSummary = message.role === "summary";
  const time = formatMessageTime(message.created_at);
  const timeEl = time ? (
    <time className="chat-time" dateTime={message.created_at}>
      {time}
    </time>
  ) : null;
  const speakerId = message.agent_id || agentId;

  const visible =
    isUser || message.role === "host_confirm" ? message.content : stripThinkTags(message.content);
  if (message.streaming && !visible && !isUser) {
    return null;
  }
  if (!isUser && message.role !== "host_confirm" && !message.streaming && !visible.trim()) {
    return null;
  }

  if (message.role === "host_confirm") {
    const item = parseHostConfirm(message.content);
    if (!item) return null;
    return (
      <div className="chat-row chat-row-assistant">
        <HostConfirmCard item={item} onDecide={item.status === "pending" ? onHostDecide : undefined} />
        {timeEl}
      </div>
    );
  }

  if (isUser) {
    return (
      <div className="chat-row chat-row-user">
        <div className="bubble bubble-user">
          {message.attachments && message.attachments.length > 0 ? (
            <div className="msg-attach-chips">
              {message.attachments.map((a) => (
                <span key={a.id || a.name} className="msg-attach-chip">
                  {a.name}
                  {a.size ? <span className="msg-attach-size">{formatSize(a.size)}</span> : null}
                </span>
              ))}
            </div>
          ) : null}
          {visible ? <div className="bubble-text">{visible}</div> : null}
        </div>
        {timeEl}
      </div>
    );
  }

  return (
    <div className={`chat-row chat-row-assistant${isSummary ? " chat-row-summary" : ""}`}>
      <div className="bubble bubble-assistant">
        {isSummary ? <div className="msg-role">摘要</div> : null}
        <ResultOrientedMessage
          content={visible}
          streaming={message.streaming}
          agentId={speakerId}
        />
      </div>
      {timeEl}
    </div>
  );
}
