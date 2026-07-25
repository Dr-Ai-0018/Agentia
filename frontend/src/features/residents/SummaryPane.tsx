import { FileText, Layers } from "lucide-react";
import { useState } from "react";
import type { SummaryPane as SummaryPaneData, SummaryPaneEvidenceRef } from "../../types/domain";

// Operator-only display of the older-round continuity pane. Wrapped
// in a collapsible so dev-mode operators can scan the resident's own
// retrospection prose. Never shown to residents — this component is only
// mounted inside ResidentsPage when useDevMode() is true.

const EVIDENCE_KIND_LABEL: Record<SummaryPaneEvidenceRef["kind"], string> = {
  round: "轮次",
  note: "note",
  guest_artifact: "guest 输出",
};

export function SummaryPane({ pane }: { pane: SummaryPaneData }) {
  const [expanded, setExpanded] = useState(true);
  return (
    <div className="summary-pane">
      <button
        type="button"
        className="summary-pane__head"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <span className="summary-pane__caret">{expanded ? "▾" : "▸"}</span>
        <Layers size={12} strokeWidth={1.8} />
        <span className="summary-pane__title">她自己的梳理</span>
        <span className="summary-pane__meta">
          吞了 {pane.roundsAbsorbed} 轮 · 约 {formatTokens(pane.approxTokens)} · {formatAgo(pane.updatedAt)}
        </span>
      </button>
      {expanded ? (
        <div className="summary-pane__body">
          <p className="summary-pane__text">{pane.text}</p>
          {pane.evidenceRefs && pane.evidenceRefs.length > 0 ? (
            <div className="summary-pane__refs">
              <div className="summary-pane__refs-label">
                <FileText size={11} strokeWidth={1.8} />
                <span>引用到的证据</span>
              </div>
              <ul className="summary-pane__refs-list">
                {pane.evidenceRefs.map((ref, index) => (
                  <li key={`${ref.kind}-${ref.ref}-${index}`}>
                    <span className="summary-pane__ref-kind">{EVIDENCE_KIND_LABEL[ref.kind]}</span>
                    <span className="summary-pane__ref-body">
                      <code>{ref.ref}</code>
                      {ref.rounds && ref.rounds.length > 0 ? (
                        <span className="summary-pane__ref-rounds"> · 轮 {ref.rounds.join(", ")}</span>
                      ) : null}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <p className="summary-pane__caveat">
            这段只在看守长跑时显示，不会进入世界里的对话。
          </p>
        </div>
      ) : null}
    </div>
  );
}

function formatTokens(n: number): string {
  if (n >= 1000) return `${Math.round(n / 100) / 10}K`;
  return String(n);
}

function formatAgo(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm} 更新`;
}
