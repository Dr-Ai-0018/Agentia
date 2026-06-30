import type {
  AlertItem,
  FollowupItem,
  OperatorTelemetry,
  ReplyRequest,
  ResidentId,
  RunRecord,
  WorldVisibleThread,
} from "../../types/domain";

export interface ArenaConsoleApi {
  getSummary(): Promise<OperatorTelemetry>;
  listRuns(limit?: number): Promise<RunRecord[]>;
  getInbox(limit?: number): Promise<FollowupItem[]>;
  getWorldThread(threadId: string): Promise<WorldVisibleThread>;
  listWorldThreads(): Promise<WorldVisibleThread[]>;
  listAlerts(): Promise<AlertItem[]>;
  sendWorldReply(input: ReplyRequest): Promise<{ ok: true; acceptedAt: string }>;
  getResidentThreads(resident: ResidentId): Promise<WorldVisibleThread[]>;
}
