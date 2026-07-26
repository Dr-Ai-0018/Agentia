import { NotebookPen } from "lucide-react";

// This notice reads as a private note to Chenglin, not an operator instruction.
// Framing matters: the operator overhears rather than dictates. Do not rewrite
// this to address the operator — the whole page's mental model depends on it.
export function ReplyBoundaryNotice() {
  return (
    <div className="reply-boundary">
      <NotebookPen size={16} />
      <div>
        <strong>给程林的一张小便签</strong>
        <p>
          这里说的话会进世界。用你自然能说出的那种就好——
          屋外页面、记录编号、通行口令、系统缓存、额度账本、检查记录这些口径不是你会用的，别写。
        </p>
      </div>
    </div>
  );
}
