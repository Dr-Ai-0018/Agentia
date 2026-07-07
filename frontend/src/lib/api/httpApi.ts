import type {
  ResidentId,
  WorldChatReplyRequest,
  WorldTicketReplyRequest,
} from "../../types/domain";
import type { ArenaConsoleApi } from "./types";

type HttpOptions = {
  basePath: string;
};

async function parseJson<T = any>(response: Response): Promise<T> {
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

export class HttpArenaConsoleApi implements ArenaConsoleApi {
  private readonly basePath: string;

  constructor(options: HttpOptions) {
    this.basePath = options.basePath.replace(/\/$/, "");
  }

  private url(path: string) {
    return `${this.basePath}${path.startsWith("/") ? path : `/${path}`}`;
  }

  async getSummary() {
    return parseJson(await fetch(this.url("/summary")));
  }

  async listRuns(limit = 20) {
    return parseJson(await fetch(this.url(`/runs?limit=${limit}`)));
  }

  async getInbox(limit = 20) {
    return parseJson(await fetch(this.url(`/inbox?limit=${limit}`)));
  }

  async listAlerts() {
    return parseJson(await fetch(this.url("/alerts")));
  }

  async listWorldThreads() {
    return parseJson(await fetch(this.url("/messages/threads")));
  }

  async getWorldThread(threadId: string) {
    return parseJson(await fetch(this.url(`/messages/thread/${encodeURIComponent(threadId)}`)));
  }

  async getResidentThreads(resident: ResidentId) {
    return parseJson(await fetch(this.url(`/messages/${encodeURIComponent(resident)}/thread`)));
  }

  async sendWorldChatReply(input: WorldChatReplyRequest) {
    return parseJson(
      await fetch(this.url("/reply"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    );
  }

  async sendWorldTicketReply(input: WorldTicketReplyRequest) {
    return parseJson(
      await fetch(this.url("/ticket-reply"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    );
  }

  async getPreflight() {
    return parseJson(await fetch(this.url("/preflight")));
  }
}