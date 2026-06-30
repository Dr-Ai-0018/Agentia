import { useEffect, useMemo, useState } from "react";
import { Panel } from "../components/ui/Panel";
import { arenaApi } from "../lib/api/client";
import { buildReplyRequest } from "../lib/reply";
import { ReplyComposer } from "../features/world-chat/ReplyComposer";
import { WorldChatThread } from "../features/world-chat/WorldChatThread";
import { draftFromThread } from "../features/world-chat/worldSafeMappers";
import type { ReplyDraft, WorldVisibleThread } from "../types/domain";
import { residentLabel } from "../features/residents/residentTheme";

type WorldChatPageProps = {
  threads: WorldVisibleThread[];
  activeThreadId: string;
  onSelectThread: (threadId: string) => void;
};

export function WorldChatPage({ threads, activeThreadId, onSelectThread }: WorldChatPageProps) {
  const activeThread = threads.find((thread) => thread.threadId === activeThreadId) ?? threads[0];
  const [drafts, setDrafts] = useState<Record<string, ReplyDraft>>({});
  const [sendState, setSendState] = useState<"idle" | "sending" | "sent">("idle");

  useEffect(() => {
    if (!activeThread) return;
    setDrafts((current) => current[activeThread.threadId] ? current : {
      ...current,
      [activeThread.threadId]: draftFromThread(activeThread),
    });
  }, [activeThread]);

  const draft = activeThread ? drafts[activeThread.threadId] : undefined;
  const selectedMessages = useMemo(() => activeThread?.messages ?? [], [activeThread]);

  async function submitDraft(input: ReplyDraft) {
    setSendState("sending");
    await arenaApi.sendWorldReply(buildReplyRequest(input));
    setSendState("sent");
  }

  if (!activeThread || !draft) {
    return (
      <Panel title="World Chat">
        <p className="empty-state">No world-visible threads are available.</p>
      </Panel>
    );
  }

  return (
    <div className="world-chat-page">
      <aside className="thread-list">
        <p className="eyebrow">Pending world messages</p>
        {threads.map((thread) => (
          <button
            type="button"
            key={thread.threadId}
            className={thread.threadId === activeThread.threadId ? "active" : ""}
            onClick={() => onSelectThread(thread.threadId)}
          >
            <strong>{residentLabel(thread.resident)}</strong>
            <span>{thread.kind}</span>
            <small>{thread.messages[thread.messages.length - 1]?.body}</small>
          </button>
        ))}
      </aside>
      <div className="world-chat-main">
        <WorldChatThread thread={{ ...activeThread, messages: selectedMessages }} />
        <ReplyComposer
          draft={draft}
          isSending={sendState === "sending"}
          onDraftChange={(nextDraft) => {
            setSendState("idle");
            setDrafts((current) => ({ ...current, [nextDraft.threadId]: nextDraft }));
          }}
          onSubmit={submitDraft}
        />
        {sendState === "sent" && <div className="sent-banner">Mock adapter accepted the reply payload.</div>}
      </div>
    </div>
  );
}
