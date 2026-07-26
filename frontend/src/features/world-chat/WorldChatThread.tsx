import { useEffect, useLayoutEffect, useRef } from "react";
import type { WorldMessage, WorldVisibleThread } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";
import { WorldMessageBubble } from "./WorldMessageBubble";

const kindLabel: Record<WorldVisibleThread["kind"], string> = {
  chat: "对话",
  ticket: "单据",
};

type WorldChatThreadProps = {
  thread: WorldVisibleThread;
  onLoadOlder: () => Promise<void>;
  onReply: (message: WorldMessage) => void;
  isLoadingOlder: boolean;
};

export function WorldChatThread({ thread, onLoadOlder, onReply, isLoadingOlder }: WorldChatThreadProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const previousThread = useRef("");
  const previousCount = useRef(0);
  const preserveHeight = useRef<number | null>(null);
  const stickToBottom = useRef(true);

  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    if (preserveHeight.current !== null) {
      element.scrollTop += element.scrollHeight - preserveHeight.current;
      preserveHeight.current = null;
    } else if (previousThread.current !== thread.threadId || (thread.messages.length > previousCount.current && stickToBottom.current)) {
      element.scrollTop = element.scrollHeight;
    }
    previousThread.current = thread.threadId;
    previousCount.current = thread.messages.length;
  }, [thread.threadId, thread.messages.length]);

  useEffect(() => {
    previousCount.current = thread.messages.length;
  }, [thread.messages.length]);

  async function loadOlder() {
    if (scrollRef.current) preserveHeight.current = scrollRef.current.scrollHeight;
    await onLoadOlder();
  }

  return (
    <section className="world-thread">
      <header className="world-thread__header">
        <div>
          <h2>{residentLabel(thread.resident)}</h2>
          <p className="world-thread__sub">跟程林之间的{kindLabel[thread.kind]}</p>
        </div>
        <span className="world-thread__kind">{kindLabel[thread.kind]}</span>
      </header>
      <div
        className="world-thread__messages"
        ref={scrollRef}
        onScroll={(event) => {
          const element = event.currentTarget;
          stickToBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
        }}
      >
        {thread.hasMore ? (
          <button type="button" className="world-thread__older" disabled={isLoadingOlder} onClick={loadOlder}>
            {isLoadingOlder ? "正在找…" : `查看更早的消息 · 共 ${thread.total} 条`}
          </button>
        ) : thread.messages.length > 0 ? <div className="world-thread__beginning">已经到最早一条了</div> : null}
        {thread.messages.map((message, index) => {
          const previous = thread.messages[index - 1];
          const showDate = !previous || messageDay(previous.createdAt) !== messageDay(message.createdAt);
          return (
            <div className="world-thread__message-row" key={message.id}>
              {showDate ? <div className="world-thread__date">{formatMessageDay(message.createdAt)}</div> : null}
              <WorldMessageBubble message={message} onReply={onReply} />
            </div>
          );
        })}
        {thread.messages.length === 0 ? <div className="world-thread__empty">还没有说过话，从这里开始吧。</div> : null}
      </div>
    </section>
  );
}

function messageDay(value: string): string {
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toDateString();
}

function formatMessageDay(value: string): string {
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) return "";
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (time.toDateString() === today.toDateString()) return "今天";
  if (time.toDateString() === yesterday.toDateString()) return "昨天";
  return new Intl.DateTimeFormat(undefined, { year: "numeric", month: "long", day: "numeric" }).format(time);
}
