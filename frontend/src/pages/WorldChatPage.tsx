// WorldChatPage is the entry to the WORLD-VISIBLE surface. Its props are
// pinned to a narrow shape via AssertNoGodViewLeak so that any future attempt
// to widen them with a god-view field (telemetry, budgets, runs, alerts, etc.)
// fails at type-check. Do not weaken this boundary — the reply composer subtree
// downstream depends on it.

import { useEffect, useMemo, useState } from "react";
import { arenaApi } from "../lib/api/client";
import { buildChatReplyRequest, buildTicketReplyRequest } from "../lib/reply";
import { ReplyComposer } from "../features/world-chat/ReplyComposer";
import { WorldChatThread } from "../features/world-chat/WorldChatThread";
import { draftFromThread } from "../features/world-chat/worldSafeMappers";
import { loadPersistedDrafts, persistDrafts } from "../features/world-chat/draftPersistence";
import type {
  AssertNoGodViewLeak,
  ReplyDraft,
  WorldVisibleThread,
} from "../types/domain";
import { residentLabel } from "../features/residents/residentTheme";

const kindLabel: Record<WorldVisibleThread["kind"], string> = {
  chat: "对话",
  ticket: "单据",
};

type WorldChatPageProps = AssertNoGodViewLeak<{
  threads: WorldVisibleThread[];
  activeThreadId: string;
  onSelectThread: (threadId: string) => void;
}>;

export function WorldChatPage({ threads, activeThreadId, onSelectThread }: WorldChatPageProps) {
  const activeThread = threads.find((thread) => thread.threadId === activeThreadId) ?? threads[0];
  const [drafts, setDrafts] = useState<Record<string, ReplyDraft>>(() => loadPersistedDrafts());
  const [sendState, setSendState] = useState<"idle" | "sending" | "sent">("idle");

  useEffect(() => {
    if (!activeThread) return;
    setDrafts((current) => current[activeThread.threadId] ? current : {
      ...current,
      [activeThread.threadId]: draftFromThread(activeThread),
    });
  }, [activeThread]);

  useEffect(() => {
    persistDrafts(drafts);
  }, [drafts]);

  const draft = activeThread ? drafts[activeThread.threadId] : undefined;
  const selectedMessages = useMemo(() => activeThread?.messages ?? [], [activeThread]);

  async function submitDraft(input: ReplyDraft) {
    setSendState("sending");
    try {
      if (input.kind === "ticket_reply") {
        await arenaApi.sendWorldTicketReply(buildTicketReplyRequest(input));
      } else {
        await arenaApi.sendWorldChatReply(buildChatReplyRequest(input));
      }
      setSendState("sent");
      setDrafts((current) => {
        if (!(input.threadId in current)) return current;
        const next = { ...current };
        delete next[input.threadId];
        return next;
      });
    } catch (submitError) {
      setSendState("idle");
      throw submitError;
    }
  }

  if (!activeThread || !draft) {
    return (
      <div className="world-chat-empty">
        <h2>暂时没有需要程林回话的线</h2>
        <p>住户们说了话之后，会出现在这里等程林回。</p>
      </div>
    );
  }

  return (
    <div className="world-chat-page">
      <aside className="thread-list">
        <div className="thread-list__title">
          <span className="thread-list__title-h">程林还没回话的</span>
          <span className="thread-list__title-n">{threads.length} 条</span>
        </div>
        {threads.map((thread) => {
          const last = thread.messages[thread.messages.length - 1];
          const isActive = thread.threadId === activeThread.threadId;
          const hasDraft = Boolean(drafts[thread.threadId]?.body.trim());
          return (
            <button
              type="button"
              key={thread.threadId}
              className={isActive ? "active" : ""}
              onClick={() => onSelectThread(thread.threadId)}
            >
              <div className="thread-list__row-head">
                <strong>
                  {residentLabel(thread.resident)}
                  {hasDraft ? <span className="thread-list__draft-dot" title="有未发出的草稿">·</span> : null}
                </strong>
                <span>{kindLabel[thread.kind]}</span>
              </div>
              <small>{last?.body}</small>
            </button>
          );
        })}
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
        {sendState === "sent" && <div className="sent-banner">回话已送出。</div>}
      </div>
    </div>
  );
}
