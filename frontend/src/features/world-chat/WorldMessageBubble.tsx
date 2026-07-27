import { Reply, TriangleAlert } from "lucide-react";
import type { WorldMessage } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";
import { WorldMarkdown } from "./WorldMarkdown";
import { visibleWorldMessageBody } from "./messageIntegrity";

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
      {message.bodyIntegrity === "legacy_truncated" ? (
        <div className="world-message__integrity" role="note">
          <TriangleAlert size={14} />
          <span>旧版本保存到这里就截断了，后文没有写入记录。下面显示的是已保存残片，不是完整原文。</span>
        </div>
      ) : null}
      <WorldMarkdown body={visibleWorldMessageBody(message)} />
      {message.bodyIntegrity === "legacy_truncated" ? (
        <div className="world-message__cutoff" aria-label="旧版本正文截断位置">
          <span>记录到此中断</span>
          <strong>后文未保存</strong>
        </div>
      ) : null}
      {message.status === "pending" && onReply ? (
        <button type="button" className="world-message__reply" onClick={() => onReply(message)}>
          <Reply size={13} />
          {message.bodyIntegrity === "legacy_truncated" ? "回复这条残片" : "回复这条"}
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
