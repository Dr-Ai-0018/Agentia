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
  WorldTicketReplyRequest,
  WorldVisibleThread,
} from "../../types/domain";

export interface ArenaConsoleApi {
  getSummary(): Promise<OperatorTelemetry>;
  listRuns(limit?: number): Promise<RunRecord[]>;
  getInbox(limit?: number): Promise<FollowupItem[]>;
  getWorldThread(threadId: string): Promise<WorldVisibleThread>;
  getWorldThreadPage(resident: ResidentId, before?: string, limit?: number): Promise<WorldThreadPage>;
  listWorldThreads(): Promise<WorldVisibleThread[]>;
  listAlerts(): Promise<AlertItem[]>;
  getResidentThreads(resident: ResidentId): Promise<WorldVisibleThread[]>;
  sendWorldChatReply(input: WorldChatReplyRequest): Promise<{ ok: true; acceptedAt: string }>;
  sendWorldChat(input: WorldChatRequest): Promise<WorldMessage>;
  sendWorldTicketReply(input: WorldTicketReplyRequest): Promise<{ ok: true; acceptedAt: string }>;
  getPreflight(): Promise<PreflightResponse>;
  getCompactionDiagnostics(): Promise<CompactionDiagnostics>;
}
