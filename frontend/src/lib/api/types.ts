import type {
  AlertItem,
  CompactionDiagnostics,
  FollowupItem,
  OperatorTelemetry,
  PreflightResponse,
  ResidentId,
  RunRecord,
  WorldChatReplyRequest,
  WorldChatRequest,
  WorldMessage,
  WorldThreadPage,
  WorldTicket,
  WorldTicketSummary,
  WorldTicketReplyRequest,
  WorldVisibleThread,
} from "../../types/domain";

export interface ArenaConsoleApi {
  getSummary(signal?: AbortSignal): Promise<OperatorTelemetry>;
  listRuns(limit?: number): Promise<RunRecord[]>;
  getInbox(limit?: number): Promise<FollowupItem[]>;
  getWorldThread(threadId: string, signal?: AbortSignal): Promise<WorldVisibleThread>;
  getWorldThreadPage(resident: ResidentId, before?: string, limit?: number, signal?: AbortSignal): Promise<WorldThreadPage>;
  listWorldThreads(signal?: AbortSignal): Promise<WorldVisibleThread[]>;
  listAlerts(): Promise<AlertItem[]>;
  getResidentThreads(resident: ResidentId): Promise<WorldVisibleThread[]>;
  sendWorldChatReply(input: WorldChatReplyRequest): Promise<WorldMessage>;
  sendWorldChat(input: WorldChatRequest): Promise<WorldMessage>;
  sendWorldTicketReply(input: WorldTicketReplyRequest): Promise<WorldTicket>;
  listTickets(filters?: { resident?: ResidentId; status?: string; priority?: string; limit?: number }): Promise<WorldTicketSummary[]>;
  getTicket(ticketId: string): Promise<WorldTicket>;
  getPreflight(): Promise<PreflightResponse>;
  getCompactionDiagnostics(): Promise<CompactionDiagnostics>;
}
