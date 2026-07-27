import { MessageCircleMore } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { residentLabel } from "../features/residents/residentTheme";
import { ReplyComposer } from "../features/world-chat/ReplyComposer";
import { TicketWorkspace } from "../features/world-chat/TicketWorkspace";
import { WorldChatThread } from "../features/world-chat/WorldChatThread";
import { loadPersistedDrafts, persistDrafts } from "../features/world-chat/draftPersistence";
import { draftFromThread } from "../features/world-chat/worldSafeMappers";
import { arenaApi } from "../lib/api/client";
import { buildChatReplyRequest } from "../lib/reply";
import type {
  AssertWorldSurfaceIsolated,
  FollowupItem,
  ReplyDraft,
  WorldMessage,
  WorldVisibleThread,
} from "../types/domain";

type WorldChatPageProps = AssertWorldSurfaceIsolated<{
  threads: WorldVisibleThread[];
  interventions: FollowupItem[];
  activeThreadId: string;
  onSelectThread: (threadId: string) => void;
}>;

type SendState = { status: "idle" | "sending" | "error"; error?: string };

export function WorldChatPage({ threads, interventions, activeThreadId, onSelectThread }: WorldChatPageProps) {
  const [view, setView] = useState<"chat" | "tickets">("chat");
  const [localThreads, setLocalThreads] = useState<WorldVisibleThread[]>(threads);
  const [drafts, setDrafts] = useState<Record<string, ReplyDraft>>(() => loadPersistedDrafts());
  const [sendByThread, setSendByThread] = useState<Record<string, SendState>>({});
  const [replyTargetByThread, setReplyTargetByThread] = useState<Record<string, WorldMessage | undefined>>({});
  const [loadingOlder, setLoadingOlder] = useState("");

  useEffect(() => {
    setLocalThreads((current) => mergeThreads(current, threads));
  }, [threads]);

  const activeThread = localThreads.find((thread) => thread.threadId === activeThreadId) ?? localThreads[0];

  useEffect(() => {
    if (!activeThread) return;
    setDrafts((current) => current[activeThread.threadId] ? current : {
      ...current,
      [activeThread.threadId]: draftFromThread(activeThread),
    });
  }, [activeThread]);

  useEffect(() => persistDrafts(drafts), [drafts]);

  const draft = activeThread ? drafts[activeThread.threadId] : undefined;
  const sendState = activeThread ? sendByThread[activeThread.threadId] ?? { status: "idle" } : { status: "idle" as const };
  const replyTarget = activeThread ? replyTargetByThread[activeThread.threadId] : undefined;
  const contacts = useMemo(() => localThreads, [localThreads]);

  async function submitDraft(input: ReplyDraft) {
    const thread = localThreads.find((item) => item.threadId === input.threadId);
    if (!thread) return;
    setSendByThread((current) => ({ ...current, [input.threadId]: { status: "sending" } }));
    try {
      const target = replyTargetByThread[input.threadId];
      const sent = target
        ? await arenaApi.sendWorldChatReply(buildChatReplyRequest({ ...input, targetId: target.id }))
        : await arenaApi.sendWorldChat({ resident: input.residentId, body: input.body, boundary_ack: true });
      setLocalThreads((current) => current.map((item) => item.threadId === input.threadId
        ? appendMessage(item, sent)
        : item));
      setDrafts((current) => ({
        ...current,
        [input.threadId]: { ...input, body: "", boundaryAck: false, targetId: "", clientNonce: crypto.randomUUID() },
      }));
      setReplyTargetByThread((current) => ({ ...current, [input.threadId]: undefined }));
      setSendByThread((current) => ({ ...current, [input.threadId]: { status: "idle" } }));
    } catch (error) {
      setSendByThread((current) => ({
        ...current,
        [input.threadId]: { status: "error", error: error instanceof Error ? error.message : String(error) },
      }));
    }
  }

  async function loadOlder() {
    if (!activeThread?.nextBefore || loadingOlder) return;
    setLoadingOlder(activeThread.threadId);
    try {
      const page = await arenaApi.getWorldThreadPage(activeThread.resident, activeThread.nextBefore);
      setLocalThreads((current) => current.map((thread) => thread.threadId === activeThread.threadId ? {
        ...thread,
        messages: mergeMessages(page.messages, thread.messages),
        total: page.total,
        hasMore: page.hasMore,
        nextBefore: page.nextBefore,
      } : thread));
    } finally {
      setLoadingOlder("");
    }
  }

  if (!activeThread || !draft) {
    return <div className="world-chat-empty">会话正在连上。</div>;
  }

  return (
    <div className="world-communication">
      <div className="world-communication__tabs" role="tablist" aria-label="对话与单据">
        <button type="button" role="tab" aria-selected={view === "chat"} className={view === "chat" ? "active" : ""} onClick={() => setView("chat")}>对话</button>
        <button type="button" role="tab" aria-selected={view === "tickets"} className={view === "tickets" ? "active" : ""} onClick={() => setView("tickets")}>单据</button>
      </div>
      {view === "tickets" ? <TicketWorkspace interventions={interventions} /> : <div className="world-chat-page">
      <aside className="thread-list" aria-label="住户会话">
        <div className="thread-list__title">
          <span className="thread-list__title-h">对话</span>
          <span className="thread-list__title-n">3 位住户</span>
        </div>
        {contacts.map((thread) => {
          const isActive = thread.threadId === activeThread.threadId;
          const hasDraft = Boolean(drafts[thread.threadId]?.body.trim());
          return (
            <button
              type="button"
              key={thread.threadId}
              className={isActive ? "active" : ""}
              onClick={() => onSelectThread(thread.threadId)}
            >
              <span className="thread-list__avatar">{residentLabel(thread.resident).slice(0, 1)}</span>
              <span className="thread-list__body">
                <span className="thread-list__row-head">
                  <strong>{residentLabel(thread.resident)}{hasDraft ? <i>草稿</i> : null}</strong>
                  <time>{formatContactTime(thread.lastMessageAt)}</time>
                </span>
                <small>{thread.lastPreview || thread.messages.at(-1)?.body || "还没有说过话"}</small>
              </span>
              {thread.pendingCount > 0 ? <span className="thread-list__unread">{thread.pendingCount > 99 ? "99+" : thread.pendingCount}</span> : null}
            </button>
          );
        })}
      </aside>
      <div className="world-chat-main">
        <div className="world-chat-main__head">
          <MessageCircleMore size={18} />
          <div><strong>{residentLabel(activeThread.resident)}</strong><span>{activeThread.total} 条消息</span></div>
        </div>
        <WorldChatThread
          thread={activeThread}
          onLoadOlder={loadOlder}
          onReply={(message) => setReplyTargetByThread((current) => ({ ...current, [activeThread.threadId]: message }))}
          isLoadingOlder={loadingOlder === activeThread.threadId}
        />
        <ReplyComposer
          draft={draft}
          replyTarget={replyTarget}
          isSending={sendState.status === "sending"}
          error={sendState.error}
          onCancelReply={() => setReplyTargetByThread((current) => ({ ...current, [activeThread.threadId]: undefined }))}
          onDraftChange={(next) => {
            setDrafts((current) => ({ ...current, [next.threadId]: next }));
            setSendByThread((current) => ({ ...current, [next.threadId]: { status: "idle" } }));
          }}
          onSubmit={submitDraft}
        />
      </div>
      </div>}
    </div>
  );
}

