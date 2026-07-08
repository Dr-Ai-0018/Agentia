import type {
  ActiveRun,
  AlertItem,
  EvidenceItem,
  FatigueMood,
  FollowupItem,
  OperatorTelemetry,
  Pressure,
  QuotaLayer,
  ResidentBudget,
  ResidentId,
  ResidentRuntime,
  RunRecord,
  RunStatus,
  WorldChatReplyRequest,
  WorldMessage,
  WorldTicketReplyRequest,
  WorldVisibleThread,
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

type ApiSummary = {
  generated_at: string;
  active_run?: ApiRunStatus;
  latest_run?: ApiRunRecord;
  runs?: ApiRunRecord[];
  budget?: ApiBudgetStatus;
  inbox?: ApiInbox;
  followups?: ApiFollowup[];
  inspect?: ApiInspect;
  acceptance?: ApiAcceptance;
  alerts?: ApiAlert[];
};

type ApiRunStatus = {
  run_id: string;
  status: string;
  mode: "parallel" | "sequential";
  residents?: ApiResidentRunStatus[];
  started_at?: string;
  updated_at?: string;
};

type ApiResidentRunStatus = {
  resident: string;
  status?: string;
  updated_at?: string;
  transient_blocked?: boolean;
  current_phase?: string;
  current_round?: number;
  remaining_sec?: number;
  last_action?: string;
  in_flight_started_at?: string;
  total_input_tokens?: number;
  total_cached_tokens?: number;
  total_output_tokens?: number;
};

type ApiRunRecord = {
  run_id: string;
  status: string;
  mode: "parallel" | "sequential";
  residents?: string[];
  started_at?: string;
  updated_at?: string;
  finished_at?: string;
};

type ApiBudgetStatus = {
  residents?: ApiResidentBudget[];
  totals?: {
    spark_balance?: number;
    work_allowed_count?: number;
    blocked_count?: number;
  };
};

type ApiResidentBudget = {
  resident_id: string;
  spark_balance?: number;
  work_allowed_now?: boolean;
  effective_window_6h_cap?: number;
  effective_window_6h_remaining?: number;
  effective_day_cap?: number;
  effective_day_remaining?: number;
  rolling_day_remaining?: number;
  effective_week_cap?: number;
  effective_week_remaining?: number;
  rolling_week_remaining?: number;
  quota_tightest_layer?: string;
  pressure?: string;
  next_recovery_at?: string;
  fatigue?: {
    level?: number;
    mood?: string;
  };
  sleep?: {
    depth?: string;
    debt_hours?: number;
  };
};

type ApiInbox = {
  pending_chat_messages?: ApiThreadMessage[];
  open_tickets?: ApiTicketSummary[];
};

type ApiThreadMessage = {
  id: string;
  direction?: string;
  resident: string;
  from?: string;
  body: string;
  created_at?: string;
  status?: string;
};

type ApiTicketSummary = {
  id: string;
  resident: string;
  title?: string;
  priority?: "low" | "medium" | "high" | "urgent";
  status?: string;
  created_at?: string;
  updated_at?: string;
  last_preview?: string;
};

type ApiFollowup = {
  kind: string;
  resident: string;
  target_id: string;
  priority?: "low" | "medium" | "high" | "urgent";
  created_at?: string;
  updated_at?: string;
  title?: string;
  preview?: string;
  status?: string;
};

type ApiInspect = {
  capacity?: { pools?: Array<{ resource: string; host_available?: number; allocatable_free?: number; unit?: string }> };
  residents_running?: number;
  resident_count?: number;
  memory_resident_review_queue?: number;
  memory_operator_review_required?: number;
  memory_duplicate_history_groups?: number;
  resident_risk?: Array<{ resident_id: string; status?: string }>;
  latest_orchestrator?: {
    duration?: string;
    residents_finished?: number;
    residents_errored?: number;
    transient_blocked?: number;
    budget_blocked_runs?: number;
    residents_planned?: string[];
  };
};

type ApiAcceptance = {
  checks?: Array<{ id: string; title?: string; status: string; required?: boolean; blocks_release?: boolean }>;
};

type ApiAlert = {
  severity: string;
  kind: string;
  resident?: string;
  message: string;
};

export class HttpArenaConsoleApi implements ArenaConsoleApi {
  private readonly basePath: string;

  constructor(options: HttpOptions) {
    this.basePath = options.basePath.replace(/\/$/, "");
  }

  private url(path: string) {
    return `${this.basePath}${path.startsWith("/") ? path : `/${path}`}`;
  }

  async getSummary() {
    return normalizeSummary(await parseJson<ApiSummary>(await fetch(this.url("/summary"))));
  }

  async listRuns(limit = 20) {
    const runs = await parseJson<ApiRunRecord[]>(await fetch(this.url(`/runs?limit=${limit}`)));
    return runs.map((run) => normalizeRunRecord(run, undefined));
  }

  async getInbox(limit = 20) {
    const inbox = await parseJson<ApiInbox>(await fetch(this.url(`/inbox?limit=${limit}`)));
    return followupsFromInbox(inbox);
  }

  async listAlerts() {
    const summary = await parseJson<ApiSummary>(await fetch(this.url("/summary?limit=20")));
    return (summary.alerts ?? []).map(normalizeAlert);
  }

  async listWorldThreads() {
    const inbox = await parseJson<ApiInbox>(await fetch(this.url("/inbox?limit=20")));
    return threadsFromInbox(inbox);
  }

  async getWorldThread(threadId: string) {
    const threads = await this.listWorldThreads();
    const thread = threads.find((item) => item.threadId === threadId);
    if (!thread) throw new Error(`World thread not found: ${threadId}`);
    return thread;
  }

  async getResidentThreads(resident: ResidentId) {
    return (await this.listWorldThreads()).filter((thread) => thread.resident === resident);
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

function normalizeSummary(input: ApiSummary): OperatorTelemetry {
  const activeRun = normalizeActiveRun(input.active_run, input.latest_run, input.inspect);
  const budgets = (input.budget?.residents ?? []).map(normalizeBudget).filter(Boolean) as ResidentBudget[];
  const residents = normalizeResidents(input.active_run?.residents, budgets, input.inspect);
  const residentBudgets = ensureResidentBudgets(budgets, residents.map((resident) => resident.resident));
  const followups = normalizeFollowups(input.followups, input.inbox);
  const runs = (input.runs ?? []).map((run) => normalizeRunRecord(run, input.inspect));

  return {
    activeRun,
    residents,
    budgets: residentBudgets,
    alerts: (input.alerts ?? []).map(normalizeAlert),
    followups,
    runs,
    evidence: normalizeEvidence(input.acceptance),
    system: normalizeSystem(input.inspect, input.budget),
  };
}

function ensureResidentBudgets(budgets: ResidentBudget[], residents: ResidentId[]): ResidentBudget[] {
  const byResident = new Map(budgets.map((budget) => [budget.resident, budget]));
  return residents.map((resident) => byResident.get(resident) ?? emptyBudget(resident));
}

function emptyBudget(resident: ResidentId): ResidentBudget {
  return {
    resident,
    sparkBalance: 0,
    workAllowedNow: false,
    tightestLayer: "6h",
    pressure: "critical",
    nextRecoveryAt: "",
    remaining: { "6h": 0, day: 0, week: 0 },
    rawRemaining: { "6h": 0, day: 0, week: 0 },
  };
}

function normalizeActiveRun(active: ApiRunStatus | undefined, latest: ApiRunRecord | undefined, inspect: ApiInspect | undefined): ActiveRun {
  const source = active ?? latest;
  const startedAt = source?.started_at ?? "";
  const updatedAt = source && "updated_at" in source ? source.updated_at ?? "" : "";
  const duration = active ? elapsedBetween(startedAt, updatedAt) : inspect?.latest_orchestrator?.duration ?? "";
  const residents = normalizeResidentList(
    active?.residents?.map((item) => item.resident) ?? latest?.residents ?? inspect?.latest_orchestrator?.residents_planned,
  );
  return {
    runId: source?.run_id ?? "no-active-run",
    purpose: active ? "观察中" : "最近一次观察",
    status: normalizeRunStatus(source?.status),
    mode: source?.mode ?? "parallel",
    startedAt,
    updatedAt,
    elapsed: compactDuration(duration),
    targetDuration: active ? "24h" : "—",
    expectedEndAt: "",
    residents,
    residentsFinished: inspect?.latest_orchestrator?.residents_finished ?? (active ? 0 : residents.length),
    residentsErrored: inspect?.latest_orchestrator?.residents_errored ?? 0,
    budgetBlockedRuns: inspect?.latest_orchestrator?.budget_blocked_runs ?? 0,
    transientBlocked: inspect?.latest_orchestrator?.transient_blocked ?? 0,
  };
}

function normalizeResidents(
  activeResidents: ApiResidentRunStatus[] | undefined,
  budgets: ResidentBudget[],
  inspect: ApiInspect | undefined,
): ResidentRuntime[] {
  const activeByID = new Map((activeResidents ?? []).map((item) => [item.resident, item]));
  const riskByID = new Map((inspect?.resident_risk ?? []).map((item) => [item.resident_id, item]));
  const ids = normalizeResidentList([
    ...Array.from(activeByID.keys()),
    ...budgets.map((item) => item.resident),
    ...Array.from(riskByID.keys()),
    "jade",
    "amber",
    "onyx",
  ]);
  return ids.map((resident) => {
    const active = activeByID.get(resident);
    const risk = riskByID.get(resident);
    return {
      resident,
      status: normalizeResidentStatus(active?.status ?? risk?.status),
      phase: active?.current_phase ?? "",
      round: active?.current_round ?? 0,
      lastAction: active?.last_action ?? "",
      lastUpdateAge: active?.updated_at ? ageFromNow(active.updated_at) : "",
      inFlight: active?.in_flight_started_at ? ageClock(active.in_flight_started_at) : "",
      totalInputTokens: active?.total_input_tokens ?? 0,
      totalCachedTokens: active?.total_cached_tokens ?? 0,
      totalOutputTokens: active?.total_output_tokens ?? 0,
    };
  });
}

function normalizeBudget(input: ApiResidentBudget): ResidentBudget | null {
  const resident = asResident(input.resident_id);
  if (!resident) return null;
  return {
    resident,
    sparkBalance: input.spark_balance ?? 0,
    workAllowedNow: input.work_allowed_now ?? false,
    tightestLayer: normalizeLayer(input.quota_tightest_layer),
    pressure: normalizePressure(input.pressure),
    nextRecoveryAt: input.next_recovery_at ?? "",
    remaining: {
      "6h": percentRemaining(input.effective_window_6h_remaining, input.effective_window_6h_cap),
      day: percentRemaining(input.rolling_day_remaining ?? input.effective_day_remaining, input.effective_day_cap),
      week: percentRemaining(input.rolling_week_remaining ?? input.effective_week_remaining, input.effective_week_cap),
    },
    rawRemaining: {
      "6h": input.effective_window_6h_remaining ?? 0,
      day: input.rolling_day_remaining ?? input.effective_day_remaining ?? 0,
      week: input.rolling_week_remaining ?? input.effective_week_remaining ?? 0,
    },
    fatigue: normalizeFatigue(input.fatigue),
    sleep: normalizeSleep(input.sleep),
  };
}

function normalizeFatigue(input: ApiResidentBudget["fatigue"]): ResidentBudget["fatigue"] {
  if (!input) return undefined;
  const level = Math.max(0, Math.min(100, input.level ?? 0));
  return {
    level,
    mood: normalizeFatigueMood(input.mood) ?? fatigueMoodFromLevel(level),
  };
}

function normalizeFatigueMood(input: string | undefined): FatigueMood | null {
  switch (input) {
    case "fresh":
    case "warming_up":
    case "some_tiredness":
    case "quite_tired":
    case "exhausted":
      return input;
    default:
      return null;
  }
}

function fatigueMoodFromLevel(level: number): FatigueMood {
  if (level >= 80) return "exhausted";
  if (level >= 60) return "quite_tired";
  if (level >= 40) return "some_tiredness";
  if (level >= 20) return "warming_up";
  return "fresh";
}

function normalizeSleep(input: ApiResidentBudget["sleep"]): ResidentBudget["sleep"] {
  if (!input) return undefined;
  return {
    depth: normalizeSleepDepth(input.depth),
    debtHours: Math.max(0, input.debt_hours ?? 0),
  };
}

function normalizeSleepDepth(input: string | undefined): NonNullable<ResidentBudget["sleep"]>["depth"] {
  switch (input) {
    case "rest":
    case "sleep":
    case "deep_sleep":
      return input;
    default:
      return "awake";
  }
}

function normalizeFollowups(input: ApiFollowup[] | undefined, inbox: ApiInbox | undefined): FollowupItem[] {
  if (!input || input.length === 0) return followupsFromInbox(inbox);
  return input.flatMap((item) => {
    const kind = normalizeFollowupKind(item.kind);
    if (!kind) return [];
    return [{
      kind,
      resident: asResident(item.resident) ?? "jade",
      targetId: item.target_id,
      threadId: threadIDFor(kind, item.resident, item.target_id),
      createdAt: item.created_at ?? "",
      age: item.created_at ? ageFromNow(item.created_at) : "",
      status: item.status === "closed" || item.status === "replied" ? item.status : kind === "ticket" ? "open" : "pending",
      priority: item.priority,
      preview: item.preview ?? item.title ?? "",
    }];
  });
}

function normalizeRunRecord(input: ApiRunRecord, inspect: ApiInspect | undefined): RunRecord {
  return {
    runId: input.run_id,
    status: normalizeRunStatus(input.status),
    mode: input.mode ?? "parallel",
    startedAt: input.started_at ?? "",
    elapsed: compactDuration(elapsedBetween(input.started_at, input.finished_at ?? input.updated_at)),
    residentsSummary: `${input.residents?.length ?? inspect?.latest_orchestrator?.residents_planned?.length ?? 0}/3 alive`,
    budgetBlockedRuns: inspect?.latest_orchestrator?.budget_blocked_runs ?? 0,
    transientBlocked: inspect?.latest_orchestrator?.transient_blocked ?? 0,
  };
}

function normalizeAlert(input: ApiAlert): AlertItem {
  const severity = input.severity?.toLowerCase?.() ?? "info";
  return {
    severity: severity === "p0" ? "p0" : severity === "p1" ? "p1" : severity === "p2" ? "p2" : "info",
    kind: input.kind,
    resident: asResident(input.resident),
    message: input.message,
  };
}

function normalizeFollowupKind(kind: string): FollowupItem["kind"] | null {
  if (kind === "ticket" || kind === "ticket_reply") return "ticket";
  if (kind === "chat" || kind === "chat_reply") return "chat";
  return null;
}

function normalizeEvidence(input: ApiAcceptance | undefined): EvidenceItem[] {
  const checks = input?.checks ?? [];
  if (checks.length === 0) return [];
  return checks.slice(0, 8).map((check) => ({
    label: check.title ?? check.id,
    state: check.status === "pass" ? "ready" : check.status === "fail" ? "failed" : "pending",
  }));
}

function normalizeSystem(inspect: ApiInspect | undefined, budget: ApiBudgetStatus | undefined): OperatorTelemetry["system"] {
  const pools = inspect?.capacity?.pools ?? [];
  const poolLine = (resource: string) => {
    const pool = pools.find((item) => item.resource === resource);
    if (!pool) return `${resource}：未知`;
    return `${resource} 空闲：${pool.allocatable_free ?? pool.host_available ?? 0} ${pool.unit ?? ""}`.trim();
  };
  const risks = inspect?.resident_risk ?? [];
  return {
    capacity: [poolLine("cpu"), poolLine("memory"), poolLine("disk")],
    vms: normalizeResidentList(risks.map((item) => item.resident_id)).map((id) => `${id}：${risks.find((item) => item.resident_id === id)?.status ?? "未知"}`),
    providers: ["主线：通过后端健康检查", "请求节奏：preflight 监测", "临时挡住：0 次"],
    memory: [
      `住户自己要整理：${inspect?.memory_resident_review_queue ?? 0} 条`,
      `需要 host 出手：${inspect?.memory_operator_review_required ?? 0} 条`,
      `重复片段：${inspect?.memory_duplicate_history_groups ?? 0} 组`,
    ],
    economy: [
      `spark 总量：${Math.round(budget?.totals?.spark_balance ?? 0)}`,
      `能继续做事：${budget?.totals?.work_allowed_count ?? 0} 位`,
      `暂时卡住：${budget?.totals?.blocked_count ?? 0} 位`,
    ],
  };
}

function followupsFromInbox(inbox: ApiInbox | undefined): FollowupItem[] {
  const chat = (inbox?.pending_chat_messages ?? []).map((message) => ({
    kind: "chat" as const,
    resident: asResident(message.resident) ?? "jade",
    targetId: message.id,
    threadId: threadIDFor("chat", message.resident, message.id),
    createdAt: message.created_at ?? "",
    age: message.created_at ? ageFromNow(message.created_at) : "",
    status: "pending" as const,
    preview: message.body,
  }));
  const tickets = (inbox?.open_tickets ?? []).map((ticket) => ({
    kind: "ticket" as const,
    resident: asResident(ticket.resident) ?? "jade",
    targetId: ticket.id,
    threadId: threadIDFor("ticket", ticket.resident, ticket.id),
    createdAt: ticket.created_at ?? "",
    age: ticket.created_at ? ageFromNow(ticket.created_at) : "",
    status: "open" as const,
    priority: ticket.priority,
    preview: ticket.last_preview || ticket.title || "",
  }));
  return [...chat, ...tickets];
}

function threadsFromInbox(inbox: ApiInbox | undefined): WorldVisibleThread[] {
  const chatThreads = (inbox?.pending_chat_messages ?? []).map((message): WorldVisibleThread | null => {
    const resident = asResident(message.resident);
    if (!resident) return null;
    return {
      resident,
      threadId: threadIDFor("chat", message.resident, message.id),
      targetId: message.id,
      kind: "chat",
      messages: [messageFromThreadMessage(message)],
    };
  });
  const ticketThreads = (inbox?.open_tickets ?? []).map((ticket): WorldVisibleThread | null => {
    const resident = asResident(ticket.resident);
    if (!resident) return null;
    return {
      resident,
      threadId: threadIDFor("ticket", ticket.resident, ticket.id),
      targetId: ticket.id,
      kind: "ticket",
      messages: [{
        id: ticket.id,
        from: resident,
        createdAt: ticket.created_at ? ageFromNow(ticket.created_at) : "",
        body: ticket.last_preview || ticket.title || "",
      }],
    };
  });
  return [...chatThreads, ...ticketThreads].filter(Boolean) as WorldVisibleThread[];
}

function messageFromThreadMessage(input: ApiThreadMessage): WorldMessage {
  const resident = asResident(input.resident) ?? "jade";
  return {
    id: input.id,
    from: input.from === "chenglin" || input.direction === "chenglin_to_resident" ? "chenglin" : resident,
    createdAt: input.created_at ? ageFromNow(input.created_at) : "",
    body: input.body,
  };
}

function normalizeResidentStatus(status: string | undefined): ResidentRuntime["status"] {
  const normalized = status?.toLowerCase();
  if (normalized === "sleeping" || normalized === "resident_sleep") return "sleeping";
  if (normalized === "finished" || normalized === "ok") return "finished";
  if (normalized === "error" || normalized === "failed") return "error";
  if (normalized === "blocked") return "blocked";
  return "running";
}

function normalizeRunStatus(status: string | undefined): RunStatus {
  const normalized = status?.toLowerCase();
  if (normalized === "paused") return "paused";
  if (normalized === "failed" || normalized === "error") return "failed";
  if (normalized === "finished" || normalized === "ok") return "finished";
  return "running";
}

function normalizeLayer(layer: string | undefined): QuotaLayer {
  if (layer === "6h" || layer === "window_6h") return "6h";
  if (layer === "week") return "week";
  return "day";
}

function normalizePressure(pressure: string | undefined): Pressure {
  if (pressure === "low" || pressure === "moderate" || pressure === "high" || pressure === "critical") return pressure;
  return "low";
}

function percentRemaining(remaining = 0, cap = 0): number {
  if (cap <= 0) return 0;
  return Math.max(0, Math.min(100, (remaining / cap) * 100));
}

function asResident(value: string | undefined): ResidentId | undefined {
  if (value === "jade" || value === "amber" || value === "onyx") return value;
  return undefined;
}

function normalizeResidentList(values: Array<string | undefined> | undefined): ResidentId[] {
  const out: ResidentId[] = [];
  for (const value of values ?? []) {
    const resident = asResident(value);
    if (resident && !out.includes(resident)) out.push(resident);
  }
  return out;
}

function threadIDFor(kind: string, resident: string, targetID: string): string {
  return `${kind === "ticket" || kind === "ticket_reply" ? "ticket" : "chat"}-${resident}-${targetID}`;
}

function elapsedBetween(start: string | undefined, end: string | undefined): string {
  if (!start || !end) return "";
  const startMs = Date.parse(start);
  const endMs = Date.parse(end);
  if (Number.isNaN(startMs) || Number.isNaN(endMs) || endMs < startMs) return "";
  return humanDuration(Math.round((endMs - startMs) / 1000));
}

function ageFromNow(iso: string): string {
  const time = Date.parse(iso);
  if (Number.isNaN(time)) return iso;
  const sec = Math.max(0, Math.round((Date.now() - time) / 1000));
  return humanDuration(sec);
}

function ageClock(iso: string): string {
  const time = Date.parse(iso);
  if (Number.isNaN(time)) return "";
  const sec = Math.max(0, Math.round((Date.now() - time) / 1000));
  const min = Math.floor(sec / 60);
  return `${String(min).padStart(2, "0")}:${String(sec % 60).padStart(2, "0")}`;
}

function humanDuration(totalSec: number): string {
  if (totalSec < 60) return `${totalSec}s`;
  const min = Math.floor(totalSec / 60);
  if (min < 60) return `${min}m`;
  const hours = Math.floor(min / 60);
  const restMin = min % 60;
  if (hours < 48) return restMin > 0 ? `${hours}h${restMin}m` : `${hours}h`;
  const days = Math.floor(hours / 24);
  const restHours = hours % 24;
  return restHours > 0 ? `${days}d${restHours}h` : `${days}d`;
}

function compactDuration(duration: string): string {
  return duration.replace(/(\d+)h(\d+)m.*/, "$1h$2m").replace(/(\d+)m(\d+)s.*/, "$1m$2s");
}
