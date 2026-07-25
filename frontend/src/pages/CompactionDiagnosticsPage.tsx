import { AlertTriangle, Layers, RefreshCw, ShieldAlert, Zap } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { residentAccent } from "../data/mockConsole";
import { arenaApi } from "../lib/api/client";
import type {
  CompactionDiagnostics,
  CompactionEvent,
  CompactionOutcome,
  CompactionRunSummary,
  CompactionTriggerReason,
} from "../types/domain";

const TRIGGER_LABEL: Record<CompactionTriggerReason, string> = {
  preflight_measured: "开跑前实测",
  acceptance_microcompact: "验收前收摊",
  reactive_overflow: "撞墙后自救",
  manual: "手动",
};

const OUTCOME_LABEL: Record<CompactionOutcome, string> = {
  summarized: "摘要完成",
  silent_trim: "静默裁剪",
  guard_rejected: "词表拦下",
  failed: "调用失败",
};

const OUTCOME_TONE: Record<CompactionOutcome, string> = {
  summarized: "ok",
  silent_trim: "info",
  guard_rejected: "warn",
  failed: "warn",
};

export function CompactionDiagnosticsPage() {
  const [diagnostics, setDiagnostics] = useState<CompactionDiagnostics | null>(null);
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState<boolean>(false);

  const fetchOnce = useCallback(async () => {
    setLoading(true);
    try {
      const data = await arenaApi.getCompactionDiagnostics();
      setDiagnostics(data);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchOnce();
    const timer = window.setInterval(() => {
      void fetchOnce();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [fetchOnce]);

  return (
    <div className="diagnostics-page">
      <header className="diagnostics-hero">
        <div className="diagnostics-hero__row">
          <div>
            <h1 className="diagnostics-hero__title">梳理</h1>
            <p className="diagnostics-hero__sub">
              看长跑中旧轮次被收进随身便条的情况，用来判断连续性有没有变薄。
            </p>
          </div>
          <button
            type="button"
            className="diagnostics-refresh"
            onClick={() => void fetchOnce()}
            disabled={loading}
          >
            <RefreshCw size={12} strokeWidth={1.5} />
            <span>{loading ? "读取中…" : "刷新"}</span>
          </button>
        </div>
        {diagnostics ? (
          <p className="diagnostics-hero__anchor">
            数据生成时间 · {formatTime(diagnostics.generatedAt)}
          </p>
        ) : null}
      </header>

      {error ? (
        <div className="diagnostics-error" role="alert">
          <AlertTriangle size={14} strokeWidth={1.8} />
          <span>
            <strong>取不到梳理记录。</strong>
            <span className="diagnostics-error__hint"> · </span>
            {backendGap(error)}
          </span>
        </div>
      ) : null}

      {diagnostics ? (
        <>
          <section className="diagnostics-section">
            <div className="section-title">
              <h3>各住户的梳理情况</h3>
              <span className="section-title__hint">按 run 分组</span>
            </div>
            <div className="diagnostics-run-grid">
              {diagnostics.runs.length > 0 ? (
                diagnostics.runs.map((run) => <RunSummaryCard key={`${run.runId}-${run.resident}`} run={run} />)
              ) : (
                <div className="diagnostics-empty">还没有长跑梳理记录</div>
              )}
            </div>
          </section>

          <section className="diagnostics-section">
            <div className="section-title">
              <h3>最近发生的</h3>
              <span className="section-title__hint">新的在前</span>
            </div>
            <RecentEventsTable events={diagnostics.recentEvents} />
          </section>
        </>
      ) : !error ? (
        <div className="diagnostics-loading">正在读住户们的压缩记录…</div>
      ) : null}
    </div>
  );
}

function RunSummaryCard({ run }: { run: CompactionRunSummary }) {
  const accent = residentAccent[run.resident];
  const reduction =
    run.totalTokensBefore > 0
      ? Math.round(((run.totalTokensBefore - run.totalTokensAfter) / run.totalTokensBefore) * 100)
      : 0;
  const paneRatio = run.currentContextWindowTokens
    ? run.latestSummaryPaneTokens / run.currentContextWindowTokens
    : 0;
  const cachePct = Math.round(run.cacheHitRateOnCompactionCall * 100);
  const cacheTone = run.cacheHitRateOnCompactionCall >= 0.9 ? "ok" : run.cacheHitRateOnCompactionCall >= 0.7 ? "info" : "warn";

  return (
    <article
      className="diagnostics-run-card"
      style={{ borderTop: `2px solid ${accent.color}` }}
    >
      <header className="diagnostics-run-card__head">
        <div>
          <div className="diagnostics-run-card__resident">
            <span className="diagnostics-run-card__dot" style={{ background: accent.color }} />
            <strong>{accent.label}</strong>
          </div>
          <div className="diagnostics-run-card__label">{run.runLabel ?? run.runId}</div>
        </div>
        <div className="diagnostics-run-card__count">
          <span className="diagnostics-run-card__count-num">{run.totalCompactions}</span>
        <span className="diagnostics-run-card__count-label">次梳理</span>
        </div>
      </header>

      <div className="diagnostics-run-card__row">
        <span className="diagnostics-run-card__row-label">触发来源</span>
        <div className="diagnostics-pill-row">
          {(Object.keys(TRIGGER_LABEL) as CompactionTriggerReason[])
            .filter((key) => (run.triggerBreakdown[key] ?? 0) > 0)
            .map((key) => (
              <span key={key} className="diagnostics-pill">
                <Zap size={10} strokeWidth={1.8} />
                {TRIGGER_LABEL[key]} · {run.triggerBreakdown[key]}
              </span>
            ))}
        </div>
      </div>

      <div className="diagnostics-run-card__row">
        <span className="diagnostics-run-card__row-label">结果</span>
        <div className="diagnostics-pill-row">
          {(Object.keys(OUTCOME_LABEL) as CompactionOutcome[])
            .filter((key) => (run.outcomeBreakdown[key] ?? 0) > 0)
            .map((key) => (
              <span key={key} className={`diagnostics-pill diagnostics-pill--${OUTCOME_TONE[key]}`}>
                {OUTCOME_LABEL[key]} · {run.outcomeBreakdown[key]}
              </span>
            ))}
        </div>
      </div>

      <dl className="diagnostics-run-card__stats">
        <div>
          <dt>上下文前 → 后</dt>
          <dd>
            {formatTokens(run.totalTokensBefore)} → {formatTokens(run.totalTokensAfter)}
            <span className="diagnostics-run-card__delta"> · 省 {reduction}%</span>
          </dd>
        </div>
        <div>
          <dt>最新旧事梳理</dt>
          <dd>
            {formatTokens(run.latestSummaryPaneTokens)}
            <span className="diagnostics-run-card__delta">
              {" "}
              · 占窗口 {Math.round(paneRatio * 1000) / 10}%
            </span>
          </dd>
        </div>
        <div>
          <dt>整理调用缓存命中</dt>
          <dd>
            <span className={`diagnostics-run-card__cache diagnostics-run-card__cache--${cacheTone}`}>
              {cachePct}%
            </span>
          </dd>
        </div>
      </dl>
    </article>
  );
}

function RecentEventsTable({ events }: { events: CompactionEvent[] }) {
  if (events.length === 0) {
    return <div className="diagnostics-empty">这轮还没发生过压缩</div>;
  }
  return (
    <div className="diagnostics-table-wrap">
      <table className="diagnostics-table">
        <thead>
          <tr>
            <th>时间</th>
            <th>住户</th>
            <th>触发</th>
            <th>结果</th>
            <th>上下文前 → 后</th>
            <th>吞掉的轮数</th>
            <th>用时</th>
            <th>缓存</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <EventRow key={event.compactionId} event={event} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EventRow({ event }: { event: CompactionEvent }) {
  const accent = residentAccent[event.resident];
  const reduction =
    event.tokensBefore > 0
      ? Math.round(((event.tokensBefore - event.tokensAfter) / event.tokensBefore) * 100)
      : 0;
  return (
    <>
      <tr className={`diagnostics-table-row diagnostics-table-row--${OUTCOME_TONE[event.outcome]}`}>
        <td className="diagnostics-table__time">{formatTime(event.occurredAt)}</td>
        <td>
          <span className="diagnostics-resident-cell">
            <span className="diagnostics-run-card__dot" style={{ background: accent.color }} />
            {accent.label}
          </span>
        </td>
        <td>{TRIGGER_LABEL[event.triggerReason]}</td>
        <td>
          <span className={`diagnostics-pill diagnostics-pill--${OUTCOME_TONE[event.outcome]}`}>
            {event.outcome === "guard_rejected" ? <ShieldAlert size={10} strokeWidth={1.8} /> : null}
            {OUTCOME_LABEL[event.outcome]}
          </span>
        </td>
        <td className="diagnostics-table__nums">
          {formatTokens(event.tokensBefore)} → {formatTokens(event.tokensAfter)}
          <span className="diagnostics-table__delta"> · 省 {reduction}%</span>
        </td>
        <td className="diagnostics-table__nums">{event.roundsAbsorbed}</td>
        <td className="diagnostics-table__nums">{Math.round(event.durationMs / 100) / 10}s</td>
        <td>
          <span
            className={`diagnostics-cache-badge diagnostics-cache-badge--${event.cachePrefixHitOnCompactionCall ? "hit" : "miss"}`}
            title={event.cachePrefixHitOnCompactionCall ? "整理调用命中前缀缓存" : "整理调用未命中前缀缓存"}
          >
            {event.cachePrefixHitOnCompactionCall ? "hit" : "miss"}
          </span>
        </td>
      </tr>
      {event.triggerDetail ? (
        <tr className="diagnostics-table-detail">
          <td colSpan={8}>
            <span className="diagnostics-table-detail__label">detail · </span>
            {event.triggerDetail}
            {event.guardRejectedSample ? (
              <>
                <span className="diagnostics-table-detail__sep"> · </span>
                <span className="diagnostics-table-detail__guard">
                  <Layers size={10} strokeWidth={1.8} />
                  {event.guardRejectedSample}
                </span>
              </>
            ) : null}
          </td>
        </tr>
      ) : null}
    </>
  );
}

function formatTokens(n: number): string {
  if (n >= 1000) return `${Math.round(n / 100) / 10}K`;
  return String(n);
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  const ss = String(d.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}

function backendGap(message: string): string {
  if (message.includes("404") || message.toLowerCase().includes("not found")) {
    return "接口暂时不可用。可以先切到 mock 模式看演示数据，或者检查 console-server 日志。";
  }
  return message;
}
