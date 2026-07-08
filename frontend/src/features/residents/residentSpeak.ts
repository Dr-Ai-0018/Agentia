import type { FatigueMood, Pressure, QuotaLayer, ResidentRuntime, ResidentStatus, SleepDepth } from "../../types/domain";

const verbMap: Record<string, string> = {
  guest_exec: "接待来客",
  note_append: "写笔记",
  memory_review: "整理短期记忆",
  self_quota: "自查额度",
  model_stream: "在敲字",
  action_exec: "在做事",
  resident_sleep: "睡了",
  sleep: "睡了",
};

export function verbLabel(action: string): string {
  return verbMap[action] ?? action;
}

export type DoingLine = { verb: string; suffix: string };

export function describeDoing(runtime: ResidentRuntime): DoingLine {
  if (runtime.status === "sleeping") {
    if (runtime.sleepUntil) {
      return { verb: `睡到 ${formatClock(runtime.sleepUntil)}`, suffix: "" };
    }
    return { verb: "睡着", suffix: "" };
  }
  if (runtime.status === "blocked") {
    return { verb: "挡住了", suffix: "" };
  }
  if (runtime.status === "error") {
    return { verb: "出错了", suffix: "" };
  }
  if (runtime.status === "finished") {
    return { verb: "结束了", suffix: "" };
  }

  const key = runtime.lastAction || runtime.phase;
  const verb = verbMap[key] ?? "在忙";
  const suffix = runtime.inFlight ? `· 第 ${friendlyElapsed(runtime.inFlight)} 秒` : "";
  return { verb, suffix };
}

export function residentStateLabel(status: ResidentStatus): {
  text: string;
  variant: "running" | "sleeping" | "muted";
} {
  if (status === "running") return { text: "在忙", variant: "running" };
  if (status === "sleeping") return { text: "睡着", variant: "sleeping" };
  return { text: statusToText(status), variant: "muted" };
}

function statusToText(status: ResidentStatus): string {
  switch (status) {
    case "blocked":
      return "挡住";
    case "error":
      return "出错";
    case "finished":
      return "结束";
    default:
      return "在忙";
  }
}

function friendlyElapsed(inFlight: string): string {
  // inFlight like "00:06" — return "6"
  const match = /^(\d+):(\d+)$/.exec(inFlight);
  if (!match) return inFlight;
  const mins = Number(match[1]);
  const secs = Number(match[2]);
  if (mins > 0) return `${mins} 分 ${secs}`;
  return String(secs);
}

function formatClock(iso: string): string {
  // Accept either ISO string or already-formatted "14:48"
  const trimmed = iso.trim();
  if (/^\d{1,2}:\d{2}(:\d{2})?$/.test(trimmed)) return trimmed.slice(0, 5);
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) return trimmed;
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

export function pressureToQuotaTone(pressure: Pressure): "ok" | "mid" | "warn" {
  if (pressure === "low") return "ok";
  if (pressure === "moderate") return "mid";
  return "warn";
}

export function layerWindowLabel(layer: QuotaLayer): string {
  if (layer === "6h") return "近 6h";
  if (layer === "day") return "今日";
  return "本周";
}

// forecast 是"按当前速率还够多少时间"的口语估算。
// 前端只拿百分比，乘上 layer 时长，得到剩余人时。粗略、不承诺精度。
export function forecastFromRemainingPct(pct: number, layer: QuotaLayer): string {
  const layerHours = layer === "6h" ? 6 : layer === "day" ? 24 : 168;
  const remainHours = Math.max(0, (layerHours * pct) / 100);
  return humanizeHours(remainHours);
}

function humanizeHours(hours: number): string {
  if (hours < 1) {
    const mins = Math.round(hours * 60);
    return `~ ${mins} 分`;
  }
  if (hours < 24) {
    const h = Math.floor(hours);
    const m = Math.round((hours - h) * 60);
    if (m === 0) return `~ ${h}h`;
    return `~ ${h}h${String(m).padStart(2, "0")}`;
  }
  const days = hours / 24;
  if (days < 10) return `~ ${days.toFixed(1)}d`;
  return `~ ${Math.round(days)}d`;
}

// Feel-words for fatigue. Deliberately no numbers — residents and
// operators should read this like a person's state, not a gauge.
// Each tier has multiple variants; a stable hash of (resident, tier)
// picks one so the same resident says the same variant for a given
// mood (personality-like), but different residents in the same mood
// say different sentences (not machine-flat).
const fatigueMoodVariants: Record<FatigueMood, string[]> = {
  fresh: ["精力充沛", "状态刚刚好", "今天挺来劲的"],
  warming_up: ["热了点身", "刚进入状态", "开始有节奏了"],
  some_tiredness: ["有点累", "开始有点乏", "肩膀有点沉"],
  quite_tired: ["很累了，该歇歇", "扛不住多久了", "眼睛开始涩了"],
  exhausted: ["撑不住了，得睡了", "整个人都空了", "再干下去要出错了"],
};

export function fatigueMoodLabel(mood: FatigueMood, resident?: string): string {
  return pickVariant(fatigueMoodVariants[mood], resident, `mood:${mood}`);
}

// Fallback if backend only sends a raw fatigue level (0-100) and not a mood.
export function fatigueMoodFromLevel(level: number): FatigueMood {
  if (level < 20) return "fresh";
  if (level < 40) return "warming_up";
  if (level < 60) return "some_tiredness";
  if (level < 80) return "quite_tired";
  return "exhausted";
}

const sleepDepthVariants: Record<SleepDepth, string[]> = {
  awake: ["醒着"],
  rest: ["在歇一会儿", "在小憩", "闭眼缓一缓"],
  sleep: ["在睡", "正在睡", "睡着了"],
  deep_sleep: ["深度睡眠", "睡得很沉", "深睡中"],
};

export function sleepDepthLabel(depth: SleepDepth, resident?: string): string {
  return pickVariant(sleepDepthVariants[depth], resident, `sleep:${depth}`);
}

// Sleep debt hint. Only surfaces above 2h to avoid nagging the operator
// about normal minor debt accumulation.
const sleepDebtVariants = {
  mild: [
    "最近睡得不够，恢复慢一点",
    "这几晚都没睡够，白天有点飘",
    "睡眠账刚开始欠一点",
  ],
  heavy: [
    "欠了不少觉，得连着睡几天才能追回来",
    "睡眠债堆得有点厚，一觉补不完",
    "这一阵子觉都不够，得慢慢还",
  ],
} as const;

export function sleepDebtHint(debtHours: number, resident?: string): string | null {
  if (debtHours < 2) return null;
  const tier = debtHours < 4 ? "mild" : "heavy";
  return pickVariant(sleepDebtVariants[tier], resident, `debt:${tier}`);
}

// pickVariant chooses one string deterministically from a list based on a
// seed string. Same seed → same choice, so a given resident in a given
// mood always shows the same variant (feels personality-like), but two
// residents in the same mood pick different variants.
function pickVariant(list: readonly string[], resident: string | undefined, tierKey: string): string {
  if (list.length === 0) return "";
  if (list.length === 1) return list[0];
  const seed = `${resident ?? "any"}::${tierKey}`;
  let hash = 0;
  for (let i = 0; i < seed.length; i += 1) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  }
  const idx = Math.abs(hash) % list.length;
  return list[idx];
}
