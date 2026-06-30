type StatusDotProps = {
  tone?: "good" | "warning" | "danger" | "info" | "muted";
  pulse?: boolean;
};

export function StatusDot({ tone = "muted", pulse = false }: StatusDotProps) {
  return <span className={`status-dot status-dot--${tone} ${pulse ? "status-dot--pulse" : ""}`} />;
}
