import { ShieldCheck } from "lucide-react";

export function ReplyBoundaryNotice() {
  return (
    <div className="reply-boundary">
      <ShieldCheck size={16} />
      <div>
        <strong>World-safe</strong>
        <p>Reply naturally as Chenglin. Keep dashboard facts, run ids, token/cache details, budget audit data, hidden test rules, and operator-only observations private.</p>
      </div>
    </div>
  );
}
