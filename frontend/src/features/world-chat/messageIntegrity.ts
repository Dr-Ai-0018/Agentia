import type { WorldMessage } from "../../types/domain";

export function visibleWorldMessageBody(message: Pick<WorldMessage, "body" | "bodyIntegrity">): string {
  if (message.bodyIntegrity !== "legacy_truncated") return message.body;
  return message.body
    .replace(/\.\.\.$/u, "")
    .replace(/[�…]{1,2}$/u, "")
    .trimEnd();
}
