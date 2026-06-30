import { Server } from "lucide-react";
import type { CSSProperties } from "react";
import { Badge } from "../../components/ui/Badge";
import { ProgressBar } from "../../components/ui/ProgressBar";
import { StatusDot } from "../../components/ui/StatusDot";
import { formatCompactNumber, formatPercent, formatSpark } from "../../lib/domain/formatting";
import { pressureTone, quotaToneClass } from "../../lib/domain/severity";
import type { ResidentBudget, ResidentRuntime } from "../../types/domain";
import { residentLabel, residentTheme, statusTone } from "./residentTheme";

type ResidentCardProps = {
  runtime: ResidentRuntime;
  budget: ResidentBudget;
};

export function ResidentCard({ runtime, budget }: ResidentCardProps) {
  const theme = residentTheme(runtime.resident);
  const tone = statusTone(runtime.status);

  return (
    <article className="resident-card" style={{ "--resident": theme.color, "--resident-soft": theme.soft } as CSSProperties}>
      <header className="resident-card__header">
        <div className="resident-card__identity">
          <div className="resident-card__icon">
            <Server size={20} />
          </div>
          <div>
            <h3>{residentLabel(runtime.resident)}</h3>
            <div className="inline-status">
              <StatusDot tone={tone} pulse={runtime.status === "running"} />
              <span>{runtime.status}</span>
            </div>
          </div>
        </div>
        <div className="round-chip">#{runtime.round}</div>
      </header>

      <div className="metric-grid two">
        <div className="metric-cell">
          <span>Spark balance</span>
          <strong>{formatSpark(budget.sparkBalance)}</strong>
        </div>
        <div className="metric-cell">
          <span>Current action</span>
          <strong>{runtime.lastAction}</strong>
          <small>{runtime.phase}</small>
        </div>
        <div className="metric-cell">
          <span>Last update</span>
          <strong>{runtime.lastUpdateAge}</strong>
        </div>
        <div className="metric-cell">
          <span>In flight</span>
          <strong>{runtime.inFlight}</strong>
        </div>
      </div>

      <div className="resident-card__quota">
        <div>
          <span>Tightest quota</span>
          <Badge tone={pressureTone(budget.pressure).replace("tone-", "") as "good" | "warning" | "danger" | "info" | "muted"}>
            {budget.tightestLayer} / {budget.pressure}
          </Badge>
        </div>
        <strong>{formatPercent(budget.remaining[budget.tightestLayer])} remaining</strong>
      </div>
      <ProgressBar value={budget.remaining[budget.tightestLayer]} toneClass={quotaToneClass(budget.remaining[budget.tightestLayer])} />
      <div className="resident-card__tokens">
        <span>Input {formatCompactNumber(runtime.totalInputTokens)}</span>
        <span>Cached {formatCompactNumber(runtime.totalCachedTokens)}</span>
        <span>Output {formatCompactNumber(runtime.totalOutputTokens)}</span>
      </div>
    </article>
  );
}
