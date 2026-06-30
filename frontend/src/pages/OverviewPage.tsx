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
    <div className="page-stack">
      <RunSummaryHeader run={telemetry.activeRun} />
      <div className="resident-grid">
        {telemetry.residents.map((runtime) => {
          const budget = telemetry.budgets.find((item) => item.resident === runtime.resident);
          if (!budget) return null;
          return <ResidentCard runtime={runtime} budget={budget} key={runtime.resident} />;
        })}
      </div>
      <div className="two-column wide-left">
        <Panel title="System Telemetry" eyebrow="Observed metrics">
          <div className="chart-frame">
            <AreaMetricChart data={sparkSeries} />
          </div>
        </Panel>
        <Panel title="Observed Events" eyebrow="Live stream">
          <EventStream />
        </Panel>
      </div>
      <div className="two-column">
        <Panel title="Quota Remaining Matrix" eyebrow="Resource view">
          <BudgetMatrix budgets={telemetry.budgets} />
        </Panel>
        <Panel title="Inbox Preview" eyebrow="World-facing followups">
          <InboxPreview followups={telemetry.followups} onOpenThread={onOpenThread} />
        </Panel>
      </div>
      <Panel title="Watchpoints" eyebrow="Attention needed">
        <AlertStrip alerts={telemetry.alerts} />
      </Panel>
    </div>
  );
}
