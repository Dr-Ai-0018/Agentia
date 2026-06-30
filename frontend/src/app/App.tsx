import { useEffect, useState } from "react";
import { ConsoleShell, type ConsolePage } from "./layouts/ConsoleShell";
import { arenaApi } from "../lib/api/client";
import { OverviewPage } from "../pages/OverviewPage";
import { ResidentsPage } from "../pages/ResidentsPage";
import { RunsPage } from "../pages/RunsPage";
import { SettingsPage } from "../pages/SettingsPage";
import { SystemPage } from "../pages/SystemPage";
import { WorldChatPage } from "../pages/WorldChatPage";
import type { OperatorTelemetry, WorldVisibleThread } from "../types/domain";

export function App() {
  const [page, setPage] = useState<ConsolePage>("overview");
  const [telemetry, setTelemetry] = useState<OperatorTelemetry | null>(null);
  const [threads, setThreads] = useState<WorldVisibleThread[]>([]);
  const [activeThreadId, setActiveThreadId] = useState<string>("");
  const [error, setError] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [summary, worldThreads] = await Promise.all([arenaApi.getSummary(), arenaApi.listWorldThreads()]);
        if (cancelled) return;
        setTelemetry(summary);
        setThreads(worldThreads);
        setActiveThreadId((current) => current || worldThreads[0]?.threadId || "");
      } catch (loadError) {
        if (!cancelled) setError(loadError instanceof Error ? loadError.message : String(loadError));
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, []);

  function openThread(threadId: string) {
    setActiveThreadId(threadId);
    setPage("world-chat");
  }

  if (error) {
    return <div className="fatal-state">Failed to load console data: {error}</div>;
  }

  if (!telemetry) {
    return <div className="fatal-state">Loading operator console...</div>;
  }

  return (
    <ConsoleShell activePage={page} onNavigate={setPage}>
      {page === "overview" && <OverviewPage telemetry={telemetry} onOpenThread={openThread} />}
      {page === "residents" && <ResidentsPage telemetry={telemetry} />}
      {page === "world-chat" && (
        <WorldChatPage threads={threads} activeThreadId={activeThreadId} onSelectThread={setActiveThreadId} />
      )}
      {page === "runs" && <RunsPage telemetry={telemetry} />}
      {page === "system" && <SystemPage telemetry={telemetry} />}
      {page === "settings" && <SettingsPage />}
    </ConsoleShell>
  );
}