function mergeThreads(current: WorldVisibleThread[], incoming: WorldVisibleThread[]): WorldVisibleThread[] {
  if (current.length === 0) return incoming;
  const currentByID = new Map(current.map((thread) => [thread.threadId, thread]));
  return incoming.map((next) => {
    const previous = currentByID.get(next.threadId);
    if (!previous) return next;
    const hasLoadedOlder = previous.messages.length > next.messages.length;
    return {
      ...next,
      messages: mergeMessages(previous.messages, next.messages),
      hasMore: hasLoadedOlder ? previous.hasMore : next.hasMore,
      nextBefore: hasLoadedOlder ? previous.nextBefore : next.nextBefore,
    };
  });
}

function mergeMessages(...groups: WorldMessage[][]): WorldMessage[] {
  const byID = new Map<string, WorldMessage>();
  for (const group of groups) for (const message of group) byID.set(message.id, message);
  return [...byID.values()].sort((a, b) => a.createdAt.localeCompare(b.createdAt));
}

function appendMessage(thread: WorldVisibleThread, message: WorldMessage): WorldVisibleThread {
  const messages = mergeMessages(thread.messages, [message]);
  return {
    ...thread,
    messages,
    total: Math.max(thread.total + (messages.length > thread.messages.length ? 1 : 0), messages.length),
    lastMessageAt: message.createdAt,
    lastPreview: message.body,
    deliveredCount: thread.deliveredCount + (message.status === "delivered" ? 1 : 0),
  };
}

function formatContactTime(value?: string): string {
  if (!value) return "";
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) return value;
  const now = new Date();
  if (time.toDateString() === now.toDateString()) {
    return new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" }).format(time);
  }
  return new Intl.DateTimeFormat(undefined, { month: "numeric", day: "numeric" }).format(time);
}
