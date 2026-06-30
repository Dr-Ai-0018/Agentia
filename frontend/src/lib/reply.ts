import type { ReplyDraft, ReplyRequest } from "../types/domain";

const forbiddenPatterns = [
  /dashboard/i,
  /telemetry/i,
  /run id/i,
  /token/i,
  /cache/i,
  /internal usd/i,
  /operator/i,
  /admin/i,
  /管理员/,
  /后台/,
  /隐藏测试/,
  /测试规则/,
];

export function buildReplyRequest(draft: ReplyDraft): ReplyRequest {
  return {
    resident_id: draft.residentId,
    thread_id: draft.threadId,
    target_id: draft.targetId,
    kind: draft.kind,
    body: draft.body,
    client_nonce: draft.clientNonce,
  };
}

export function findForbiddenReplyTerms(body: string): string[] {
  return forbiddenPatterns
    .filter((pattern) => pattern.test(body))
    .map((pattern) => pattern.source.replace(/\\/g, ""));
}

export function canSendReply(draft: ReplyDraft): boolean {
  return draft.boundaryAck && draft.body.trim().length > 0 && findForbiddenReplyTerms(draft.body).length === 0;
}
