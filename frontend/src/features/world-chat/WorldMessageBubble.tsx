import { Reply } from "lucide-react";
import type { WorldMessage } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";

export function WorldMessageBubble({ message, onReply }: { message: WorldMessage; onReply?: (message: WorldMessage) => void }) {
  const isChenglin = message.from === "chenglin";
  let author = "Chenglin";
  if (message.from !== "chenglin") {
    author = residentLabel(message.from);
  }

  return (
    <article className={`world-message ${isChenglin ? "world-message--chenglin" : "world-message--resident"}`}>
      <header>
        <strong>{author}</strong>
        <time>{formatMessageTime(message.createdAt)}</time>
      </header>
      <p>{message.body}</p>
      {message.status === "pending" && onReply ? (
        <button type="button" className="world-message__reply" onClick={() => onReply(message)}>
          <Reply size={13} />
          回复这条
        </button>
      ) : null}
    </article>
  );
}

function formatMessageTime(value: string): string {
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(time);
}
