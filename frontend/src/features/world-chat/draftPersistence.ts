import type { ReplyDraft } from "../../types/domain";

// Bump the version if the persisted shape changes; a stale shape is treated
// as absent rather than a crash.
const STORAGE_KEY = "arena-console:drafts:v1";

// Only these fields survive a refresh. boundaryAck deliberately does NOT —
// the operator has to look at the restored text and re-acknowledge it, which
// matches the "editing body clears ack" invariant already enforced in the
// composer. clientNonce is per-session, so we regenerate on load too.
type PersistedDraft = Pick<
  ReplyDraft,
  "residentId" | "threadId" | "targetId" | "kind" | "body" | "closeTicket"
>;

export function loadPersistedDrafts(): Record<string, ReplyDraft> {
  if (typeof window === "undefined") return {};
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return {};
  }
  if (!raw) return {};

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {};
  }
  if (!parsed || typeof parsed !== "object") return {};

  const out: Record<string, ReplyDraft> = {};
  for (const [threadId, value] of Object.entries(parsed as Record<string, unknown>)) {
    if (!value || typeof value !== "object") continue;
    const candidate = value as Partial<PersistedDraft>;
    if (
      typeof candidate.body !== "string" ||
      typeof candidate.targetId !== "string" ||
      typeof candidate.residentId !== "string" ||
      typeof candidate.threadId !== "string" ||
      (candidate.kind !== "chat_reply" && candidate.kind !== "ticket_reply")
    ) {
      continue;
    }
    out[threadId] = {
      residentId: candidate.residentId,
      threadId: candidate.threadId,
      targetId: candidate.targetId,
      kind: candidate.kind,
      body: candidate.body,
      boundaryAck: false,
      clientNonce: crypto.randomUUID(),
      ...(candidate.closeTicket !== undefined ? { closeTicket: candidate.closeTicket } : {}),
    };
  }
  return out;
}

export function persistDrafts(drafts: Record<string, ReplyDraft>): void {
  if (typeof window === "undefined") return;
  const forStorage: Record<string, PersistedDraft> = {};
  for (const [threadId, draft] of Object.entries(drafts)) {
    if (!draft || !draft.body.trim()) continue;
    forStorage[threadId] = {
      residentId: draft.residentId,
      threadId: draft.threadId,
      targetId: draft.targetId,
      kind: draft.kind,
      body: draft.body,
      ...(draft.closeTicket !== undefined ? { closeTicket: draft.closeTicket } : {}),
    };
  }
  try {
    if (Object.keys(forStorage).length === 0) {
      window.localStorage.removeItem(STORAGE_KEY);
    } else {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(forStorage));
    }
  } catch {
    // Quota exceeded or storage disabled — losing persistence is acceptable;
    // the composer stays functional in memory.
  }
}
