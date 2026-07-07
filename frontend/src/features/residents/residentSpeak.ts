import type { Pressure, QuotaLayer, ResidentRuntime, ResidentStatus } from "../../types/domain";

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
