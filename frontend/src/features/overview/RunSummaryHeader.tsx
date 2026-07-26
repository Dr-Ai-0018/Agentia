import { Activity, Clock, Database, Radio } from "lucide-react";
import { Badge } from "../../components/ui/Badge";
import type { ActiveRun } from "../../types/domain";

export function RunSummaryHeader({ run }: { run: ActiveRun }) {
  return (
    <section className="run-hero">
      <div>
        <p className="eyebrow">Private Operator Console</p>
        <h1>Autonomy Observation</h1>
        <p className="hero-copy">
          Live surface for resident autonomy, quota recovery, world-safe inbox, and pre-release evidence.
        </p>
      </div>
      <div className="run-hero__metrics">
        <div className="hero-metric">
          <Activity size={18} />
          <span>Status</span>
          <strong>{run.status}</strong>
        </div>
        <div className="hero-metric">
          <Clock size={18} />
          <span>Elapsed</span>
          <strong>{run.elapsed}</strong>
        </div>
        <div className="hero-metric">
          <Radio size={18} />
          <span>Mode</span>
          <strong>{run.mode}</strong>
        </div>
        <div className="hero-metric">
          <Database size={18} />
          <span>Freshness</span>
          <strong>{new Date(run.updatedAt).toISOString().slice(11, 19)}Z</strong>
        </div>
      </div>
      <div className="run-hero__footer">
        <Badge tone="info">{run.runId}</Badge>
        <span>Duration {run.targetDuration}</span>
        {run.expectedEndAt ? <span>Expected end {new Date(run.expectedEndAt).toISOString().slice(0, 16)}Z</span> : null}
      </div>
    </section>
  );
}
