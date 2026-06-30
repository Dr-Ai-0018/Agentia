import { Panel } from "../components/ui/Panel";
import { ResidentCard } from "../features/residents/ResidentCard";
import { BudgetMatrix } from "../features/telemetry/BudgetMatrix";
import type { OperatorTelemetry } from "../types/domain";

export function ResidentsPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  return (
    <div className="page-stack">
      <header className="page-header">
        <p className="eyebrow">Resident Detail</p>
        <h1>Residents</h1>
        <p>Runtime, pacing, quota pressure, and current work posture for Jade, Amber, and Onyx.</p>
      </header>
      <div className="resident-grid">
        {telemetry.residents.map((runtime) => {
          const budget = telemetry.budgets.find((item) => item.resident === runtime.resident);
          if (!budget) return null;
          return <ResidentCard runtime={runtime} budget={budget} key={runtime.resident} />;
        })}
      </div>
      <Panel title="Budget Detail" eyebrow="6h / day / week">
        <BudgetMatrix budgets={telemetry.budgets} />
      </Panel>
    </div>
  );
}
