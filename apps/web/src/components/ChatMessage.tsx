import type { AttachmentMeta } from "../api";
import { ResultOrientedMessage } from "./ArtifactCards";

export type ChatMessageData = {
  id: string;
  role: string;
  content: string;
  streaming?: boolean;
  attachments?: AttachmentMeta[];
  agent_id?: string;
  agent_name?: string;
};

type Props = {
  message: ChatMessageData;
  /** Fallback selected bot when message.agent_id is missing. */
  agentId?: string;
};

function formatSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function ChatMessage({ message, agentId }: Props) {
  const isUser = message.role === "user";
  const isSummary = message.role === "summary";

  if (message.streaming && !message.content && !isUser) {
    return null;
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
          {message.content ? <div className="bubble-text">{message.content}</div> : null}
        </div>
      </div>
    );
  }

  return (
    <div className={`chat-row chat-row-assistant${isSummary ? " chat-row-summary" : ""}`}>
      <div className="bubble bubble-assistant">
        {isSummary ? <div className="msg-role">摘要</div> : null}
        <ResultOrientedMessage
          content={message.content}
          streaming={message.streaming}
          agentId={message.agent_id || agentId}
        />
      </div>
    </div>
  );
}
