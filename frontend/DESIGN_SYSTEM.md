# Design System

The console should feel like a private observation deck for a living AI world: dark, dense, glassy, and precise. It should not feel like a marketing landing page, a game leaderboard, or a military command interface.

## Visual Direction

- Dark background with restrained atmospheric light.
- Glass panels with low opacity and 8px radius for operational surfaces.
- Strong information density, especially for budget, resident status, run state, and evidence.
- Resident colors are identity markers, not ranking or ownership signals.
- Blue is informational, green is healthy/running, amber is attention/pending, red is blocked/stale/danger.
- Purple is not a primary palette.

## Copy

Use observation language:

- `Operator Console`
- `Overview`
- `World Chat`
- `Watchpoints`
- `Observed Events`
- `Attention needed`
- `Quota pressure`
- `Send as Chenglin`

Avoid control or military language:

- `Mission Control`
- `Command Center`
- `Intervention`
- `Transmit`
- `Secure Channel`
- `Target`
- `Unit`
- `Admin Control`

## Layout

Primary navigation:

```txt
Overview / Residents / World Chat / Runs / System / Settings
```

`World Chat` is a separate page. Do not place reply composer beside telemetry or system panels in the same context.

Recommended page content:

- `Overview`: active run, resident status cards, watchpoints, quota matrix, telemetry chart, event stream, inbox preview.
- `Residents`: runtime, action, quota, spark, pacing state per resident.
- `World Chat`: thread list, world-visible messages, reply composer, boundary notice.
- `Runs`: registry, status, evidence checklist.
- `System`: host/provider/cache/memory/operator-only state.
- `Settings`: base path, data mode, auth/read-only/write-surface state.

## Component Rules

- Use shared UI primitives for panels, badges, status dots, progress bars, charts.
- Keep cards at 8px radius unless used as a high-level hero container.
- Use tables or grids for dense metrics.
- Do not nest decorative cards inside cards.
- Avoid component-local color conditionals; use domain helpers such as `severity.ts` and `residentTheme.ts`.
- Keep text containers responsive and use `overflow-wrap` or ellipsis for long run ids and previews.

## World Chat Rules

`features/world-chat` must not import telemetry, budget, run, system, or evidence feature modules. The composer props are limited to:

- `ReplyDraft`
- callbacks for draft change and submit
- send state

Boundary copy should be short and visible, but not dominate the chat experience.
