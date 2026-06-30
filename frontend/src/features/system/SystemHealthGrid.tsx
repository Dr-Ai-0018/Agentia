import { Database, HardDrive, Server, Wifi, Zap } from "lucide-react";
import { Panel } from "../../components/ui/Panel";
import type { OperatorTelemetry } from "../../types/domain";

const groups = [
  ["capacity", Server],
  ["vms", HardDrive],
  ["providers", Wifi],
  ["memory", Database],
  ["economy", Zap],
] as const;

export function SystemHealthGrid({ system }: { system: OperatorTelemetry["system"] }) {
  return (
    <div className="system-grid">
      {groups.map(([key, Icon]) => (
        <Panel title={key} key={key} action={<Icon size={17} />}>
          <ul className="system-list">
            {system[key].map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </Panel>
      ))}
    </div>
  );
}
