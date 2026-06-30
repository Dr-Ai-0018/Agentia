import { residentAccent } from "../../data/mockConsole";
import type { ResidentId, ResidentStatus } from "../../types/domain";

export function residentLabel(resident: ResidentId): string {
  return residentAccent[resident].label;
}

export function residentTheme(resident: ResidentId) {
  return residentAccent[resident];
}

export function statusTone(status: ResidentStatus): "good" | "warning" | "danger" | "info" | "muted" {
  if (status === "running" || status === "finished") return "good";
  if (status === "sleeping") return "info";
  if (status === "blocked") return "warning";
  if (status === "error") return "danger";
  return "muted";
}
