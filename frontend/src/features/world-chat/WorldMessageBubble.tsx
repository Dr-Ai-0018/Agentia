import type { WorldMessage } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";

export function WorldMessageBubble({ message }: { message: WorldMessage }) {
  const isChenglin = message.from === "chenglin";
  let author = "Chenglin";
  if (message.from !== "chenglin") {
    author = residentLabel(message.from);
  }

  return (
    <article className={`world-message ${isChenglin ? "world-message--chenglin" : "world-message--resident"}`}>
      <header>
        <strong>{author}</strong>
        <time>{message.createdAt}</time>
      </header>
      <p>{message.body}</p>
    </article>
  );
}
