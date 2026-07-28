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
  SummaryPane,
  SummaryPaneEvidenceRef,
  WorldChatReplyRequest,
  WorldChatRequest,
  WorldMessage,
  WorldThreadPage,
  WorldTicket,
  WorldTicketSummary,
  WorldTicketReplyRequest,
  WorldVisibleThread,
} from "../../types/domain";
import { fatigueMoodFromLevel } from "../../features/residents/residentSpeak";
import { compactDuration } from "../duration";
import type { ArenaConsoleApi } from "./types";

type HttpOptions = {
  basePath: string;
};

async function parseJson<T = any>(response: Response): Promise<T> {
  if (!response.ok) {
    const text = await response.text();
    let code = "";
    try {
      code = (JSON.parse(text) as { error?: { code?: string } }).error?.code ?? "";
    } catch {
      // Keep the fallback below for non-JSON failures from a proxy or server.
    }
    const friendlyMessage: Record<string, string> = {
      chat_failed: "消息发送失败，请稍后重试；草稿已保留。",
      reply_failed: "回复发送失败，请稍后重试；草稿已保留。",
      chat_rate_limited: "发送得太快了，请稍等片刻再试；草稿已保留。",
    };
    throw new Error(friendlyMessage[code] ?? (text || `HTTP ${response.status}`));
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
  target_duration_seconds?: number;
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
  summary_pane?: ApiSummaryPane;
};

type ApiSummaryPaneEvidenceRef = {
  kind?: string;
  ref?: string;
  rounds?: number[];
};

type ApiSummaryPane = {
  text?: string;
  updated_at?: string;
  rounds_absorbed?: number;
  approx_tokens?: number;
  evidence_refs?: ApiSummaryPaneEvidenceRef[];
};

type ApiRunRecord = {
  run_id: string;
  status: string;
  mode: "parallel" | "sequential";
  residents?: string[];
  started_at?: string;
  updated_at?: string;
  finished_at?: string;
  target_duration_seconds?: number;
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
  six_hour_burn?: number[];
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
  to?: string;
  body: string;
  created_at?: string;
  status?: string;
  reply_to_id?: string;
  read_at?: string;
  body_integrity?: string;
};

type ApiThreadSummary = {
  resident: string;
  last_message_at?: string;
  last_direction?: string;
  last_status?: string;
  last_preview?: string;
  pending_count?: number;
  replied_count?: number;
  delivered_count?: number;
  legacy_truncated_count?: number;
};

type ApiThreadPage = {
  resident: string;
  messages?: ApiThreadMessage[];
  total?: number;
  has_more?: boolean;
  next_before?: string;
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
  last_reply_at?: string;
  reply_count?: number;
  needs_reply?: boolean;
};

type ApiTicket = {
  id: string;
  resident: string;
  title?: string;
  body?: string;
  priority?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
  opened_by?: string;
  replies?: Array<{ id: string; from?: string; body?: string; created_at?: string }>;
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
  maintenance?: Record<string, string>;
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

  async getSummary(signal?: AbortSignal) {
    return normalizeSummary(await parseJson<ApiSummary>(await fetch(this.url("/summary"), { signal })));
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

  async listWorldThreads(signal?: AbortSignal) {
    const summaries = await parseJson<ApiThreadSummary[]>(await fetch(this.url("/threads"), { signal }));
    return Promise.all(summaries.map(async (summary) => {
      const resident = asResident(summary.resident);
      if (!resident) throw new Error(`Unknown resident in thread summary: ${summary.resident}`);
      const page = await this.getWorldThreadPage(resident, undefined, 50, signal);
      return threadFromSummary(summary, page);
    }));
  }

  async getWorldThread(threadId: string, signal?: AbortSignal) {
    const resident = residentFromThreadID(threadId);
    const summaries = await parseJson<ApiThreadSummary[]>(await fetch(this.url("/threads"), { signal }));
    const summary = summaries.find((item) => item.resident === resident);
    if (!summary) throw new Error(`World thread not found: ${threadId}`);
    return threadFromSummary(summary, await this.getWorldThreadPage(resident, undefined, 50, signal));
  }

  async getWorldThreadPage(resident: ResidentId, before?: string, limit = 50, signal?: AbortSignal): Promise<WorldThreadPage> {
    const query = new URLSearchParams({ limit: String(limit) });
    if (before) query.set("before", before);
    return normalizeThreadPage(await parseJson<ApiThreadPage>(
      await fetch(this.url(`/messages/${resident}/thread?${query.toString()}`), { signal }),
    ));
  }

  async getResidentThreads(resident: ResidentId) {
    return (await this.listWorldThreads()).filter((thread) => thread.resident === resident);
  }

  async sendWorldChatReply(input: WorldChatReplyRequest) {
    return messageFromThreadMessage(await parseJson<ApiThreadMessage>(
      await fetch(this.url("/reply"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    ));
  }

  async sendWorldChat(input: WorldChatRequest) {
    return messageFromThreadMessage(await parseJson<ApiThreadMessage>(
      await fetch(this.url("/chat"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    ));
  }

  async sendWorldTicketReply(input: WorldTicketReplyRequest) {
    return normalizeTicket(await parseJson<ApiTicket>(
      await fetch(this.url("/ticket-reply"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    ));
  }

  async listTickets(filters: { resident?: ResidentId; status?: string; priority?: string; limit?: number } = {}) {
    const query = new URLSearchParams({ limit: String(filters.limit ?? 100) });
    if (filters.resident) query.set("resident", filters.resident);
    if (filters.status) query.set("status", filters.status);
    if (filters.priority) query.set("priority", filters.priority);
    const tickets = await parseJson<ApiTicketSummary[]>(await fetch(this.url(`/tickets?${query.toString()}`)));
    return tickets.flatMap((ticket) => {
      const normalized = normalizeTicketSummary(ticket);
      return normalized ? [normalized] : [];
    });
  }

  async getTicket(ticketId: string) {
    return normalizeTicket(await parseJson<ApiTicket>(await fetch(this.url(`/tickets/${encodeURIComponent(ticketId)}`))));
  }

  async getPreflight() {
    return parseJson(await fetch(this.url("/preflight")));
  }

  async getCompactionDiagnostics() {
    // This endpoint already returns camelCase JSON matching the frontend
    // CompactionDiagnostics type, so no normalization layer is needed.
    return parseJson(await fetch(this.url("/diagnostics/compaction")));
  }
}

function normalizeTicketSummary(input: ApiTicketSummary): WorldTicketSummary | null {
  const resident = asResident(input.resident);
  if (!resident) return null;
  return {
    id: input.id,
    resident,
    title: input.title ?? "未命名单据",
    priority: normalizeTicketPriority(input.priority),
    status: normalizeTicketStatus(input.status),
    createdAt: input.created_at ?? "",
    updatedAt: input.updated_at ?? input.created_at ?? "",
    lastReplyAt: input.last_reply_at || undefined,
    lastPreview: input.last_preview ?? "",
    replyCount: input.reply_count ?? 0,
    needsReply: input.needs_reply ?? input.status === "open",
  };
}

function normalizeTicket(input: ApiTicket): WorldTicket {
  const resident = asResident(input.resident);
  if (!resident) throw new Error(`Unknown resident in ticket: ${input.resident}`);
  return {
    id: input.id,
    resident,
    title: input.title ?? "未命名单据",
    body: input.body ?? "",
    priority: normalizeTicketPriority(input.priority),
    status: normalizeTicketStatus(input.status),
    createdAt: input.created_at ?? "",
    updatedAt: input.updated_at ?? input.created_at ?? "",
    openedBy: input.opened_by ?? resident,
    replies: (input.replies ?? []).map((reply) => ({
      id: reply.id,
      from: reply.from === "chenglin" ? "chenglin" : resident,
      body: reply.body ?? "",
      createdAt: reply.created_at ?? "",
    })),
  };
}

function normalizeTicketStatus(value: string | undefined): WorldTicket["status"] {
  if (value === "answered" || value === "closed") return value;
  return "open";
}

function normalizeTicketPriority(value: string | undefined): WorldTicket["priority"] {
  if (value === "low" || value === "high" || value === "urgent") return value;
  return "medium";
}

function normalizeSummary(input: ApiSummary): OperatorTelemetry {
  const activeRun = normalizeActiveRun(input.active_run, input.latest_run, input.inspect);
  const budgets = (input.budget?.residents ?? []).map(normalizeBudget).filter(Boolean) as ResidentBudget[];
  const residents = normalizeResidents(input.active_run?.residents, budgets, input.inspect, Boolean(input.active_run));
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
    tightestLayer: "day",
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
  const targetDurationSeconds = active?.target_duration_seconds ?? latest?.target_duration_seconds ?? 0;
  const residents = normalizeResidentList(
    active?.residents?.map((item) => item.resident) ?? latest?.residents ?? inspect?.latest_orchestrator?.residents_planned,
  );
  return {
    isLive: Boolean(active),
    runId: source?.run_id ?? "no-active-run",
    purpose: active ? "观察中" : "最近一次观察",
    status: normalizeRunStatus(source?.status),
    mode: source?.mode ?? "parallel",
    startedAt,
    updatedAt,
    elapsed: compactDuration(duration),
    targetDuration: formatTargetDuration(targetDurationSeconds),
    expectedEndAt: active ? expectedEnd(startedAt, targetDurationSeconds) : "",
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
  hasLiveRun: boolean,
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
      status: hasLiveRun ? normalizeResidentStatus(active?.status ?? risk?.status) : "idle",
      phase: active?.current_phase ?? "",
      round: active?.current_round ?? 0,
      lastAction: active?.last_action ?? "",
      lastUpdateAge: active?.updated_at ? ageFromNow(active.updated_at) : "",
      inFlight: active?.in_flight_started_at ? ageClock(active.in_flight_started_at) : "",
      totalInputTokens: active?.total_input_tokens ?? 0,
      totalCachedTokens: active?.total_cached_tokens ?? 0,
      totalOutputTokens: active?.total_output_tokens ?? 0,
      summaryPane: normalizeSummaryPane(active?.summary_pane),
    };
  });
}

function normalizeSummaryPane(input: ApiSummaryPane | undefined): SummaryPane | undefined {
  if (!input || !input.text) return undefined;
  return {
    text: input.text,
    updatedAt: input.updated_at ?? "",
    roundsAbsorbed: input.rounds_absorbed ?? 0,
    approxTokens: input.approx_tokens ?? 0,
    evidenceRefs: normalizeEvidenceRefs(input.evidence_refs),
  };
}

function normalizeEvidenceRefs(input: ApiSummaryPaneEvidenceRef[] | undefined): SummaryPaneEvidenceRef[] | undefined {
  if (!input || input.length === 0) return undefined;
  const refs = input
    .map((raw): SummaryPaneEvidenceRef | null => {
      if (!raw || !raw.ref) return null;
      const kind = normalizeEvidenceKind(raw.kind);
      if (!kind) return null;
      return {
        kind,
        ref: raw.ref,
        rounds: Array.isArray(raw.rounds) ? raw.rounds.filter((n) => typeof n === "number") : undefined,
      };
    })
    .filter(Boolean) as SummaryPaneEvidenceRef[];
  return refs.length > 0 ? refs : undefined;
}

function normalizeEvidenceKind(kind: string | undefined): SummaryPaneEvidenceRef["kind"] | null {
  if (kind === "round" || kind === "note" || kind === "guest_artifact") return kind;
  return null;
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
    sixHourBurn: normalizeSixHourBurn(input.six_hour_burn),
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

function normalizeSixHourBurn(input: number[] | undefined): number[] | undefined {
  if (!Array.isArray(input)) return undefined;
  return input.map((sample) => Math.max(0, Math.round(Number.isFinite(sample) ? sample : 0)));
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
      threadId: kind === "intervention" ? undefined : threadIDFor(kind, item.resident, item.target_id),
      createdAt: item.created_at ?? "",
      updatedAt: item.updated_at || undefined,
      age: item.created_at ? ageFromNow(item.created_at) : "",
      status: normalizeFollowupStatus(item.status, kind),
      priority: item.priority,
      title: item.title || undefined,
      maintenance: item.maintenance,
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
    message: normalizeAlertMessage(input),
  };
}

function normalizeAlertMessage(input: ApiAlert): string {
  return input.message;
}

function normalizeFollowupKind(kind: string): FollowupItem["kind"] | null {
  if (kind === "ticket" || kind === "ticket_reply") return "ticket";
  if (kind === "chat" || kind === "chat_reply") return "chat";
  if (kind === "host_intervention" || kind === "intervention") return "intervention";
  return null;
}

function normalizeFollowupStatus(status: string | undefined, kind: FollowupItem["kind"]): FollowupItem["status"] {
  switch (status) {
    case "pending":
    case "open":
    case "replied":
    case "closed":
    case "planned":
    case "in_progress":
    case "failed":
    case "rolled_back":
      return status;
    default:
      return kind === "ticket" ? "open" : kind === "chat" ? "pending" : "unknown";
  }
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

export function messageFromThreadMessage(input: ApiThreadMessage): WorldMessage {
  const resident = asResident(input.resident) ?? "jade";
  const from = input.from === "chenglin" || input.direction === "chenglin_to_resident" ? "chenglin" : resident;
  return {
    id: input.id,
    from,
    to: from === "chenglin" ? resident : "chenglin",
    direction: from === "chenglin" ? "chenglin_to_resident" : "resident_to_chenglin",
    createdAt: input.created_at ?? "",
    body: input.body,
    status: normalizeMessageStatus(input.status, from),
    replyToId: input.reply_to_id || undefined,
    readAt: input.read_at || undefined,
    bodyIntegrity: input.body_integrity === "legacy_truncated" ? "legacy_truncated" : undefined,
  };
}

function normalizeThreadPage(input: ApiThreadPage): WorldThreadPage {
  const resident = asResident(input.resident);
  if (!resident) throw new Error(`Unknown resident in thread page: ${input.resident}`);
  return {
    resident,
    messages: (input.messages ?? []).map(messageFromThreadMessage),
    total: input.total ?? 0,
    hasMore: input.has_more ?? false,
    nextBefore: input.next_before || undefined,
  };
}

function threadFromSummary(summary: ApiThreadSummary, page: WorldThreadPage): WorldVisibleThread {
  const target = [...page.messages].reverse().find((message) => message.status === "pending");
  return {
    resident: page.resident,
    threadId: `chat-${page.resident}`,
    targetId: target?.id,
    kind: "chat",
    messages: page.messages,
    total: page.total,
    pendingCount: summary.pending_count ?? 0,
    repliedCount: summary.replied_count ?? 0,
    deliveredCount: summary.delivered_count ?? 0,
    legacyTruncatedCount: summary.legacy_truncated_count ?? 0,
    hasMore: page.hasMore,
    nextBefore: page.nextBefore,
    lastMessageAt: summary.last_message_at,
    lastPreview: summary.last_preview,
  };
}

function residentFromThreadID(threadId: string): ResidentId {
  const resident = asResident(threadId.replace(/^chat-/, ""));
  if (!resident) throw new Error(`Invalid world thread id: ${threadId}`);
  return resident;
}

function normalizeMessageStatus(status: string | undefined, from: WorldMessage["from"]): WorldMessage["status"] {
  if (status === "pending" || status === "replied" || status === "delivered") return status;
  return from === "chenglin" ? "delivered" : "pending";
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
  if (normalized === "abandoned" || normalized === "stale") return "abandoned";
  if (normalized === "failed" || normalized === "error" || normalized === "finished_with_errors") return "failed";
  if (normalized === "finished" || normalized === "ok" || normalized === "finished_with_transient_blocks") return "finished";
  return "running";
}

function normalizeLayer(layer: string | undefined): QuotaLayer {
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

function expectedEnd(start: string | undefined, targetDurationSeconds: number): string {
  if (!start || targetDurationSeconds <= 0) return "";
  const startMs = Date.parse(start);
  if (Number.isNaN(startMs)) return "";
  return new Date(startMs + targetDurationSeconds * 1000).toISOString();
}

function formatTargetDuration(targetDurationSeconds: number): string {
  if (targetDurationSeconds <= 0) return "未标明";
  return humanDuration(targetDurationSeconds);
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
