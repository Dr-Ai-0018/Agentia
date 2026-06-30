import { Panel } from "../components/ui/Panel";
import { EvidenceChecklist } from "../features/runs/EvidenceChecklist";
import { RunRegistryTable } from "../features/runs/RunRegistryTable";
import type { OperatorTelemetry } from "../types/domain";

export function RunsPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  return (
    <div className="page-stack">
      <header className="page-header">
        <p className="eyebrow">Run registry</p>
        <h1>Runs</h1>
        <p>Active and recent orchestrator runs, lineage readiness, and 24h evidence state.</p>
      </header>
      <Panel title="Run History" eyebrow="Latest first">
        <RunRegistryTable runs={telemetry.runs} />
      </Panel>
      <Panel title="Evidence Checklist" eyebrow="Pre-release gate">
        <EvidenceChecklist evidence={telemetry.evidence} />
      </Panel>
    </div>
  );
}
