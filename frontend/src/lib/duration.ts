export function compactDuration(duration: string): string {
  const normalized = normalizeGoDuration(duration);
  return normalized
    .replace(/^(\d+)h(\d+)m.*/, "$1h$2m")
    .replace(/^(\d+)m(\d+)s.*/, "$1m$2s");
}

export function compactElapsedLabel(elapsed: string): string {
  const normalized = compactDuration(elapsed);
  const match = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(normalized);
  if (!match) return normalized;
  const parts: string[] = [];
  if (match[1]) parts.push(`${match[1]}h`);
  if (match[2] && !(match[1] && match[2] === "0")) parts.push(`${match[2]}m`);
  return parts.join(" ") || normalized;
}

function normalizeGoDuration(duration: string): string {
  return duration.trim().replace(/(\d+)\.\d+s/g, "$1s");
}
