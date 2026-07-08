import { Sparkline } from "../../components/charts/Sparkline";
import type { ResidentBudget, ResidentRuntime } from "../../types/domain";
import {
  describeDoing,
  fatigueMoodFromLevel,
  fatigueMoodLabel,
  pressureToQuotaTone,
  residentStateLabel,
} from "./residentSpeak";
import { residentLabel } from "./residentTheme";

type ResidentCardProps = {
  runtime: ResidentRuntime;
  budget: ResidentBudget;
  spark?: number[];
};

const defaultSpark: Record<string, number[]> = {
  jade: [32, 28, 30, 22, 24, 18, 20, 14, 17, 12, 15],
  amber: [15, 18, 16, 14, 20, 25, 30, 32, 34, 34, 35],
  onyx: [22, 20, 25, 26, 24, 28, 30, 28, 32, 30, 33],
};

export function ResidentCard({ runtime, budget, spark }: ResidentCardProps) {
  const state = residentStateLabel(runtime.status);
  const doing = describeDoing(runtime);
  const quotaTone = pressureToQuotaTone(budget.pressure);
  // 6h is observation-only in the new model; the "tightest" gate is always
  // day or week. Fall back to day if a legacy budget still reports "6h".
  const gateLayer: "day" | "week" =
    budget.tightestLayer === "week" ? "week" : "day";
  const remaining = budget.remaining[gateLayer];
  const layerLabel = gateLayer === "day" ? "今日" : "本周";
  const burn = budget.sixHourBurn ?? spark ?? defaultSpark[runtime.resident] ?? [15, 18, 22, 20, 24, 22, 26];
  const mood = budget.fatigue?.mood ?? (typeof budget.fatigue?.level === "number" ? fatigueMoodFromLevel(budget.fatigue.level) : null);

  return (
    <article className="resident-card">
      <div className="resident-card__head">
        <h3 className="resident-card__name">{residentLabel(runtime.resident)}</h3>
        <span className={`resident-state resident-state--${state.variant}`}>{state.text}</span>
      </div>

      <p className="resident-card__doing">
        {doing.verb ? (
          <>
            {runtime.status === "sleeping" ? "" : "正在"}
            <span className="resident-card__doing-verb">{doing.verb}</span>
            {doing.suffix ? ` ${doing.suffix}` : ""}
          </>
        ) : (
          "—"
        )}
      </p>

      {mood ? (
        <div className="resident-card__mood">
          <span className={`fatigue-dot fatigue-dot--${mood}`} />
          <span>{fatigueMoodLabel(mood)}</span>
        </div>
      ) : null}

      <div className="resident-card__metrics">
        <div>
          <div className="resident-card__metric-label">精力（spark）</div>
          <div className="resident-card__metric-val">{Math.round(budget.sparkBalance)}</div>
        </div>
        <div>
          <div className="resident-card__metric-label">
            {runtime.status === "sleeping" ? "最近动静" : "最近说话"}
          </div>
          <div className="resident-card__metric-val">{runtime.lastUpdateAge}</div>
        </div>
      </div>

      <div className="resident-card__spark">
        <Sparkline values={burn} color="#1f2328" height={36} />
      </div>

      <div className="resident-card__quota">
        <div className="resident-card__quota-labels">
          <span>{layerLabel}额度</span>
          <span className="resident-card__quota-num">剩 {Math.round(remaining)}%</span>
        </div>
        <div className="progress-bar">
          <div
            className={`progress-bar__fill progress-bar__fill--${quotaTone}`}
            style={{ width: `${Math.max(0, Math.min(100, remaining))}%` }}
          />
        </div>
      </div>
    </article>
  );
}
