import { useState } from "react";
import { Sparkline } from "../components/charts/Sparkline";
import { dataMode } from "../lib/api/client";
import { useDevMode } from "../lib/devMode";
import { eventStream } from "../data/mockConsole";
import {
  describeDoing,
  fatigueMoodFromLevel,
  fatigueMoodLabel,
  forecastFromRemainingPct,
  layerWindowLabel,
  pressureToQuotaTone,
  residentStateLabel,
  sleepDebtHint,
  sleepDepthLabel,
  verbLabel,
} from "../features/residents/residentSpeak";
import { SummaryPane } from "../features/residents/SummaryPane";
import { residentLabel } from "../features/residents/residentTheme";
import type { OperatorTelemetry, ResidentBudget, ResidentId, ResidentRuntime } from "../types/domain";

const sparkFallback: Record<ResidentId, number[]> = {
  jade: [22, 20, 18, 22, 24, 18, 16, 18, 20, 22, 19, 17],
  amber: [15, 18, 16, 14, 20, 25, 30, 32, 28, 22, 18, 14],
  onyx: [26, 28, 30, 28, 32, 30, 33, 30, 28, 30, 32, 28],
};

// Only day / week are real gates in the 2026-07-07 quota model. 6h is
// rendered as an observation sparkline in its own section above.
const GATE_LAYERS = ["day", "week"] as const;

export function ResidentsPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  const [active, setActive] = useState<ResidentId>(telemetry.residents[0]?.resident ?? "jade");

  const runtime = telemetry.residents.find((r) => r.resident === active);
  const budget = telemetry.budgets.find((b) => b.resident === active);
  if (!runtime || !budget) return null;

  return (
    <div className="residents-page">
      <div className="resident-tabs" role="tablist" aria-label="住户">
        {telemetry.residents.map((r) => {
          const state = residentStateLabel(r.status);
          const isActive = r.resident === active;
          return (
            <button
              key={r.resident}
              type="button"
              role="tab"
              aria-selected={isActive}
              className={`resident-tab ${isActive ? "resident-tab--active" : ""}`}
              onClick={() => setActive(r.resident)}
            >
              <span className="resident-tab__name">{residentLabel(r.resident)}</span>
              <span className={`resident-tab__state resident-tab__state--${state.variant}`}>{state.text}</span>
            </button>
          );
        })}
      </div>

      <ResidentDetail runtime={runtime} budget={budget} />
    </div>
  );
}

