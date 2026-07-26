export type ResidentId = "jade" | "amber" | "onyx";

export type ResidentStatus = "running" | "sleeping" | "finished" | "error" | "blocked" | "idle";

export type Severity = "p0" | "p1" | "p2" | "info";

export type FollowupKind = "chat" | "ticket";

export type ReplyKind = "chat_reply" | "ticket_reply";

export type RunStatus = "running" | "paused" | "finished" | "failed" | "abandoned";

export type QuotaLayer = "6h" | "day" | "week";

export type Pressure = "low" | "moderate" | "high" | "critical";

// Fatigue: continuous variable, 0-100. Rendered as a feel-word, not a
// number, to residents / operators. Backed by broker fatigue accumulator.
export type FatigueMood = "fresh" | "warming_up" | "some_tiredness" | "quite_tired" | "exhausted";

// Sleep depth tiers per the 2026-07-07 quota model decision:
// - rest: 5-30m nap, recovery x1.5
// - sleep: 30m-4h regular, recovery x2.5
// - deep_sleep: continuous 4h+ AND 24h cumulative >= 6-8h, recovery x4
export type SleepDepth = "awake" | "rest" | "sleep" | "deep_sleep";

export interface Fatigue {
  level: number;
  mood: FatigueMood;
}

export interface SleepState {
  depth: SleepDepth;
  // Sleep debt in hours. The third ledger — under-slept last 24h means
  // recovery rates get discounted until debt drops. Cannot be paid off in
  // one session; needs consecutive nights of proper sleep.
  debtHours: number;
}

export interface ActiveRun {
  isLive: boolean;
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
  summaryPane?: SummaryPane;
}

// SummaryPane is operator-only long-run continuity data. Text is
// resident-voice retrospection prose; residents themselves never see this
// field.
export interface SummaryPaneEvidenceRef {
  kind: "round" | "note" | "guest_artifact";
  ref: string;
  rounds?: number[];
}

export interface SummaryPane {
  text: string;
  updatedAt: string;
  roundsAbsorbed: number;
  approxTokens: number;
  evidenceRefs?: SummaryPaneEvidenceRef[];
}

// ResidentBudget carries the observation surface for a single resident.
// After the 2026-07-07 quota model decision (see PLAN.md § Quota Model
// Decision), the semantics are:
//   remaining.day  — real rolling 24h remaining %; hard gate at 0
//   remaining.week — real rolling 7d remaining %; hard gate at 0
//   remaining["6h"] — OBSERVATION only, not a gate. Rendered as sparkline.
//   tightestLayer  — which of day/week is currently tightest. Never "6h".
//   pressure       — derived from day/week + fatigue + sleep debt
//   fatigue        — continuous accumulator; drains via sleep depth tiers
//   sleep          — current depth + accumulated sleep debt in hours
//   sixHourBurn    — last 6h consumption samples for the observation window
// Fields marked optional so this can roll out before backend adapter emits.
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
  fatigue?: Fatigue;
  sleep?: SleepState;
  sixHourBurn?: number[];
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
  threadId?: string;
  createdAt: string;
  age: string;
  status: "pending" | "open" | "replied" | "closed";
  priority?: "low" | "medium" | "high" | "urgent";
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

export type PreflightStatus = "good" | "watch" | "unknown";

export interface PreflightCheck {
  id: string;
  section: string;
  label: string;
  status: PreflightStatus;
  required: boolean;
  detail: string;
  data?: Record<string, unknown>;
}

export interface PreflightResponse {
  generated_at: string;
  overall: PreflightStatus;
  summary: string;
  checks: PreflightCheck[];
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
  to: "chenglin" | ResidentId;
  direction: "resident_to_chenglin" | "chenglin_to_resident";
  createdAt: string;
  body: string;
  status: "pending" | "replied" | "delivered";
  replyToId?: string;
  readAt?: string;
}

