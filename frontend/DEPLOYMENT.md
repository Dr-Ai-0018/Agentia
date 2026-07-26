# Deployment Notes

This console is currently served as a private root-path web surface behind nginx.

Current public host:

```txt
https://skrime.killerbest.com/
```

The frontend is a static Vite build. nginx serves `frontend/dist` directly. Running a production build in this directory updates the live static artifact on the current host.

## Production Build

From the repository root:

```bash
VITE_CONSOLE_DATA_MODE=http VITE_API_BASE_PATH=/api npm --prefix frontend run build
```

Equivalent command from `frontend/`:

```bash
VITE_CONSOLE_DATA_MODE=http VITE_API_BASE_PATH=/api corepack pnpm run build
```

Important:

- `VITE_CONSOLE_DATA_MODE=http` is required for the deployed artifact.
- `VITE_API_BASE_PATH=/api` is the current root-path API route.
- Do not use mock mode for the public artifact.
- Do not set `/arena/` unless nginx is intentionally changed to serve the app under that subpath.
- `VITE_BASE_PATH` defaults to `/`, which is correct for the current host.

## Current nginx Shape

The current production shape is:

```txt
/      -> static files from /root/ai-arena/frontend/dist
/api/  -> arena-console-server on 127.0.0.1:8788
```

The redacted nginx behavior is:

```nginx
location /api/ {
  proxy_pass http://127.0.0.1:8788/api/;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  # The real config must set X-Arena-Console-Token from nginx-owned secret
  # material. Do not paste secret values into this repository.
}

location / {
  try_files $uri $uri/ /index.html;
}
```

Keep all public console pages behind nginx authentication. Do not print or commit nginx secret values.

## Local Development

The Vite dev server is for local development only:

```bash
npm --prefix frontend run dev
```

It listens on `127.0.0.1:8787`. Do not use the Vite dev server as the production frontend upstream for the current host.

Local mock mode:

```bash
VITE_CONSOLE_DATA_MODE=mock npm --prefix frontend run dev
```

Local HTTP mode against the current root-path API:

```bash
VITE_CONSOLE_DATA_MODE=http VITE_API_BASE_PATH=/api npm --prefix frontend run dev
```

## Backend Service

`arena-console-server` is expected to listen on loopback only:

```txt
ARENA_CONSOLE_ADDR=127.0.0.1:8788
ARENA_ROOT=.agents
```

Build command:

```bash
env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go build \
  -o /root/ai-arena/bin/arena-console-server ./cmd/arena-console-server
```

Operational checks:

```bash
systemctl status arena-console-server
journalctl -u arena-console-server -f
```

Do not edit or restart the service during launch cleanup without fresh host approval.

## Smoke Checks

Public unauthenticated checks should return 401:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' https://skrime.killerbest.com/
curl -sS -o /dev/null -w '%{http_code}\n' https://skrime.killerbest.com/api/health
```

Authenticated browser review should confirm:

- no mock or demo banner;
- Settings shows HTTP mode;
- page navigation emits `/api/summary`, `/api/inbox`, `/api/preflight`, and `/api/diagnostics/compaction` requests;
- API failures surface as operator-facing errors, not silent mock fallbacks.

Backend direct token checks may be run locally, but command output must not print token values.

## Security Requirements

- Keep basic auth or equivalent authentication in front of all console pages.
- Keep backend upstream token handling in nginx/server configuration, not frontend code.
- Do not put auth tokens in `localStorage`.
- Browser code must not read `.agents` files directly.
- Browser code must not execute CLI commands directly.
- Reply/ticket writes must go through broker/worldstate audit paths.
- Do not expose anonymous write endpoints.

## P0 Readiness Checklist

- Public frontend is an explicit HTTP build, not mock mode.
- Public unauthenticated console/API requests return 401.
- Authenticated browser navigation calls the real `/api` endpoints.
- Overview does not show stale persisted runs as live observations.
- Inbox and world-chat previews show valid text without replacement characters.
- Reply composer only sends body plus world-safe identifiers.
- Composer does not auto-inject run, budget, telemetry, context, or system summaries.
- Evidence checklist is available for the 24h pre-release gate.
- No publish action is taken before the requested ultra-long soak and host approval.