function ResidentDetail({ runtime, budget }: { runtime: ResidentRuntime; budget: ResidentBudget }) {
  const [showDebug, setShowDebug] = useState(false);
  const [devMode] = useDevMode();
  const state = residentStateLabel(runtime.status);
  const doing = describeDoing(runtime);
  const burn = budget.sixHourBurn ?? sparkFallback[runtime.resident] ?? [15, 18, 22, 20, 24, 22, 26];
  const peak = Math.round(Math.max(...burn));
  const mean = Math.round(burn.reduce((sum, v) => sum + v, 0) / burn.length);

  const mood = budget.fatigue?.mood ?? (typeof budget.fatigue?.level === "number" ? fatigueMoodFromLevel(budget.fatigue.level) : null);
  const sleepDepth = budget.sleep?.depth ?? "awake";
  const debtHint = budget.sleep ? sleepDebtHint(budget.sleep.debtHours, runtime.resident) : null;

  const recent = recentActionsFor(runtime.resident);

  return (
    <article className="resident-detail">
      <header className="resident-detail__hero">
        <div>
          <h1 className="resident-detail__name">{residentLabel(runtime.resident)}</h1>
          <p className="resident-detail__doing">
            {doing.verb ? (
              <>
                {runtime.status === "sleeping" ? "" : "她正在"}
                <span className="resident-detail__doing-verb">{doing.verb}</span>
                {doing.suffix ? ` ${doing.suffix}` : ""}
              </>
            ) : (
              "—"
            )}
          </p>
        </div>
        <span className={`resident-state resident-state--${state.variant}`}>{state.text}</span>
      </header>

      {mood || budget.sleep ? (
        <section className="resident-section">
          <div className="section-title">
            <h3>状态</h3>
            <span className="section-title__hint">她自己的感受</span>
          </div>
          <div className="fatigue-card">
            {mood ? (
              <div className="fatigue-row">
                <span className={`fatigue-dot fatigue-dot--${mood}`} />
                <span className="fatigue-row__mood">{fatigueMoodLabel(mood, runtime.resident)}</span>
              </div>
            ) : null}
            {budget.sleep && sleepDepth !== "awake" ? (
              <div className="fatigue-row fatigue-row--sub">
                <span className="fatigue-row__label">睡眠</span>
                <span className="fatigue-row__val">{sleepDepthLabel(sleepDepth, runtime.resident)}</span>
              </div>
            ) : null}
            {debtHint ? (
              <div className="fatigue-row fatigue-row--warn">
                <span className="fatigue-row__label">睡眠债</span>
                <span className="fatigue-row__val">{debtHint}</span>
              </div>
            ) : null}
          </div>
        </section>
      ) : null}

      <section className="resident-section">
        <div className="section-title">
          <h3>最近 6h 节奏</h3>
          <span className="section-title__hint">峰值 {peak} · 均值 {mean} · 只观察，不挡人</span>
        </div>
        <div className="resident-detail__spark">
          <Sparkline values={burn} color="#1f2328" height={56} />
        </div>
      </section>

      <section className="resident-section">
        <div className="section-title">
          <h3>额度</h3>
          <span className="section-title__hint">今日 · 本周 = 真挡</span>
        </div>
        {budget.workAllowedNow === false ? (
          <div className="quota-gate">
            <strong>她现在被挡住了。</strong>
            <span>额度用完了，等旧记账过期腾出空间才能继续。</span>
          </div>
        ) : null}
        <div className="quota-layers">
          {GATE_LAYERS.map((layer) => {
            const pct = budget.remaining[layer];
            const isTightest = layer === budget.tightestLayer;
            const tone = isTightest ? pressureToQuotaTone(budget.pressure) : "ok";
            return (
              <div className={`quota-row ${isTightest ? "quota-row--tightest" : ""}`} key={layer}>
                <div className="quota-row__label">
                  {layerWindowLabel(layer)}
                  {isTightest ? <span className="quota-row__pin">最紧</span> : null}
                </div>
                <div className="quota-row__bar">
                  <div
                    className={`progress-bar__fill progress-bar__fill--${tone}`}
                    style={{ width: `${Math.max(0, Math.min(100, pct))}%` }}
                  />
                </div>
                <div className="quota-row__num">剩 {Math.round(pct)}%</div>
                <div className="quota-row__forecast">按当前速率 {forecastFromRemainingPct(pct, layer)}</div>
              </div>
            );
          })}
        </div>
      </section>

      <section className="resident-section">
        <div className="section-title">
          <h3>她刚做的</h3>
          <span className="section-title__hint">{recent.length > 0 ? `最近 ${recent.length} 条` : "还没有事件"}</span>
        </div>
        {recent.length > 0 ? (
          <ul className="action-log">
            {recent.map((row, index) => (
              <li key={`${row.time}-${index}`} className="action-log__row">
                <span className="action-log__time">{row.time}</span>
                <span className="action-log__verb">{row.verb}</span>
                <span className="action-log__body">{row.note}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="action-log__empty">这一小段时间还没落笔。</p>
        )}
      </section>

      {devMode && runtime.summaryPane ? (
        <section className="resident-section resident-section--dev">
          <div className="section-title">
            <h3>旧事梳理</h3>
            <span className="section-title__hint">只在开发模式显示</span>
          </div>
          <SummaryPane pane={runtime.summaryPane} />
        </section>
      ) : null}

      <section className="resident-section resident-section--debug">
        <button
          type="button"
          className="debug-drawer__toggle"
          onClick={() => setShowDebug((v) => !v)}
          aria-expanded={showDebug}
        >
          <span>{showDebug ? "▾" : "▸"}</span>
          <span>调试用：token 账簿</span>
          <span className="debug-drawer__hint">跟世界内不相关</span>
        </button>
        {showDebug ? (
          <div className="debug-drawer__body">
            <div className="debug-drawer__grid">
              <div>
                <div className="debug-drawer__label">输入</div>
                <div className="debug-drawer__val">{runtime.totalInputTokens.toLocaleString()}</div>
              </div>
              <div>
                <div className="debug-drawer__label">缓存</div>
                <div className="debug-drawer__val">{runtime.totalCachedTokens.toLocaleString()}</div>
              </div>
              <div>
                <div className="debug-drawer__label">输出</div>
                <div className="debug-drawer__val">{runtime.totalOutputTokens.toLocaleString()}</div>
              </div>
              <div>
                <div className="debug-drawer__label">回合</div>
                <div className="debug-drawer__val">{runtime.round}</div>
              </div>
            </div>
          </div>
        ) : null}
      </section>
    </article>
  );
}

type ActionRow = { time: string; verb: string; note: string };

// TODO: backend 出 events endpoint 后走 arenaApi.listEvents(resident)；
// 现在只在 mock 模式下拿 fixture。
function recentActionsFor(resident: ResidentId): ActionRow[] {
  if (dataMode !== "mock") return [];
  const upper = resident.toUpperCase();
  return eventStream
    .filter((e) => e.source === upper)
    .slice(-8)
    .reverse()
    .map((e) => {
      const [action, ...rest] = e.body.split(":");
      const note = rest.join(":").trim();
      return {
        time: e.time.slice(0, 5),
        verb: verbLabel(action.trim()),
        note: note || action.trim(),
      };
    });
}
