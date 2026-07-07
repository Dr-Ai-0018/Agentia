import type { PreflightCheck, PreflightResponse, PreflightStatus } from "../../types/domain";

const statusLabel: Record<PreflightStatus, string> = {
  good: "好",
  watch: "要留意",
  unknown: "不确定",
};

const sectionLabel: Record<string, string> = {
  service: "服务",
  auth: "门禁",
  storage: "数据",
  health: "健康",
  gate: "闸门",
};

export function PreflightPanel({ preflight }: { preflight: PreflightResponse }) {
  const grouped = groupBySection(preflight.checks);
  const timeText = formatTimeShort(preflight.generated_at);

  return (
    <section className="preflight-panel">
      <header className="preflight-panel__head">
        <div>
          <h3 className="preflight-panel__title">起飞前的检查</h3>
          <p className="preflight-panel__summary">{preflight.summary}</p>
        </div>
        <div className="preflight-panel__overall">
          <span className={`preflight-dot preflight-dot--${preflight.overall}`} />
          <span className="preflight-panel__overall-label">整体 {statusLabel[preflight.overall]}</span>
          <span className="preflight-panel__overall-time">· {timeText}</span>
        </div>
      </header>
      <div className="preflight-panel__groups">
        {grouped.map((group) => (
          <div className="preflight-group" key={group.section}>
            <h4 className="preflight-group__title">{sectionLabel[group.section] ?? group.section}</h4>
            <ul className="preflight-group__list">
              {group.checks.map((check) => (
                <li className="preflight-row" key={check.id}>
                  <span className={`preflight-dot preflight-dot--${check.status}`} aria-label={statusLabel[check.status]} />
                  <div className="preflight-row__body">
                    <div className="preflight-row__head">
                      <span className="preflight-row__label">{check.label}</span>
                      {!check.required ? <span className="preflight-row__optional">可选</span> : null}
                    </div>
                    <div className="preflight-row__detail">{decorate(check)}</div>
                  </div>
                  <span className="preflight-row__status">{statusLabel[check.status]}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </section>
  );
}

type Grouped = { section: string; checks: PreflightCheck[] };

function groupBySection(checks: PreflightCheck[]): Grouped[] {
  const order: string[] = [];
  const map = new Map<string, PreflightCheck[]>();
  for (const check of checks) {
    if (!map.has(check.section)) {
      order.push(check.section);
      map.set(check.section, []);
    }
    map.get(check.section)!.push(check);
  }
  return order.map((section) => ({ section, checks: map.get(section)! }));
}

function decorate(check: PreflightCheck): string {
  const base = check.detail;
  const data = check.data ?? {};
  const extras: string[] = [];
  if (typeof data.uptime_sec === "number") {
    extras.push(`活 ${humanizeSeconds(data.uptime_sec)}`);
  }
  if (typeof data.sample_count === "number") {
    extras.push(`${data.sample_count} 条`);
  }
  if (typeof data.total_5xx === "number" && data.total_5xx > 0) {
    extras.push(`5xx ${data.total_5xx}`);
  }
  if (typeof data.rate_limited === "number" && data.rate_limited > 0) {
    extras.push(`rate limit ${data.rate_limited}`);
  }
  if (extras.length === 0) return base;
  return `${base} · ${extras.join(" · ")}`;
}

function humanizeSeconds(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.round(sec / 60)}m`;
  const h = Math.floor(sec / 3600);
  const m = Math.round((sec % 3600) / 60);
  return m === 0 ? `${h}h` : `${h}h${String(m).padStart(2, "0")}`;
}

function formatTimeShort(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}
