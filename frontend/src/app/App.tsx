import { useCallback, useEffect, useState } from "react";
import { ConsoleShell, type ConsolePage } from "./layouts/ConsoleShell";
import { arenaApi } from "../lib/api/client";
import { CompactionDiagnosticsPage } from "../pages/CompactionDiagnosticsPage";
import { OverviewPage } from "../pages/OverviewPage";
import { ResidentsPage } from "../pages/ResidentsPage";
import { RunsPage } from "../pages/RunsPage";
import { SettingsPage } from "../pages/SettingsPage";
import { SystemPage } from "../pages/SystemPage";
import { WorldChatPage } from "../pages/WorldChatPage";
import type { OperatorTelemetry, WorldVisibleThread } from "../types/domain";

const POLL_INTERVAL_MS = 8000;

export function App() {
  const [page, setPage] = useState<ConsolePage>("overview");
  const [telemetry, setTelemetry] = useState<OperatorTelemetry | null>(null);
  const [threads, setThreads] = useState<WorldVisibleThread[]>([]);
  const [activeThreadId, setActiveThreadId] = useState<string>("");
  const [fatalError, setFatalError] = useState<string>("");
  const [syncError, setSyncError] = useState<string>("");
  const [lastFetchedAt, setLastFetchedAt] = useState<Date | null>(null);

  const fetchOnce = useCallback(async () => {
    const [summary, worldThreads] = await Promise.all([
      arenaApi.getSummary(),
      arenaApi.listWorldThreads(),
    ]);
    setTelemetry(summary);
    setThreads(worldThreads);
    setActiveThreadId((current) => current || worldThreads[0]?.threadId || "");
    setLastFetchedAt(new Date());
    setSyncError("");
  }, []);

  // Initial load: first failure is fatal so the user sees a real error rather
  // than an empty console.
  useEffect(() => {
    let cancelled = false;
    fetchOnce().catch((err) => {
      if (!cancelled) setFatalError(err instanceof Error ? err.message : String(err));
    });
    return () => {
      cancelled = true;
    };
  }, [fetchOnce]);

  // Polling loop. Only starts after the initial load succeeds. Skips a tick
  // when the tab is hidden — the human isn't looking, and it keeps the mock
  // fixture from producing endless "fresh" data in dev.
  useEffect(() => {
    if (!telemetry) return;
    const id = window.setInterval(() => {
      if (document.hidden) return;
      fetchOnce().catch((err) => {
        setSyncError(err instanceof Error ? err.message : String(err));
      });
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(id);
  }, [telemetry, fetchOnce]);

  const openThread = useCallback((threadId: string) => {
    setActiveThreadId(threadId);
    setPage("world-chat");
  }, []);

  if (fatalError) {
    return <div className="fatal-state">Failed to load console data: {fatalError}</div>;
  }

  if (!telemetry) {
    return <div className="fatal-state">Loading operator console...</div>;
  }

  return (
    <ConsoleShell activePage={page} onNavigate={setPage} lastFetchedAt={lastFetchedAt} syncError={syncError}>
      {page === "overview" && <OverviewPage telemetry={telemetry} onOpenThread={openThread} />}
      {page === "residents" && <ResidentsPage telemetry={telemetry} />}
      {page === "world-chat" && (
        <WorldChatPage threads={threads} activeThreadId={activeThreadId} onSelectThread={setActiveThreadId} />
      )}
      {page === "runs" && <RunsPage telemetry={telemetry} />}
      {page === "system" && <SystemPage telemetry={telemetry} />}
      {page === "compaction-diagnostics" && <CompactionDiagnosticsPage />}
      {page === "settings" && <SettingsPage />}
    </ConsoleShell>
  );
}
