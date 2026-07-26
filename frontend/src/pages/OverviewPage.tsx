import { Sparkline } from "../components/charts/Sparkline";
import { ResidentCard } from "../features/residents/ResidentCard";
import { residentLabel } from "../features/residents/residentTheme";
import { sleepDebtHint } from "../features/residents/residentSpeak";
import { compactElapsedLabel } from "../lib/duration";
import type { AlertItem, FollowupItem, OperatorTelemetry, ResidentId } from "../types/domain";

type OverviewPageProps = {
  telemetry: OperatorTelemetry;
  onOpenThread: (threadId: string) => void;
};

export function OverviewPage({ telemetry, onOpenThread }: OverviewPageProps) {
  const elapsedShort = compactElapsedLabel(telemetry.activeRun.elapsed);
  const pendingCount = telemetry.followups.filter((f) => f.status !== "closed").length;
  const oldest = telemetry.followups.reduce<FollowupItem | null>((acc, f) => {
    if (f.status === "closed") return acc;
    if (!acc) return f;
    return ageToMinutes(f.age) > ageToMinutes(acc.age) ? f : acc;
  }, null);
  const totalSpark = Math.round(
    telemetry.budgets.reduce((sum, b) => sum + b.sparkBalance, 0),
  );
  const houseOk =
    telemetry.activeRun.isLive &&
    telemetry.activeRun.budgetBlockedRuns === 0 &&
    telemetry.activeRun.residentsErrored === 0 &&
    telemetry.activeRun.transientBlocked === 0;
  const houseLabel = telemetry.activeRun.isLive ? (houseOk ? "全好" : "有事") : "空着";
  const heroSub = telemetry.activeRun.isLive
    ? `程林陪着住的这次观察，已经过了 ${elapsedShort}。她们在做各自的事，你在这里看着就好。`
    : "现在没有正在进行的观察。这里显示最近一次记录、她们留下的消息和房子的状态，别把它当成 live run。";
  const residentTitle = telemetry.activeRun.isLive ? "三个人现在的样子" : "三个人最近的状态";

  return (
    <div className="overview-layout">
      <div className="overview-main">
        <section className="hero">
          <p className="hero__kicker">观察窗</p>
          <h1 className="hero__title">Jade · Amber · Onyx</h1>
          <p className="hero__sub">{heroSub}</p>
        </section>

        <div className="section-title">
          <h3>{residentTitle}</h3>
          <span className="section-title__hint">{telemetry.activeRun.isLive ? "rolling 12s" : "没有 live run"}</span>
        </div>
        <div className="resident-grid">
          {telemetry.residents.map((runtime) => {
            const budget = telemetry.budgets.find((b) => b.resident === runtime.resident);
            if (!budget) return null;
            return <ResidentCard runtime={runtime} budget={budget} key={runtime.resident} />;
          })}
        </div>

        {(telemetry.alerts.length > 0 || sleepDebtHintsFrom(telemetry).length > 0) ? (
          <>
            <div className="section-title">
              <h3>值得留意的</h3>
              <span className="section-title__hint">她们自己会调</span>
            </div>
            <div className="watchlist">
              {telemetry.alerts.map((alert, index) => (
                <WatchItem alert={alert} key={`${alert.kind}-${alert.resident ?? "system"}-${index}`} />
              ))}
              {sleepDebtHintsFrom(telemetry).map((entry) => (
                <div className="watchlist__item" key={`sleep-${entry.resident}`}>
                  <span className="watchlist__dot watchlist__dot--info" />
                  <div className="watchlist__txt">
                    <div>
                      <span className="watchlist__subject">{residentLabel(entry.resident)} </span>
                      <span>{entry.hint}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </>
        ) : null}

        <div className="section-title">
          <h3>程林在等回话</h3>
          <span className="section-title__hint">{pendingCountHint(pendingCount, telemetry.followups)}</span>
        </div>
        <div className="pending-list">
          {telemetry.followups.map((f) => (
            <button
              key={f.targetId}
              type="button"
              className="pending-list__row"
              onClick={() => f.threadId ? onOpenThread(f.threadId) : undefined}
              disabled={!f.threadId}
            >
              <div className="pending-list__avatar">{residentLabel(f.resident).charAt(0)}</div>
              <div className="pending-list__who">
                <div className="pending-list__name">{residentLabel(f.resident)}</div>
                <div className="pending-list__kind">{f.kind}</div>
              </div>
              <div className="pending-list__snippet">{f.preview}</div>
              <div className="pending-list__age">{f.age}</div>
              <div className="pending-list__arrow">›</div>
            </button>
          ))}
        </div>
      </div>

      <aside className="overview-kpi-rail">
        <div className="kpi-card">
          <div className="kpi-card__label-row">
            <span>等程林回话</span>
            <span className="kpi-card__unit">条</span>
          </div>
          <div className="kpi-card__number">{pendingCount}</div>
          <div className="kpi-card__delta">
            {oldest ? `最老一条 ${oldest.age}` : "都回上了"}
          </div>
          <div className="kpi-card__spark">
            <Sparkline values={pendingSparkFromCount(pendingCount)} color="#1f2328" height={32} />
          </div>
        </div>

        <div className="kpi-card">
          <div className="kpi-card__label-row">
            <span>她们的 spark</span>
            <span className="kpi-card__unit">总量</span>
          </div>
          <div className="kpi-card__number">{totalSpark.toLocaleString()}</div>
          <div className="kpi-card__delta">
            Jade {Math.round(telemetry.budgets[0]?.sparkBalance ?? 0)} ·
            Amber {Math.round(telemetry.budgets[1]?.sparkBalance ?? 0)} ·
            Onyx {Math.round(telemetry.budgets[2]?.sparkBalance ?? 0)}
          </div>
          <div className="kpi-card__spark">
            <Sparkline values={[10, 12, 15, 14, 18, 20, 22, 25, 28, 26, 30, 32, 30]} color="#1f2328" height={32} />
          </div>
        </div>

        <div className="kpi-card">
          <div className="kpi-card__label-row">
            <span>房子情况</span>
            <span className="kpi-card__unit">3 项</span>
          </div>
          <div className={`kpi-card__number ${houseOk ? "kpi-card__number--good" : ""}`}>
            {houseLabel}
          </div>
          <div className="kpi-card__delta">
            {telemetry.activeRun.isLive ? "block 0 · 错 0 · transient 0" : "当前没有 live 观察"}
          </div>
        </div>

        <div className="kpi-card">
          <div className="kpi-card__label-row">
            <span>她们攒的碎念</span>
            <span className="kpi-card__unit">条</span>
          </div>
          <div className="kpi-card__number">{memoryReviewCount(telemetry.system.memory)}</div>
          <div className="kpi-card__delta">
            {memoryReviewHint(memoryReviewCount(telemetry.system.memory))}
          </div>
        </div>

        <div className="session-panel">
          <div className="session-panel__label">{telemetry.activeRun.isLive ? "这次观察" : "最近一次观察"}</div>
          <div className="session-panel__val">{elapsedShort}</div>
          <div className="session-panel__sub">
            {telemetry.activeRun.isLive
              ? liveTargetText(telemetry.activeRun)
              : `${runStatusText(telemetry.activeRun.status)} · 更新 ${isoTimeToHm(telemetry.activeRun.updatedAt)}`}
          </div>
        </div>
      </aside>
    </div>
  );
}

function runStatusText(status: OperatorTelemetry["activeRun"]["status"]): string {
  if (status === "finished") return "已结束";
  if (status === "failed") return "失败";
  if (status === "paused") return "暂停";
  if (status === "abandoned") return "旧记录";
  return "记录";
}

function liveTargetText(run: OperatorTelemetry["activeRun"]): string {
  const target = run.targetDuration === "未标明" ? "目标未标明" : `目标 ${run.targetDuration}`;
  const end = isoTimeToHm(run.expectedEndAt);
  return end ? `${target} · 至 ${end}` : target;
}

function WatchItem({ alert }: { alert: AlertItem }) {
  const variant = severityToDot(alert.severity);
  return (
    <div className="watchlist__item">
      <span className={`watchlist__dot watchlist__dot--${variant}`} />
      <div className="watchlist__txt">
        <div>
          {alert.resident ? <span className="watchlist__subject">{residentLabel(alert.resident)} </span> : null}
          <span>{alert.message}</span>
        </div>
      </div>
    </div>
  );
}

function severityToDot(severity: AlertItem["severity"]): "warn" | "info" | "good" {
  if (severity === "p0" || severity === "p1") return "warn";
  if (severity === "info") return "good";
  return "info";
}

function ageToMinutes(age: string): number {
  // "24m", "47m", "2h22m"
  let minutes = 0;
  const hourMatch = /(\d+)\s*h/.exec(age);
  if (hourMatch) minutes += Number(hourMatch[1]) * 60;
  const minMatch = /(\d+)\s*m/.exec(age);
  if (minMatch) minutes += Number(minMatch[1]);
  return minutes;
}

function pendingCountHint(count: number, followups: FollowupItem[]): string {
  if (count === 0) return "都回上了";
  const urgent = followups.some((f) => f.priority === "high" || f.priority === "urgent");
  return urgent ? `${count} 条 · 有要紧的` : `${count} 条 · 都不急`;
}

function pendingSparkFromCount(count: number): number[] {
  // gentle rising line ending at count
  const end = Math.max(1, count);
  const series: number[] = [];
  for (let i = 0; i < 12; i += 1) {
    series.push(Math.max(0, end - (12 - i) * 0.3 + Math.sin(i) * 0.4));
  }
  return series;
}

function isoTimeToHm(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

function memoryReviewCount(memoryLines: readonly string[]): number {
  // The list carries operator-facing memory metadata; the "过一眼" / "review"
  // line is the one host asked to surface. Fall back to the first line's first
  // number if the specific line isn't found. Zero if nothing parses.
  const target = memoryLines.find((line) => line.includes("过一眼") || /review/i.test(line));
  return firstIntFrom(target ?? memoryLines[0] ?? "");
}

function firstIntFrom(text: string): number {
  const match = /(\d[\d,]*)/.exec(text);
  if (!match) return 0;
  return Number(match[1].replace(/,/g, "")) || 0;
}

const memoryHintVariants = {
  zero: ["她们最近没多想", "碎念很少，比较沉静"],
  few: ["慢慢翻，她们没催", "有几条你什么时候看都行", "她们没在等你答话"],
  some: ["她们攒了些想让你过一眼的", "碎念在攒着，找个空翻翻", "有点想让你看看的东西了"],
  many: ["她们最近有点话唠", "一箩筐想让你看的", "这两天她们脑子挺满的"],
} as const;

function memoryReviewHint(count: number): string {
  // Rotate by day-of-week so the hint changes over time without flickering
  // between polls. Same day → same variant; a new day → likely a new one.
  const day = new Date().getDate();
  const tier = count === 0 ? "zero" : count < 50 ? "few" : count < 200 ? "some" : "many";
  const list = memoryHintVariants[tier];
  return list[day % list.length];
}

function sleepDebtHintsFrom(telemetry: OperatorTelemetry): Array<{ resident: ResidentId; hint: string }> {
  const out: Array<{ resident: ResidentId; hint: string }> = [];
  for (const budget of telemetry.budgets) {
    if (!budget.sleep) continue;
    const hint = sleepDebtHint(budget.sleep.debtHours, budget.resident);
    if (hint) out.push({ resident: budget.resident, hint });
  }
  return out;
}
