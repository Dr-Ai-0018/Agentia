import type { WorldVisibleThread } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";
import { WorldMessageBubble } from "./WorldMessageBubble";

export function WorldChatThread({ thread }: { thread: WorldVisibleThread }) {
  return (
    <section className="world-thread">
      <header className="world-thread__header">
        <div>
          <p className="eyebrow">World Chat</p>
          <h2>{residentLabel(thread.resident)}</h2>
        </div>
        <span>{thread.kind}</span>
      </header>
      <div className="world-thread__messages">
        {thread.messages.map((message) => (
          <WorldMessageBubble key={message.id} message={message} />
        ))}
      </div>
    </section>
  );
}
