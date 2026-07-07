import type {
  AlertItem,
  FollowupItem,
  OperatorTelemetry,
  PreflightResponse,
  ResidentId,
  RunRecord,
  WorldChatReplyRequest,
  WorldTicketReplyRequest,
  WorldVisibleThread,
} from "../../types/domain";

export interface ArenaConsoleApi {
  getSummary(): Promise<OperatorTelemetry>;
  listRuns(limit?: number): Promise<RunRecord[]>;
  getInbox(limit?: number): Promise<FollowupItem[]>;
  getWorldThread(threadId: string): Promise<WorldVisibleThread>;
  listWorldThreads(): Promise<WorldVisibleThread[]>;
  listAlerts(): Promise<AlertItem[]>;
  getResidentThreads(resident: ResidentId): Promise<WorldVisibleThread[]>;
  sendWorldChatReply(input: WorldChatReplyRequest): Promise<{ ok: true; acceptedAt: string }>;
  sendWorldTicketReply(input: WorldTicketReplyRequest): Promise<{ ok: true; acceptedAt: string }>;
  getPreflight(): Promise<PreflightResponse>;
}