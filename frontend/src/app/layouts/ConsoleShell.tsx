import { Clock, Grid, Home, MessageSquare, RefreshCw, Server, Settings, Users } from "lucide-react";
import type { ReactNode } from "react";
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
};

export function ConsoleShell({ activePage, onNavigate, children }: ConsoleShellProps) {
  const crumb = crumbs[activePage];
  const now = new Date();
  const clock =
    `${String(now.getHours()).padStart(2, "0")}:` +
    `${String(now.getMinutes()).padStart(2, "0")}:` +
    `${String(now.getSeconds()).padStart(2, "0")}`;

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
            <div className="app-topbar__sync">
              <RefreshCw size={12} strokeWidth={1.5} />
              <span>数据 · 刚刚 · {dataMode}</span>
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
