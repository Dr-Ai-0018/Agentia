import { Activity, MessageSquare, MonitorCog, Radio, Server, Settings, Users } from "lucide-react";
import type { ReactNode } from "react";
import { dataMode } from "../../lib/api/client";

export type ConsolePage = "overview" | "residents" | "world-chat" | "runs" | "system" | "settings";

const navItems: Array<{ id: ConsolePage; label: string; icon: typeof Activity }> = [
  { id: "overview", label: "Overview", icon: Activity },
  { id: "residents", label: "Residents", icon: Users },
  { id: "world-chat", label: "World Chat", icon: MessageSquare },
  { id: "runs", label: "Runs", icon: Radio },
  { id: "system", label: "System", icon: Server },
  { id: "settings", label: "Settings", icon: Settings },
];

type ConsoleShellProps = {
  activePage: ConsolePage;
  onNavigate: (page: ConsolePage) => void;
  children: ReactNode;
};

export function ConsoleShell({ activePage, onNavigate, children }: ConsoleShellProps) {
  return (
    <div className="console-shell">
      <aside className="sidebar">
        <div className="brand">
          <MonitorCog size={24} />
          <div>
            <strong>AI Arena</strong>
            <span>Operator Console</span>
          </div>
        </div>
        <nav className="nav">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.id}
                className={activePage === item.id ? "active" : ""}
                type="button"
                onClick={() => onNavigate(item.id)}
              >
                <Icon size={17} />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
        <div className="sidebar__footer">
          <span>Data mode</span>
          <strong>{dataMode}</strong>
        </div>
      </aside>
      <main className="content">{children}</main>
    </div>
  );
}
