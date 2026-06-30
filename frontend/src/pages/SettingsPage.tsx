import { Panel } from "../components/ui/Panel";
import { dataMode } from "../lib/api/client";

export function SettingsPage() {
  return (
    <div className="page-stack">
      <header className="page-header">
        <p className="eyebrow">Deployment settings</p>
        <h1>Settings</h1>
        <p>Configuration placeholders for API base path, read-only mode, auth state, and polling cadence.</p>
      </header>
      <div className="settings-grid">
        <Panel title="Data Source">
          <dl className="settings-list">
            <dt>Mode</dt>
            <dd>{dataMode}</dd>
            <dt>API base path</dt>
            <dd>{import.meta.env.VITE_API_BASE_PATH ?? "/api"}</dd>
            <dt>Frontend base path</dt>
            <dd>{import.meta.env.BASE_URL}</dd>
          </dl>
        </Panel>
        <Panel title="Write Surface">
          <dl className="settings-list">
            <dt>World replies</dt>
            <dd>Enabled through boundary-checked composer</dd>
            <dt>Run pause/resume</dt>
            <dd>Reserved for authenticated HTTP adapter</dd>
            <dt>Anonymous writes</dt>
            <dd>Must remain disabled in deployment</dd>
          </dl>
        </Panel>
      </div>
    </div>
  );
}
