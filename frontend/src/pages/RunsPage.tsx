import { useEffect, useState } from "react";
import { EvidenceChecklist } from "../features/runs/EvidenceChecklist";
import { PreflightPanel } from "../features/runs/PreflightPanel";
import { RunRegistryTable } from "../features/runs/RunRegistryTable";
import { arenaApi } from "../lib/api/client";
import type { OperatorTelemetry, PreflightResponse } from "../types/domain";

export function RunsPage({ telemetry }: { telemetry: OperatorTelemetry }) {
  const [preflight, setPreflight] = useState<PreflightResponse | null>(null);
  const [preflightError, setPreflightError] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    arenaApi
      .getPreflight()
      .then((response) => {
        if (!cancelled) setPreflight(response);
      })
      .catch((err) => {
        if (!cancelled) setPreflightError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="runs-page">
      <header className="runs-hero">
        <h1 className="runs-hero__title">起居</h1>
        <p className="runs-hero__sub">房子一天里跑过什么、还剩什么要看</p>
      </header>

      {preflight ? (
        <PreflightPanel preflight={preflight} />
      ) : preflightError ? (
        <div className="preflight-fallback preflight-fallback--error">
          <span>起飞前检查没拿到：{preflightError}</span>
        </div>
      ) : (
        <div className="preflight-fallback">正在读起飞前检查…</div>
      )}

      <section className="runs-section">
        <div className="section-title">
          <h3>最近跑过的</h3>
          <span className="section-title__hint">新的在前</span>
        </div>
        <RunRegistryTable runs={telemetry.runs} />
      </section>

      <section className="runs-section">
        <div className="section-title">
          <h3>出货前的证据</h3>
          <span className="section-title__hint">长测证据链</span>
        </div>
        <EvidenceChecklist evidence={telemetry.evidence} />
      </section>
    </div>
  );
}
