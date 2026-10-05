import type { AttachmentMeta, ReactionSummary } from "../api";
import { MessageReactions } from "./MessageReactions";
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
  reply_to_id?: string;
  thread_root_id?: string;
  reactions?: ReactionSummary[];
};

type Props = {
  message: ChatMessageData;
  /** Fallback selected bot when message.agent_id is missing. */
  agentId?: string;
  onHostDecide?: (ok: boolean) => void;
  /** Quoted parent preview when rendering a reply. */
  replyQuote?: { who: string; text: string } | null;
  /** Number of replies in this message's thread (main timeline roots). */
  replyCount?: number;
  onReply?: (message: ChatMessageData) => void;
  onOpenThread?: (rootId: string) => void;
  onJumpToParent?: (parentId: string) => void;
  /** Compact style inside an open thread panel. */
  dense?: boolean;
  onToggleReaction?: (messageId: string, emoji: string) => void | Promise<void>;
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

function previewText(content: string, max = 80): string {
  const t = stripThinkTags(content).replace(/\s+/g, " ").trim();
  if (t.length <= max) return t;
  return t.slice(0, max) + "…";
}

function canReact(message: ChatMessageData): boolean {
  if (message.streaming) return false;
  if (message.role === "host_confirm" || message.role === "handoff") return false;
  const id = message.id || "";
  if (!id || id.startsWith("local-") || id.startsWith("onboard-")) return false;
  return true;
}

export function ChatMessage({
  message,
  agentId,
  onHostDecide,
  replyQuote,
  replyCount = 0,
  onReply,
  onOpenThread,
  onJumpToParent,
  dense,
  onToggleReaction,
}: Props) {
  const isUser = message.role === "user";
  const isSummary = message.role === "summary";
  const time = formatMessageTime(message.created_at);
  const timeEl = time ? (
    <time className="chat-time" dateTime={message.created_at}>
      {time}
    </time>
  ) : null;
  const speakerId = message.agent_id || agentId;
  const canReply = Boolean(onReply) && (message.role === "user" || message.role === "assistant") && !message.streaming;

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
      <div className="chat-row chat-row-assistant" data-msg-id={message.id}>
        <HostConfirmCard item={item} onDecide={item.status === "pending" ? onHostDecide : undefined} />
        {timeEl}
      </div>
    );
  }

  const actions = canReply ? (
    <div className="msg-actions">
      <button type="button" className="msg-action-btn" title="回复" onClick={() => onReply?.(message)}>
        回复
      </button>
      {replyCount > 0 && onOpenThread ? (
        <button
          type="button"
          className="msg-action-btn msg-action-thread"
          title="查看线程"
          onClick={() => onOpenThread(message.thread_root_id || message.id)}
        >
          {replyCount} 条回复
        </button>
      ) : null}
      {message.reply_to_id && onJumpToParent ? (
        <button
          type="button"
          className="msg-action-btn"
          title="跳到原消息"
          onClick={() => onJumpToParent(message.reply_to_id!)}
        >
          原消息
        </button>
      ) : null}
    </div>
  ) : replyCount > 0 && onOpenThread ? (
    <div className="msg-actions">
      <button
        type="button"
        className="msg-action-btn msg-action-thread"
        title="查看线程"
        onClick={() => onOpenThread(message.thread_root_id || message.id)}
      >
        {replyCount} 条回复
      </button>
    </div>
  ) : null;

  const quoteEl = replyQuote ? (
    <button
      type="button"
      className="msg-reply-quote"
      title="跳到原消息"
      onClick={() => message.reply_to_id && onJumpToParent?.(message.reply_to_id)}
    >
      <span className="msg-reply-quote-who">{replyQuote.who}</span>
      <span className="msg-reply-quote-text">{previewText(replyQuote.text, 100)}</span>
    </button>
  ) : null;

  const interactive = canReact(message) && Boolean(onToggleReaction);
  const reactionsBar = (
    <MessageReactions
      reactions={message.reactions}
      interactive={interactive}
      onToggle={
        interactive
          ? (emoji) => onToggleReaction?.(message.id, emoji)
          : undefined
      }
    />
  );

  if (isUser) {
    return (
      <div className={`chat-row chat-row-user${dense ? " chat-row-dense" : ""}`} data-msg-id={message.id}>
        <div className="bubble bubble-user">
          {quoteEl}
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
        {reactionsBar}
        {actions}
        {timeEl}
      </div>
    );
  }

  return (
    <div
      className={`chat-row chat-row-assistant${isSummary ? " chat-row-summary" : ""}${dense ? " chat-row-dense" : ""}`}
      data-msg-id={message.id}
    >
      <div className="bubble bubble-assistant">
        {quoteEl}
        {isSummary ? <div className="msg-role">摘要</div> : null}
        <ResultOrientedMessage
          content={visible}
          streaming={message.streaming}
          agentId={speakerId}
        />
      </div>
      {reactionsBar}
      {actions}
      {timeEl}
    </div>
  );
}
