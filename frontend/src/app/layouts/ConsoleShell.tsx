import { Activity, MessageSquare, Orbit, Radio, Server, Settings, Users } from "lucide-react";
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
      <div className="ambient ambient--blue" />
      <div className="ambient ambient--green" />
      <header className="topbar">
        <div className="brand">
          <div className="brand__mark">
            <Orbit size={20} />
          </div>
          <div>
            <strong>AI ARENA</strong>
            <span>Private Operator Console</span>
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
                {item.id === "world-chat" && <em>3</em>}
              </button>
            );
          })}
        </nav>
        <div className="topbar__status">
          <span>Data mode</span>
          <strong>{dataMode}</strong>
          <i />
        </div>
      </header>
      <main className="content">{children}</main>
    </div>
  );
}
