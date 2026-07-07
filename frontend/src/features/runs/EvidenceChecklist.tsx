import { CheckCircle2, CircleDashed, XCircle } from "lucide-react";
import type { EvidenceItem } from "../../types/domain";

const stateLabel: Record<EvidenceItem["state"], string> = {
  ready: "就绪",
  pending: "等待",
  failed: "未通过",
};

export function EvidenceChecklist({ evidence }: { evidence: EvidenceItem[] }) {
  return (
    <ul className="evidence-list">
      {evidence.map((item) => {
        const Icon = item.state === "ready" ? CheckCircle2 : item.state === "failed" ? XCircle : CircleDashed;
        return (
          <li className={`evidence-row evidence-row--${item.state}`} key={item.label}>
            <Icon size={17} strokeWidth={1.6} />
            <span className="evidence-row__label">{translateEvidenceLabel(item.label)}</span>
            <span className="evidence-row__state">{stateLabel[item.state]}</span>
          </li>
        );
      })}
    </ul>
  );
}

const evidenceLabelMap: Record<string, string> = {
  "run still updating": "这轮运行还在更新",
  "all residents alive": "三位住户都还在",
  "budget blocked = 0": "额度没卡住任何一位",
  "world-only Chenglin replies": "世界外只有程林回话",
  "24h duration complete": "24 小时长测走完",
};

function translateEvidenceLabel(label: string): string {
  return evidenceLabelMap[label] ?? label;
}
