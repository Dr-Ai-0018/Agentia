import { MessageSquare } from "lucide-react";
import type { FollowupItem } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";

type InboxPreviewProps = {
  followups: FollowupItem[];
  onOpenThread: (threadId: string) => void;
};

export function InboxPreview({ followups, onOpenThread }: InboxPreviewProps) {
  return (
    <div className="inbox-list">
      {followups.map((item) => (
        <button className="inbox-row" key={item.targetId} onClick={() => onOpenThread(item.threadId)}>
          <MessageSquare size={16} />
          <span className="inbox-row__meta">
            <strong>{residentLabel(item.resident)}</strong>
            <small>{item.kind} / {item.age} / {item.status}</small>
          </span>
          <span className="inbox-row__preview">{item.preview}</span>
        </button>
      ))}
    </div>
  );
}
