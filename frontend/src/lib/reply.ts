import type {
  ReplyDraft,
  WorldChatReplyRequest,
  WorldTicketReplyRequest,
} from "../types/domain";

// Forbidden reply patterns. This list must stay in lockstep with the backend
// reply_guard.go pattern list. Backend is the authoritative source; frontend
// only mirrors so the operator sees "this word is private" before the roundtrip.
//
// If backend adds/removes a pattern, update here too. The mismatch does NOT
// break the boundary — backend still rejects — but it degrades the UX.
const forbiddenPatterns: RegExp[] = [
  /\bdashboard\b/i,
  /仪表盘/,
  /\btelemetry\b/i,
  /\brun\s*id\b/i,
  /\borchestrator-\d{8}T/i,
  /\btoken\b/i,
  /令牌/,
  /\bcache\b/i,
  /缓存/,
  /\binternal\s*usd\b/i,
  /\busd\b/i,
  /\$\s*\d/,
  /\bbilling\b/i,
  /\bspend\b/i,
  /\boperator\b/i,
  /\badmin\b/i,
  /管理员/,
  /后台/,
  /\bquota\b/i,
  /\bbudget\b/i,
  /\bspark\b/i,
  /\bphase\b/i,
  /\bround\b/i,
  /\bintervention\b/i,
  /\bmaintenance\b/i,
  /\bP[0-2]\b/i,
  /预算/,
  /审计/,
  /运维/,
  /维护/,
  /内部额度/,
  /隐藏测试/,
  /测试规则/,
  /验收/,
];

export function findForbiddenReplyTerms(body: string): string[] {
  return forbiddenPatterns
    .filter((pattern) => pattern.test(body))
    .map((pattern) => pattern.source.replace(/\\/g, ""));
}

export function canSendReply(draft: ReplyDraft): boolean {
  return (
    draft.boundaryAck &&
    draft.body.trim().length > 0 &&
    findForbiddenReplyTerms(draft.body).length === 0
  );
}

// buildChatReplyRequest maps the local draft state into the on-wire shape the
// backend expects at POST /api/reply. Deliberately drops resident_id, thread_id,
// kind, and client_nonce — those live in the composer for UX, not on the wire.
// message_id is the server's opaque identifier for the message being replied to.
export function buildChatReplyRequest(draft: ReplyDraft): WorldChatReplyRequest {
  if (!draft.boundaryAck) {
    throw new Error("boundary_ack must be true before building a chat reply");
  }
  return {
    message_id: draft.targetId,
    body: draft.body,
    boundary_ack: true,
  };
}

// buildTicketReplyRequest maps the local draft state into the on-wire shape the
// backend expects at POST /api/ticket-reply. Includes the optional close flag.
export function buildTicketReplyRequest(draft: ReplyDraft): WorldTicketReplyRequest {
  if (!draft.boundaryAck) {
    throw new Error("boundary_ack must be true before building a ticket reply");
  }
  return {
    ticket_id: draft.targetId,
    body: draft.body,
    close: draft.closeTicket ?? false,
    boundary_ack: true,
  };
}

