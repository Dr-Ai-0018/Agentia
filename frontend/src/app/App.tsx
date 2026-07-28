import { useCallback, useEffect, useRef, useState } from "react";
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
import { shouldPollSummary } from "./pollingPolicy";

const SUMMARY_POLL_INTERVAL_MS = 15000;
const CHAT_POLL_INTERVAL_MS = 8000;

export function App() {
  const [page, setPage] = useState<ConsolePage>("overview");
  const [telemetry, setTelemetry] = useState<OperatorTelemetry | null>(null);
  const [threads, setThreads] = useState<WorldVisibleThread[]>([]);
  const [activeThreadId, setActiveThreadId] = useState<string>("");
  const [fatalError, setFatalError] = useState<string>("");
  const [syncError, setSyncError] = useState<string>("");
  const [lastFetchedAt, setLastFetchedAt] = useState<Date | null>(null);
  const summaryRequest = useRef<AbortController | null>(null);
  const chatRequest = useRef<AbortController | null>(null);

  const fetchSummary = useCallback(async () => {
    summaryRequest.current?.abort();
    const controller = new AbortController();
    summaryRequest.current = controller;
    try {
      setTelemetry(await arenaApi.getSummary(controller.signal));
      setLastFetchedAt(new Date());
      setSyncError("");
    } finally {
      if (summaryRequest.current === controller) summaryRequest.current = null;
    }
  }, []);

  const fetchAllThreads = useCallback(async () => {
    chatRequest.current?.abort();
    const controller = new AbortController();
    chatRequest.current = controller;
    try {
      const worldThreads = await arenaApi.listWorldThreads(controller.signal);
      setThreads(worldThreads);
      setActiveThreadId((current) => current || worldThreads[0]?.threadId || "");
    } finally {
      if (chatRequest.current === controller) chatRequest.current = null;
    }
  }, []);

  const fetchActiveThread = useCallback(async () => {
    if (!activeThreadId) return;
    chatRequest.current?.abort();
    const controller = new AbortController();
    chatRequest.current = controller;
    try {
      const updated = await arenaApi.getWorldThread(activeThreadId, controller.signal);
      setThreads((current) => current.map((thread) => thread.threadId === updated.threadId ? updated : thread));
    } finally {
      if (chatRequest.current === controller) chatRequest.current = null;
    }
  }, [activeThreadId]);

  // Initial load: first failure is fatal so the user sees a real error rather
  // than an empty console.
  useEffect(() => {
    let cancelled = false;
    fetchSummary().catch((err) => {
      if (!cancelled) setFatalError(err instanceof Error ? err.message : String(err));
    });
    return () => {
      cancelled = true;
      summaryRequest.current?.abort();
    };
  }, [fetchSummary]);

  // Polling loop. Only starts after the initial load succeeds. Skips a tick
  // when the tab is hidden — the human isn't looking, and it keeps the mock
  // fixture from producing endless "fresh" data in dev.
  useEffect(() => {
    if (!telemetry || !shouldPollSummary(page)) {
      summaryRequest.current?.abort();
      return;
    }
    const id = window.setInterval(() => {
      if (document.hidden) return;
      fetchSummary().catch((err) => {
        if (!isAbortError(err)) setSyncError(err instanceof Error ? err.message : String(err));
      });
    }, SUMMARY_POLL_INTERVAL_MS);
    return () => {
      window.clearInterval(id);
      summaryRequest.current?.abort();
    };
  }, [page, telemetry, fetchSummary]);

  useEffect(() => {
    if (page !== "world-chat") return;
    fetchAllThreads().catch((err) => {
      if (!isAbortError(err)) setSyncError(err instanceof Error ? err.message : String(err));
    });
    return () => chatRequest.current?.abort();
  }, [page, fetchAllThreads]);

  useEffect(() => {
    if (page !== "world-chat" || !activeThreadId) return;
    const id = window.setInterval(() => {
      if (document.hidden) return;
      fetchActiveThread().catch((err) => {
        if (!isAbortError(err)) setSyncError(err instanceof Error ? err.message : String(err));
      });
    }, CHAT_POLL_INTERVAL_MS);
    return () => {
      window.clearInterval(id);
      chatRequest.current?.abort();
    };
  }, [page, activeThreadId, fetchActiveThread]);

  const openThread = useCallback((threadId: string) => {
    const resident = threadId.match(/(?:chat|ticket)-(jade|amber|onyx)(?:-|$)/)?.[1];
    setActiveThreadId(resident ? `chat-${resident}` : threadId);
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
        <WorldChatPage
          threads={threads}
          interventions={telemetry.followups.filter((item) => item.kind === "intervention")}
          activeThreadId={activeThreadId}
          onSelectThread={setActiveThreadId}
        />
      )}
      {page === "runs" && <RunsPage telemetry={telemetry} />}
      {page === "system" && <SystemPage telemetry={telemetry} />}
      {page === "compaction-diagnostics" && <CompactionDiagnosticsPage />}
      {page === "settings" && <SettingsPage />}
    </ConsoleShell>
  );
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
