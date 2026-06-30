import type { Pressure, Severity } from "../../types/domain";

export function severityLabel(severity: Severity): string {
  const labels: Record<Severity, string> = {
    p0: "P0",
    p1: "P1",
    p2: "P2",
    info: "Info",
  };
  return labels[severity];
}

export function severityTone(severity: Severity): string {
  const tones: Record<Severity, string> = {
    p0: "tone-danger",
    p1: "tone-warning",
    p2: "tone-info",
    info: "tone-muted",
  };
  return tones[severity];
}

export function pressureTone(pressure: Pressure): string {
  const tones: Record<Pressure, string> = {
    low: "tone-good",
    moderate: "tone-info",
    high: "tone-warning",
    critical: "tone-danger",
  };
  return tones[pressure];
}

export function quotaToneClass(value: number): string {
  if (value <= 20) return "quota-danger";
  if (value <= 40) return "quota-warning";
  return "quota-good";
}
