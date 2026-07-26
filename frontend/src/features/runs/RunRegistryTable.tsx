import type { RunRecord, RunStatus } from "../../types/domain";

const statusLabel: Record<RunStatus, string> = {
  running: "在跑",
  paused: "暂停",
  finished: "结束",
  failed: "出错",
  abandoned: "旧记录",
};

const modeLabel: Record<RunRecord["mode"], string> = {
  parallel: "并行",
  sequential: "顺序",
};

export function RunRegistryTable({ runs }: { runs: RunRecord[] }) {
  return (
    <div className="run-table">
      <div className="run-table__head">
        <span>这一轮</span>
        <span>状态</span>
        <span>方式</span>
        <span>用时</span>
        <span>住户</span>
        <span>卡住</span>
      </div>
      {runs.map((run) => {
        const stuck = run.budgetBlockedRuns + run.transientBlocked;
        return (
          <div className="run-table__row" key={run.runId}>
            <div className="run-table__cell run-table__cell--run">
              <span className="run-table__run-when">{formatStartedAt(run.startedAt)}</span>
              <span className="run-table__run-id" title={run.runId}>{shortId(run.runId)}</span>
            </div>
            <span className={`run-table__status run-table__status--${run.status}`}>{statusLabel[run.status]}</span>
            <span className="run-table__cell">{modeLabel[run.mode]}</span>
            <span className="run-table__cell run-table__cell--num">{run.elapsed}</span>
            <span className="run-table__cell">{humanizeResidentsSummary(run.residentsSummary)}</span>
            <span className={`run-table__cell run-table__cell--num ${stuck === 0 ? "run-table__cell--good" : "run-table__cell--warn"}`}>{stuck}</span>
          </div>
        );
      })}
    </div>
  );
}

function formatStartedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mi = String(d.getMinutes()).padStart(2, "0");
  return `${mm}-${dd} ${hh}:${mi} 起`;
}

function shortId(id: string): string {
  const match = /orchestrator-(\d{8}T\d{6})/.exec(id);
  if (!match) return id.length > 24 ? `${id.slice(0, 22)}…` : id;
  return `orchestrator-${match[1]}`;
}

function humanizeResidentsSummary(summary: string): string {
  return summary
    .replace(/(\d+)\/(\d+)\s*alive/i, "$1 位都在")
    .replace(/(\d+)\/(\d+)\s*useful/i, "$1 位出货")
    .replace(/(\d+)\/(\d+)\s*errored/i, "$1 位出错");
}
