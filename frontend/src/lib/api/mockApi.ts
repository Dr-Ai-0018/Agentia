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

  async sendWorldChatReply(input: import("../../types/domain").WorldChatReplyRequest) {
    await wait(140);
    return {
      id: `mock-reply-${Date.now()}`,
      from: "chenglin" as const,
      to: "jade" as const,
      direction: "chenglin_to_resident" as const,
      createdAt: new Date().toISOString(),
      body: input.body,
      status: "delivered" as const,
      replyToId: input.message_id,
    };
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

  async sendWorldTicketReply(input: import("../../types/domain").WorldTicketReplyRequest): Promise<import("../../types/domain").WorldTicket> {
    await wait(140);
    return {
      id: input.ticket_id,
      resident: "jade",
      title: "Mock ticket",
      body: "Mock ticket body",
      priority: "medium",
      status: input.close ? "closed" : "answered",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      openedBy: "jade",
      replies: [{ id: `reply-${Date.now()}`, from: "chenglin", body: input.body, createdAt: new Date().toISOString() }],
    };
  }

  async listTickets() {
    await wait();
    return [];
  }

  async getTicket(ticketId: string): Promise<import("../../types/domain").WorldTicket> {
    await wait();
    throw new Error(`World ticket not found: ${ticketId}`);
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
