import type { ReactNode } from "react";

type BadgeProps = {
  children: ReactNode;
  tone?: "good" | "warning" | "danger" | "info" | "muted";
};

export function Badge({ children, tone = "muted" }: BadgeProps) {
  return <span className={`badge tone-${tone}`}>{children}</span>;
}
