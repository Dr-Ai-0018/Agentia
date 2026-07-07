import { AlertTriangle, Clock, Grid, Home, MessageSquare, RefreshCw, Settings, Users } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { dataMode } from "../../lib/api/client";

export type ConsolePage = "overview" | "residents" | "world-chat" | "runs" | "system" | "settings";

const navItems: Array<{ id: ConsolePage; label: string; icon: typeof Grid; badge?: number }> = [
  { id: "overview", label: "窗口", icon: Grid },
  { id: "residents", label: "住户", icon: Users },
  { id: "world-chat", label: "对话", icon: MessageSquare, badge: 3 },
  { id: "runs", label: "起居", icon: Clock },
  { id: "system", label: "房子", icon: Home },
  { id: "settings", label: "房务", icon: Settings },
];

const crumbs: Record<ConsolePage, { root: string; leaf: string }> = {
  overview: { root: "观察", leaf: "住户们" },
  residents: { root: "观察", leaf: "住户详情" },
  "world-chat": { root: "观察", leaf: "对话" },
  runs: { root: "记录", leaf: "起居" },
  system: { root: "记录", leaf: "房子" },
  settings: { root: "配置", leaf: "房务" },
};

type ConsoleShellProps = {
  activePage: ConsolePage;
  onNavigate: (page: ConsolePage) => void;
  children: ReactNode;
  lastFetchedAt?: Date | null;
  syncError?: string;
};

export function ConsoleShell({ activePage, onNavigate, children, lastFetchedAt, syncError }: ConsoleShellProps) {
  const crumb = crumbs[activePage];
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(id);
  }, []);
  const clock =
    `${String(now.getHours()).padStart(2, "0")}:` +
    `${String(now.getMinutes()).padStart(2, "0")}:` +
    `${String(now.getSeconds()).padStart(2, "0")}`;
  const syncAge = lastFetchedAt ? ageLabel(now, lastFetchedAt) : "还没连上";
  const syncTone = syncError ? "warn" : "ok";

  return (
    <div className="console-shell">
      <aside className="app-rail">
        <div className="app-rail__brand">
          <div className="app-rail__brand-mark">A</div>
          <div>
            <div className="app-rail__brand-name">AI Arena</div>
            <div className="app-rail__brand-sub">Observatory</div>
          </div>
        </div>
        <div className="app-rail__section-label">观察</div>
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activePage === item.id;
          return (
            <button
              key={item.id}
              type="button"
              className={`app-rail__nav-item ${isActive ? "active" : ""}`}
              onClick={() => onNavigate(item.id)}
            >
              <Icon size={16} strokeWidth={1.5} />
              <span>{item.label}</span>
              {item.badge ? <span className="app-rail__nav-badge">{item.badge}</span> : null}
            </button>
          );
        })}
        <div className="app-rail__spacer" />
        <div className="app-rail__footer">
          <div className="app-rail__who">
            <div className="app-rail__avatar">AK</div>
            <div>
              <div className="app-rail__name">aka47</div>
              <div className="app-rail__status">
                <span className="app-rail__status-dot" />
                在场
              </div>
            </div>
          </div>
        </div>
      </aside>

      <div className="app-main">
        <header className="app-topbar">
          <div className="app-topbar__crumb">
            <span>{crumb.root}</span>
            <span className="app-topbar__crumb-sep">›</span>
            <span className="app-topbar__crumb-cur">{crumb.leaf}</span>
          </div>
          <div className="app-topbar__actions">
            <div
              className={`app-topbar__sync app-topbar__sync--${syncTone}`}
              title={syncError || undefined}
            >
              {syncError ? <AlertTriangle size={12} strokeWidth={1.5} /> : <RefreshCw size={12} strokeWidth={1.5} />}
              <span>数据 · {syncAge} · {dataMode}</span>
            </div>
            <div className="app-topbar__clock">
              <div className="app-topbar__clock-label">当前时间</div>
              <div className="app-topbar__clock-val">{clock}</div>
            </div>
          </div>
        </header>

        <main className="app-content">{children}</main>
      </div>
    </div>
  );
}

function ageLabel(now: Date, then: Date): string {
  const seconds = Math.max(0, Math.round((now.getTime() - then.getTime()) / 1000));
  if (seconds < 5) return "刚刚";
  if (seconds < 60) return `${seconds} 秒前`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} 分前`;
  const hours = Math.round(minutes / 60);
  return `${hours} 小时前`;
}
