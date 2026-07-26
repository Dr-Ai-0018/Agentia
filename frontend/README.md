# AI Arena Operator Console

This is the tracked production-direction frontend for the AI Arena v0 operator console. It is not a throwaway demo. The first release target is an authenticated private dashboard for the 24h no-admin autonomy soak before v0 launch.

The console has two separate domains:

- Observation: run freshness, resident runtime, quota pressure, spark balance, inbox preview, evidence, host/provider/cache health.
- World Chat: the only UI surface that can send Chenglin replies into the world.

World Chat is intentionally separated from telemetry. Reply components receive only world-visible thread data and a reply draft. They must not receive run ids, budget internals, cache/token metrics, acceptance gate facts, or operator notes.

## Local Development

```bash
cd frontend
corepack pnpm install --store-dir /root/ai-arena/.cache/pnpm-store
corepack pnpm run dev
```

The dev server listens on `127.0.0.1:8787`.

By default the app uses the mock adapter:

```bash
VITE_CONSOLE_DATA_MODE=mock corepack pnpm run dev
```

HTTP mode against the current root-path API:

```bash
VITE_CONSOLE_DATA_MODE=http VITE_API_BASE_PATH=/api corepack pnpm run dev
```

## Pages

- `Overview`: active run header, resident cards, telemetry chart, event stream, quota matrix, inbox preview, watchpoints.
- `Residents`: resident runtime and budget detail.
- `World Chat`: isolated Chenglin reply surface using world-visible threads only.
- `Runs`: run registry and evidence checklist.
- `System`: operator-only health, provider, memory, and economy signals.
- `Settings`: base path, data source, and write-surface placeholders.

## Guardrails

- Do not import telemetry, budget, run, system, or evidence modules into `features/world-chat`.
- Do not add automatic dashboard-derived reply generation in P0.
- Do not let `POST /api/reply` carry run/budget/token/cache/system fields.
- Do not expose anonymous write endpoints in deployment.
- Keep mock, HTTP, and future CLI-shim adapters behind `ArenaConsoleApi`.
