// ReplyComposer is a WORLD-VISIBLE surface. It emits Chenglin's world-facing
// reply and must never receive operator god-view data (run ids, budgets,
// phases, telemetry, dashboards). Its prop shape below is deliberately
// narrow — only the reply draft state and callback handlers.
//
// If you need to display or reason about operator context, do it OUTSIDE this
// subtree, never as a prop or hook here. The types/domain.ts AssertNoGodViewLeak
// guard on WorldChatPageProps blocks a whole class of accidental leaks at the
// page boundary; this file relies on that upstream contract.

import { Send } from "lucide-react";
import { useMemo } from "react";
import {
  buildChatReplyRequest,
  buildTicketReplyRequest,
  canSendReply,
  findForbiddenReplyTerms,
} from "../../lib/reply";
import type { ReplyDraft } from "../../types/domain";
import { ReplyBoundaryNotice } from "./ReplyBoundaryNotice";

type ReplyComposerProps = {
  draft: ReplyDraft;
  onDraftChange: (draft: ReplyDraft) => void;
  onSubmit: (draft: ReplyDraft) => Promise<void> | void;
  isSending?: boolean;
};

export function ReplyComposer({ draft, onDraftChange, onSubmit, isSending = false }: ReplyComposerProps) {
  const forbiddenTerms = useMemo(() => findForbiddenReplyTerms(draft.body), [draft.body]);
  const sendable = canSendReply(draft) && !isSending;
  const isTicket = draft.kind === "ticket_reply";

  const payloadPreview = useMemo(() => {
    if (!draft.boundaryAck || draft.body.trim().length === 0) return null;
    try {
      return isTicket ? buildTicketReplyRequest(draft) : buildChatReplyRequest(draft);
    } catch {
      return null;
    }
  }, [draft, isTicket]);

  return (
    <section className="reply-composer">
      <ReplyBoundaryNotice />
      <textarea
        value={draft.body}
        onChange={(event) => {
          const nextBody = event.target.value;
          if (nextBody === draft.body) return;
          // Editing the body clears the boundary acknowledgement — the operator
          // must re-check it every time content changes, so trivial auto-check
          // patterns (checkbox permanently true) do not silently ship edits.
          onDraftChange({ ...draft, body: nextBody, boundaryAck: false });
        }}
        placeholder="像程林一样回复，只使用世界内自然可说的信息。"
        rows={8}
      />
      {forbiddenTerms.length > 0 && (
        <div className="reply-warning">
          Keep private: {forbiddenTerms.join(", ")}
        </div>
      )}
      <label className="boundary-check">
        <input
          type="checkbox"
          checked={draft.boundaryAck}
          onChange={(event) => onDraftChange({ ...draft, boundaryAck: event.target.checked })}
        />
        <span>I checked this reply against the world-safe boundary.</span>
      </label>
      {isTicket && (
        <label className="ticket-close">
          <input
            type="checkbox"
            checked={draft.closeTicket ?? false}
            onChange={(event) => onDraftChange({ ...draft, closeTicket: event.target.checked })}
          />
          <span>Close this ticket after sending.</span>
        </label>
      )}
      <div className="reply-composer__footer">
        <small>Payload preview: {payloadPreview ? JSON.stringify(payloadPreview) : "—"}</small>
        <button type="button" className="primary-button" disabled={!sendable} onClick={() => onSubmit(draft)}>
          <Send size={16} />
          Send as Chenglin
        </button>
      </div>
    </section>
  );
}