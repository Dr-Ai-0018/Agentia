import { Check, Send, X } from "lucide-react";
import { useMemo } from "react";
import { canSendReply, findForbiddenReplyTerms } from "../../lib/reply";
import type { ReplyDraft, WorldMessage } from "../../types/domain";
import { visibleWorldMessageBody } from "./messageIntegrity";

// This subtree only accepts world-visible chat state. Operator telemetry,
// run metadata, budgets, and private diagnostics must stay outside it.

type ReplyComposerProps = {
  draft: ReplyDraft;
  replyTarget?: WorldMessage;
  isSending: boolean;
  error?: string;
  onDraftChange: (draft: ReplyDraft) => void;
  onCancelReply: () => void;
  onSubmit: (draft: ReplyDraft) => Promise<void> | void;
};

export function ReplyComposer({
  draft,
  replyTarget,
  isSending,
  error,
  onDraftChange,
  onCancelReply,
  onSubmit,
}: ReplyComposerProps) {
  const forbiddenTerms = useMemo(() => findForbiddenReplyTerms(draft.body), [draft.body]);
  const sendable = canSendReply(draft) && !isSending;

  function updateBody(body: string) {
    if (body === draft.body) return;
    onDraftChange({ ...draft, body, boundaryAck: false });
  }

  return (
    <section className="reply-composer">
      {replyTarget ? (
        <div className="reply-composer__target">
          <div>
            <span>{replyTarget.bodyIntegrity === "legacy_truncated" ? "回复旧版本残片" : "回复这条"}</span>
            <p>{visibleWorldMessageBody(replyTarget)}</p>
          </div>
          <button type="button" aria-label="取消回复这条" title="取消回复这条" onClick={onCancelReply}><X size={15} /></button>
        </div>
      ) : null}
      <textarea
        className={forbiddenTerms.length > 0 ? "reply-composer__textarea--warn" : undefined}
        value={draft.body}
        onChange={(event) => updateBody(event.target.value)}
        onKeyDown={(event) => {
          if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return;
          event.preventDefault();
          if (sendable) void onSubmit(draft);
        }}
        placeholder="写点什么…"
        rows={3}
      />
      {forbiddenTerms.length > 0 ? (
        <div className="reply-warning" role="status">这些词不能带进她们的世界：{forbiddenTerms.join("、")}</div>
      ) : null}
      {error ? <div className="reply-composer__error" role="alert">{error}</div> : null}
      <div className="reply-composer__footer">
        <label className="boundary-check">
          <input
            type="checkbox"
            checked={draft.boundaryAck}
            onChange={(event) => onDraftChange({ ...draft, boundaryAck: event.target.checked })}
          />
          <Check size={13} />
          <span>只说世界里自然能说的话</span>
        </label>
        <div className="reply-composer__actions">
          <span>Shift + Enter 换行</span>
          <button type="button" className="primary-button" disabled={!sendable} onClick={() => onSubmit(draft)}>
            <Send size={16} />
            {isSending ? "发送中" : "发送"}
          </button>
        </div>
      </div>
    </section>
  );
}
