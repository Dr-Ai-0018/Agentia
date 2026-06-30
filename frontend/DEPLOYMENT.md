# Deployment Notes

The intended deployment is a private web surface behind nginx, with the app served under a stable path such as `/arena/`.

## Build

```bash
cd frontend
corepack pnpm install --store-dir /root/ai-arena/.cache/pnpm-store
VITE_BASE_PATH=/arena/ VITE_CONSOLE_DATA_MODE=http VITE_API_BASE_PATH=/arena/api corepack pnpm run build
```

`VITE_BASE_PATH` controls static asset paths. `VITE_API_BASE_PATH` controls API calls.

## nginx Shape

```nginx
location /arena/api/ {
  proxy_pass http://127.0.0.1:8787/api/;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}

location /arena/ {
  try_files $uri $uri/ /arena/index.html;
}
```

The web/API server should listen on `127.0.0.1`, not directly on a public interface.

## Current skrime.killerbest.com Layout

The current host uses nginx and Certbot:

```txt
https://skrime.killerbest.com/      -> http://127.0.0.1:8787
https://skrime.killerbest.com/api/  -> http://127.0.0.1:8788
```

`127.0.0.1:8787` is currently the Vite dev server for the frontend. `127.0.0.1:8788` is `arena-console-server` managed by systemd:

```bash
systemctl status arena-console-server
journalctl -u arena-console-server -f
```

The backend service is built from:

```bash
env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go build \
  -o /root/ai-arena/bin/arena-console-server ./cmd/arena-console-server
```

The service listens with:

```txt
ARENA_CONSOLE_ADDR=127.0.0.1:8788
ARENA_ROOT=.agents
```

Smoke checks:

```bash
curl -sS https://skrime.killerbest.com/api/health
curl -sS https://skrime.killerbest.com/api/runs?limit=1
curl -sS https://skrime.killerbest.com/api/summary?limit=2
```

## Security

P0 deployment requirements:

- Basic auth, bearer auth, or same-site session auth in front of all console pages.
- All write endpoints authenticated.
- CSRF protection for browser cookie based auth.
- No auth tokens in `localStorage`.
- Browser must not read `.agents` files directly.
- Browser must not execute CLI commands directly.
- Reply/ticket writes must go through broker/worldstate audit paths.

## P0 Readiness Checklist

- Overview shows active/latest run, three resident statuses, budget, alerts, inbox preview.
- Run freshness and resident freshness can become P0/P1 watchpoints.
- Budget blocked or near-zero remaining quota is visible.
- Pending world replies are visible and open into `World Chat`.
- Reply composer only sends body plus world-safe identifiers.
- Composer does not auto-inject run/budget/telemetry/context summaries.
- Mock and HTTP adapters are separated from UI components.
- Base path works under `/arena/`.
- Evidence checklist is available for the 24h pre-release gate.
- No publish action is taken before the requested ultra-long soak.
