import type { FollowupItem, ReplyDraft, WorldVisibleThread } from "../../types/domain";

export function draftFromThread(thread: WorldVisibleThread): ReplyDraft {
  return {
    residentId: thread.resident,
    threadId: thread.threadId,
    targetId: thread.targetId ?? "",
    kind: thread.kind === "ticket" ? "ticket_reply" : "chat_reply",
    body: "",
    boundaryAck: false,
    clientNonce: crypto.randomUUID(),
  };
}

export function threadIdFromFollowup(followup: FollowupItem): string {
  if (!followup.threadId) throw new Error(`Followup has no world thread: ${followup.targetId}`);
  return followup.threadId;
}
