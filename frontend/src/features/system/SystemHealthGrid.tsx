import { Cpu, Database, Server, Users, Zap } from "lucide-react";
import type { OperatorTelemetry } from "../../types/domain";

type GroupKey = keyof OperatorTelemetry["system"];

const groups: Array<{ key: GroupKey; title: string; hint: string; Icon: typeof Server }> = [
  { key: "capacity", title: "机器", hint: "本机资源余量", Icon: Cpu },
  { key: "vms", title: "住户虚机", hint: "jade / amber / onyx 运行状态", Icon: Server },
  { key: "providers", title: "外面接口", hint: "模型 provider 的健康", Icon: Users },
  { key: "memory", title: "长期记忆", hint: "程林要过的、host 要盯的", Icon: Database },
  { key: "economy", title: "spark 账房", hint: "消耗与缓存", Icon: Zap },
];

export function SystemHealthGrid({ system }: { system: OperatorTelemetry["system"] }) {
  return (
    <div className="system-grid">
      {groups.map(({ key, title, hint, Icon }) => {
        const lines = system[key];
        return (
          <section className="system-card" key={key}>
            <header className="system-card__head">
              <div className="system-card__title">
                <Icon size={14} strokeWidth={1.6} />
                <span>{title}</span>
              </div>
              <span className="system-card__hint">{hint}</span>
            </header>
            <ul className="system-card__list">
              {lines.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}
