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

  async getResidentThreads(resident: ResidentId) {
    await wait();
    return Object.values(threads).filter((thread) => thread.resident === resident);
  }

  async sendWorldChatReply() {
    await wait(140);
    return { ok: true as const, acceptedAt: new Date().toISOString() };
  }

  async sendWorldTicketReply() {
    await wait(140);
    return { ok: true as const, acceptedAt: new Date().toISOString() };
  }

  async getPreflight() {
    await wait();
    return mockPreflight;
  }
}