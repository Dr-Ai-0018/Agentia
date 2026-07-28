import type { ConsolePage } from "./layouts/ConsoleShell";

const SUMMARY_POLL_PAGES = new Set<ConsolePage>(["overview", "residents", "runs", "system"]);

export function shouldPollSummary(page: ConsolePage): boolean {
  return SUMMARY_POLL_PAGES.has(page);
}
