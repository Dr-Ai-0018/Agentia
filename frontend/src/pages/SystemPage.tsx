import { SystemHealthGrid } from "../features/system/SystemHealthGrid";
import type { OperatorTelemetry } from "../types/domain";

export function SystemPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  return (
    <div className="page-stack">
      <header className="page-header">
        <p className="eyebrow">Operator-only health</p>
        <h1>System</h1>
        <p>Host capacity, provider health, cache/memory inspection, and economy signals. These facts never enter Chenglin replies.</p>
      </header>
      <SystemHealthGrid system={telemetry.system} />
    </div>
  );
}
