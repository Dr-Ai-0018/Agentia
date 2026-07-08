import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { dataMode } from "../lib/api/client";

export function SettingsPage() {
  const isMock = dataMode === "mock";
  const apiBase = import.meta.env.VITE_API_BASE_PATH ?? "/api";
  const frontendBase = import.meta.env.BASE_URL;

  return (
    <div className="settings-page">
      <header className="settings-hero">
        <h1 className="settings-hero__title">房务</h1>
        <p className="settings-hero__sub">当前生效配置（只读）。</p>
      </header>

      <section
        className={`mode-banner ${isMock ? "mode-banner--warn" : "mode-banner--good"}`}
        role={isMock ? "alert" : undefined}
      >
        {isMock ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />}
        <div className="mode-banner__body">
          <strong>
            {isMock ? "演示模式 · Mode = mock" : `真实数据 · Mode = ${dataMode}`}
          </strong>
          <p>
            {isMock
              ? "现在整个 console 展示的都是内置演示数据，不是真实 backend——住户状态、额度、疲劳、睡眠债这些数字都是编好的样本。24h 长测前必须切到 http。"
              : "连接到真实 backend，页面上的数字都是活的。"}
          </p>
        </div>
      </section>

      <section className="settings-section">
        <div className="section-title">
          <h3>数据源</h3>
          <span className="section-title__hint">前端从哪儿拉数据</span>
        </div>
        <dl className="settings-list">
          <div className="settings-list__row">
            <dt>模式</dt>
            <dd>{dataMode}</dd>
          </div>
          <div className="settings-list__row">
            <dt>API 根路径</dt>
            <dd><code>{apiBase}</code></dd>
          </div>
          <div className="settings-list__row">
            <dt>前端根路径</dt>
            <dd><code>{frontendBase}</code></dd>
          </div>
        </dl>
      </section>

      <section className="settings-section">
        <div className="section-title">
          <h3>能写的地方</h3>
          <span className="section-title__hint">世界里能出手的动作</span>
        </div>
        <dl className="settings-list">
          <div className="settings-list__row">
            <dt>世界内回话</dt>
            <dd>开着——走 boundary-checked composer。</dd>
          </div>
          <div className="settings-list__row">
            <dt>运行 暂停 / 恢复</dt>
            <dd>关着——留给认证 HTTP adapter，不给面板按钮。</dd>
          </div>
          <div className="settings-list__row">
            <dt>匿名写入</dt>
            <dd>关着——部署上永远关。</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
