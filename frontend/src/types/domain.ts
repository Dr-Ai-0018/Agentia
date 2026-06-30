export type ResidentId = "jade" | "amber" | "onyx";

export type ResidentStatus = "running" | "sleeping" | "finished" | "error" | "blocked";

export type Severity = "p0" | "p1" | "p2" | "info";

export type FollowupKind = "chat" | "ticket";

export type ReplyKind = "chat_reply" | "ticket_reply";

export type RunStatus = "running" | "paused" | "finished" | "failed";

export type QuotaLayer = "6h" | "day" | "week";

export type Pressure = "low" | "moderate" | "high" | "critical";

export interface ActiveRun {
  runId: string;
  purpose: string;
  status: RunStatus;
  mode: "parallel" | "sequential";
  startedAt: string;
  updatedAt: string;
  elapsed: string;
  targetDuration: string;
  expectedEndAt: string;
  residents: ResidentId[];
  residentsFinished: number;
  residentsErrored: number;
  budgetBlockedRuns: number;
  transientBlocked: number;
}

export interface ResidentRuntime {
  resident: ResidentId;
  status: ResidentStatus;
  phase: string;
  round: number;
  lastAction: string;
  lastUpdateAge: string;
  inFlight: string;
  totalInputTokens: number;
  totalCachedTokens: number;
  totalOutputTokens: number;
  sleepUntil?: string;
}

export interface ResidentBudget {
  resident: ResidentId;
  sparkBalance: number;
  workAllowedNow: boolean;
  tightestLayer: QuotaLayer;
  pressure: Pressure;
  nextRecoveryAt: string;
  remaining: {
    "6h": number;
    day: number;
    week: number;
  };
  rawRemaining: {
    "6h": number;
    day: number;
    week: number;
  };
}

export interface AlertItem {
  severity: Severity;
  kind: string;
  resident?: ResidentId;
  message: string;
}

export interface FollowupItem {
  kind: FollowupKind;
  resident: ResidentId;
  targetId: string;
  threadId: string;
  createdAt: string;
  age: string;
  status: "pending" | "open" | "replied" | "closed";
  priority?: "low" | "medium" | "high";
  preview: string;
}

export interface RunRecord {
  runId: string;
  status: RunStatus;
  mode: "parallel" | "sequential";
  startedAt: string;
  elapsed: string;
  residentsSummary: string;
  budgetBlockedRuns: number;
  transientBlocked: number;
}

export interface EvidenceItem {
  label: string;
  state: "ready" | "pending" | "failed";
}

export interface OperatorTelemetry {
  activeRun: ActiveRun;
  residents: ResidentRuntime[];
  budgets: ResidentBudget[];
  alerts: AlertItem[];
  followups: FollowupItem[];
  runs: RunRecord[];
  evidence: EvidenceItem[];
  system: {
    capacity: string[];
    vms: string[];
    providers: string[];
    memory: string[];
    economy: string[];
  };
}

export interface WorldMessage {
  id: string;
  from: "chenglin" | ResidentId;
  createdAt: string;
  body: string;
}

export interface WorldVisibleThread {
  resident: ResidentId;
  threadId: string;
  targetId: string;
  kind: FollowupKind;
  messages: WorldMessage[];
}

export interface ReplyDraft {
  residentId: ResidentId;
  threadId: string;
  targetId: string;
  kind: ReplyKind;
  body: string;
  boundaryAck: boolean;
  clientNonce: string;
}

export interface ReplyRequest {
  resident_id: ResidentId;
  thread_id: string;
  target_id: string;
  kind: ReplyKind;
  body: string;
  client_nonce: string;
}
