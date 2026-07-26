import { mockCompactionDiagnostics } from "../../data/mockCompaction";
import { mockPreflight, telemetry, threads } from "../../data/mockConsole";
import type { ResidentId } from "../../types/domain";
import type { ArenaConsoleApi } from "./types";

const wait = (ms = 80) => new Promise((resolve) => window.setTimeout(resolve, ms));

export class MockArenaConsoleApi implements ArenaConsoleApi {
  async getSummary() {
    await wait();
    return telemetry;
  }

  async listRuns(limit = 20) {
    await wait();
    return telemetry.runs.slice(0, limit);
  }

  async getInbox(limit = 20) {
    await wait();
    return telemetry.followups.slice(0, limit);
  }

  async listAlerts() {
    await wait();
    return telemetry.alerts;
  }

  async listWorldThreads() {
    await wait();
    return Object.values(threads);
  }

  async getWorldThread(threadId: string) {
    await wait();
    const thread = threads[threadId];
    if (!thread) throw new Error(`World thread not found: ${threadId}`);
    return thread;
  }

  async getWorldThreadPage(resident: ResidentId) {
    await wait();
    const thread = Object.values(threads).find((item) => item.resident === resident);
    return {
      resident,
      messages: thread?.messages ?? [],
      total: thread?.messages.length ?? 0,
      hasMore: false,
    };
  }

  async getResidentThreads(resident: ResidentId) {
    await wait();
    return Object.values(threads).filter((thread) => thread.resident === resident);
  }

  async sendWorldChatReply() {
    await wait(140);
    return { ok: true as const, acceptedAt: new Date().toISOString() };
  }

  async sendWorldChat(input: import("../../types/domain").WorldChatRequest) {
    await wait(140);
    return {
      id: `mock-chat-${Date.now()}`,
      from: "chenglin" as const,
      to: input.resident,
      direction: "chenglin_to_resident" as const,
      createdAt: new Date().toISOString(),
      body: input.body,
      status: "delivered" as const,
    };
  }

  async sendWorldTicketReply() {
    await wait(140);
    return { ok: true as const, acceptedAt: new Date().toISOString() };
  }

  async getPreflight() {
    await wait();
    return mockPreflight;
  }

  async getCompactionDiagnostics() {
    await wait();
    return mockCompactionDiagnostics;
  }
}
