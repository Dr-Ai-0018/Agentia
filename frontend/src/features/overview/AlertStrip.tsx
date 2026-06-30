import { AlertTriangle } from "lucide-react";
import { severityLabel, severityTone } from "../../lib/domain/severity";
import type { AlertItem } from "../../types/domain";
import { residentLabel } from "../residents/residentTheme";

export function AlertStrip({ alerts }: { alerts: AlertItem[] }) {
  return (
    <div className="alert-list">
      {alerts.map((alert) => (
        <div className={`alert-row ${severityTone(alert.severity)}`} key={`${alert.kind}-${alert.resident ?? "system"}`}>
          <AlertTriangle size={16} />
          <strong>{severityLabel(alert.severity)}</strong>
          {alert.resident && <span>{residentLabel(alert.resident)}</span>}
          <p>{alert.message}</p>
        </div>
      ))}
    </div>
  );
}
