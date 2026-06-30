import { ProgressBar } from "../../components/ui/ProgressBar";
import { formatPercent, formatSpark } from "../../lib/domain/formatting";
import { quotaToneClass } from "../../lib/domain/severity";
import type { ResidentBudget } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";

export function BudgetMatrix({ budgets }: { budgets: ResidentBudget[] }) {
  return (
    <div className="budget-table">
      <div className="budget-table__head">
        <span>Resident</span>
        <span>6h</span>
        <span>Day</span>
        <span>Week</span>
        <span>Spark</span>
      </div>
      {budgets.map((budget) => (
        <div className="budget-table__row" key={budget.resident}>
          <strong>{residentLabel(budget.resident)}</strong>
          {(["6h", "day", "week"] as const).map((layer) => (
            <div className="budget-cell" key={layer}>
              <span>{formatPercent(budget.remaining[layer])}</span>
              <ProgressBar value={budget.remaining[layer]} toneClass={quotaToneClass(budget.remaining[layer])} />
            </div>
          ))}
          <span>{formatSpark(budget.sparkBalance)}</span>
        </div>
      ))}
    </div>
  );
}
