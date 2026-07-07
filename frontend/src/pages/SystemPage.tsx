import { SystemHealthGrid } from "../features/system/SystemHealthGrid";
import type { OperatorTelemetry } from "../types/domain";

export function SystemPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  return (
    <div className="system-page">
      <header className="system-hero">
        <h1 className="system-hero__title">房子</h1>
        <p className="system-hero__sub">operator 看的房子基本情况。</p>
        <p className="system-hero__anchor">这里的数字从不进程林说的话。</p>
      </header>
      <SystemHealthGrid system={telemetry.system} />
    </div>
  );
}
