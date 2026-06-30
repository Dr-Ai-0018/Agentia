import { Panel } from "../components/ui/Panel";
import { AreaMetricChart } from "../components/charts/AreaMetricChart";
import { sparkSeries } from "../data/mockConsole";
import { AlertStrip } from "../features/overview/AlertStrip";
import { InboxPreview } from "../features/overview/InboxPreview";
import { RunSummaryHeader } from "../features/overview/RunSummaryHeader";
import { ResidentCard } from "../features/residents/ResidentCard";
import { BudgetMatrix } from "../features/telemetry/BudgetMatrix";
import { EventStream } from "../features/telemetry/EventStream";
import type { OperatorTelemetry } from "../types/domain";

type OverviewPageProps = {
  telemetry: OperatorTelemetry;
  onOpenThread: (threadId: string) => void;
};

export function OverviewPage({ telemetry, onOpenThread }: OverviewPageProps) {
  return (
    <div className="overview-grid">
      <div className="grid-hero">
        <RunSummaryHeader run={telemetry.activeRun} />
      </div>
      <div className="resident-grid grid-residents">
        {telemetry.residents.map((runtime) => {
          const budget = telemetry.budgets.find((item) => item.resident === runtime.resident);
          if (!budget) return null;
          return <ResidentCard runtime={runtime} budget={budget} key={runtime.resident} />;
        })}
      </div>
      <div className="grid-telemetry">
        <Panel title="System Telemetry" eyebrow="Observed metrics">
          <div className="chart-frame">
            <AreaMetricChart data={sparkSeries} />
          </div>
        </Panel>
      </div>
      <div className="grid-events">
        <Panel title="Observed Events" eyebrow="Live stream">
          <EventStream />
        </Panel>
      </div>
      <div className="grid-budget">
        <Panel title="Quota Remaining Matrix" eyebrow="Resource view">
          <BudgetMatrix budgets={telemetry.budgets} />
        </Panel>
      </div>
      <div className="grid-inbox">
        <Panel title="Inbox Preview" eyebrow="World-facing followups">
          <InboxPreview followups={telemetry.followups} onOpenThread={onOpenThread} />
        </Panel>
      </div>
      <div className="grid-alerts">
        <Panel title="Watchpoints" eyebrow="Attention needed">
          <AlertStrip alerts={telemetry.alerts} />
        </Panel>
      </div>
    </div>
  );
}
