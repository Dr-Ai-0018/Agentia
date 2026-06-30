import { Send } from "lucide-react";
import { useMemo } from "react";
import { buildReplyRequest, canSendReply, findForbiddenReplyTerms } from "../../lib/reply";
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

  return (
    <section className="reply-composer">
      <ReplyBoundaryNotice />
      <textarea
        value={draft.body}
        onChange={(event) => onDraftChange({ ...draft, body: event.target.value })}
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
      <div className="reply-composer__footer">
        <small>Payload preview: {JSON.stringify(buildReplyRequest(draft))}</small>
        <button type="button" className="primary-button" disabled={!sendable} onClick={() => onSubmit(draft)}>
          <Send size={16} />
          Send as Chenglin
        </button>
      </div>
    </section>
  );
}