export interface WorldVisibleThread {
  resident: ResidentId;
  threadId: string;
  targetId?: string;
  kind: FollowupKind;
  messages: WorldMessage[];
  total: number;
  pendingCount: number;
  repliedCount: number;
  deliveredCount: number;
  hasMore: boolean;
  nextBefore?: string;
  lastMessageAt?: string;
  lastPreview?: string;
}

export interface WorldThreadPage {
  resident: ResidentId;
  messages: WorldMessage[];
  total: number;
  hasMore: boolean;
  nextBefore?: string;
}

// ReplyDraft is the local editing state for the reply composer. It stays
// inside the world-chat feature; outside observation data must not enter it.
export interface ReplyDraft {
  residentId: ResidentId;
  threadId: string;
  targetId: string;
  kind: ReplyKind;
  body: string;
  boundaryAck: boolean;
  clientNonce: string;
  closeTicket?: boolean;
}

// Server-facing request shapes — match backend /api/reply and /api/ticket-reply
// exactly. Field names are snake_case to survive JSON.stringify without a
// mapping layer. The narrow shape is intentional: it is a hard structural
// boundary that prevents run ids, budgets, phases, and other operator-only
// context from ever reaching the world through this door.
export interface WorldChatReplyRequest {
  message_id: string;
  body: string;
  boundary_ack: true;
}

export interface WorldChatRequest {
  resident: ResidentId;
  body: string;
  boundary_ack: true;
}

export interface WorldTicketReplyRequest {
  ticket_id: string;
  body: string;
  close: boolean;
  boundary_ack: true;
}

// Compaction diagnostics types are operator-only long-run continuity data.
// Residents never see any of this; the console panel reads these fields.
export type CompactionTriggerReason =
  | "preflight_measured"
  | "acceptance_microcompact"
  | "reactive_overflow"
  | "manual";

export type CompactionOutcome =
  | "summarized"
  | "silent_trim"
  | "guard_rejected"
  | "failed";

export interface CompactionEvent {
  compactionId: string;
  runId: string;
  resident: ResidentId;
  occurredAt: string;
  triggerReason: CompactionTriggerReason;
  triggerDetail?: string;
  tokensBefore: number;
  tokensAfter: number;
  providerCostOnlySpark?: number;
  providerCostOnlyUsd?: number;
  providerCostClass?: string;
  providerUsageRecorded: boolean;
  roundsAbsorbed: number;
  summaryPaneTokensAfter: number;
  outcome: CompactionOutcome;
  guardRejectedSample?: string;
  durationMs: number;
  cachePrefixHitOnCompactionCall: boolean;
}

export interface CompactionRunSummary {
  runId: string;
  runLabel?: string;
  runStartedAt: string;
  resident: ResidentId;
  totalCompactions: number;
  triggerBreakdown: Record<CompactionTriggerReason, number>;
  outcomeBreakdown: Record<CompactionOutcome, number>;
  totalTokensBefore: number;
  totalTokensAfter: number;
  providerCostOnlySpark?: number;
  providerCostOnlyUsd?: number;
  providerUsageMissing?: number;
  cacheHitRateOnCompactionCall: number;
  latestSummaryPaneTokens: number;
  currentContextWindowTokens: number;
}

export interface CompactionDiagnostics {
  generatedAt: string;
  runs: CompactionRunSummary[];
  recentEvents: CompactionEvent[];
}

// Compile-time guard: proves a props type P has no keys overlapping with
// OperatorTelemetry. Used at world-visible component boundaries to make
// "no telemetry here" a type-check, not a convention.
//
// Usage:
//   type WorldChatPageProps = AssertWorldSurfaceIsolated<{
//     threads: WorldVisibleThread[];
//     ...
//   }>;
//
// If any future maintainer adds `telemetry: OperatorTelemetry` or a matching
// key like `alerts`, `budgets`, `runs`, etc., the type collapses to `never`
// and any component using those props fails to compile.
export type AssertWorldSurfaceIsolated<P> =
  Extract<keyof P, keyof OperatorTelemetry> extends never ? P : never;
