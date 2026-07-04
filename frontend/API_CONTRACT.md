# Operator Console API Contract

The current backend is CLI/state-file based, so this frontend uses an adapter boundary. UI components depend on `ArenaConsoleApi`, not on CLI names or JSON files.

## P0 Read Endpoints

```txt
GET /api/summary
GET /api/runs?limit=20
GET /api/runs/:runId/status
GET /api/runs/:runId/summary
GET /api/runs/:runId/report
GET /api/budget
GET /api/followups?limit=20
GET /api/inbox?limit=20
GET /api/messages/:resident?status=pending&limit=50
GET /api/messages/:resident/thread?limit=100
GET /api/tickets?resident=&status=&priority=&limit=50
GET /api/tickets/:ticketId
GET /api/system/inspect-summary
GET /api/acceptance
GET /api/acceptance/evidence?limit=20
```

## P0 Write Endpoints

```txt
POST /api/reply           (implemented)
POST /api/ticket-reply    (implemented)
POST /api/runs/:runId/pause         (deferred — no-admin soak disallows pause/resume from UI)
POST /api/runs/:runId/resume        (deferred — same as above)
POST /api/runs/:runId/retry-failed  (deferred — post v0)
```

All write endpoints require authentication (nginx basic auth + backend `X-Arena-Console-Token`) and CSRF protection or an equivalent same-site deployment control. Do not expose anonymous writes.

## Reply Boundary

`POST /api/reply` and `POST /api/ticket-reply` represent only Chenglin world replies. They must not carry telemetry, run state, budgets, token/cache metrics, operator notes, acceptance gates, hidden test rules, or auto-generated dashboard summaries.

The server picked the narrow message-id contract, so the on-wire shapes are:

```ts
// POST /api/reply
type WorldChatReplyRequest = {
  message_id: string;   // opaque server id of the message being replied to
  body: string;         // world-visible text
  boundary_ack: true;   // must be literally true; server rejects false
};

// POST /api/ticket-reply
type WorldTicketReplyRequest = {
  ticket_id: string;
  body: string;
  close: boolean;       // whether to close the ticket after replying
  boundary_ack: true;
};
```

Frontend keeps a richer `ReplyDraft` shape locally (resident id, thread id, kind, client nonce, closeTicket, boundaryAck) for UX, but the wire adapters — `buildChatReplyRequest` and `buildTicketReplyRequest` in `src/lib/reply.ts` — strip it down to the exact server shape above. **Never widen the wire shape without matching a backend change** — the narrow shape is a structural boundary that keeps operator context out of the world.

Do not design a request like:

```ts
type BadReplyRequest = {
  body: string;
  run?: unknown;
  budget?: unknown;
  telemetry?: unknown;
  context_summary?: string;
};
```

## Adapter Plan

- `MockArenaConsoleApi`: UI development fixtures.
- `HttpArenaConsoleApi`: browser-side HTTP adapter.
- `CliShimArenaConsoleApi`: optional local server-side shim that maps HTTP to `arena-broker` / `arena-orchestrator`. This must not run in the browser.

Responses should be JSON. Error shape:

```ts
type ApiError = {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
};
```

Polling defaults:

- run status: 5 seconds
- budget and inbox: 15-30 seconds
- system health: 30-60 seconds

SSE/WebSocket can be added later behind the same adapter boundary.
