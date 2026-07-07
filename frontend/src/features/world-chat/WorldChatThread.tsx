import type { WorldVisibleThread } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";
import { WorldMessageBubble } from "./WorldMessageBubble";

const kindLabel: Record<WorldVisibleThread["kind"], string> = {
  chat: "对话",
  ticket: "单据",
};

export function WorldChatThread({ thread }: { thread: WorldVisibleThread }) {
  return (
    <section className="world-thread">
      <header className="world-thread__header">
        <div>
          <h2>{residentLabel(thread.resident)}</h2>
          <p className="world-thread__sub">跟程林之间的{kindLabel[thread.kind]}</p>
        </div>
        <span className="world-thread__kind">{kindLabel[thread.kind]}</span>
      </header>
      <div className="world-thread__messages">
        {thread.messages.map((message) => (
          <WorldMessageBubble key={message.id} message={message} />
        ))}
      </div>
    </section>
  );
}
