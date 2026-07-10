# ONT Lock Ops Console Design

**Date:** 2026-07-10  
**Status:** Approved  
**Goal:** Make ONT Lock easier to scan and operate by splitting the page into task-based tabs, adding Overview KPIs/chart, and paginating Activity lists.

## Problem

The previous single-page layout stacked Config, Whitelist, Policies, Unauthorized, Commands, Unsupported, and Audit. Side-by-side Unauthorized/Commands clipped columns; operators could not focus on one task; Commands/Audit silently capped at 100/200 rows with no pagination UI.

## Decisions

| Topic | Choice |
|-------|--------|
| IA | Top Tabs: Overview / Policies / Exceptions / Activity (`?tab=queue` redirects to `exceptions`) |
| URL | `?tab=overview\|policies\|queue\|activity` |
| Overview | Config + KPI strip + live column chart + GSAP entrance |
| Activity pagination | Server `page_number` + `page_size`, Devices-style `TablePagination` |
| Commands Clear | Added (confirm dialog); clears completed (success/failed) attempts only — pending/retry preserved for the retry scheduler; logs `commands_clear` audit |
| Commands Export | Current page rows only |
| Visual language | Existing MUI / Oktopus theme |

## Tab contents

### Overview

- Master / AutoLock switches (unchanged API)
- KPI cards: Unauthorized count, Unsupported count, Commands success count, Commands failed count (from recent commands window used for the chart)
- Column chart of command status distribution (live ApexCharts; GSAP for KPI stagger/count-up only)
- `gsap.matchMedia` honors `prefers-reduced-motion`

### Policies

- Full-width policies table, search, Add Whitelist, Import CSV
- Client-side `TablePagination` (default 25)

### Exceptions (formerly Queue)

- Unauthorized (search, select-all current page, multi-select whitelist) then Unsupported (opt-out / resume)
- Full-width; Unauthorized client pagination (default 25)
- Reason column shows friendly labels mapped from engine codes

### Activity

- Recent Commands (search, Export CSV, Clear with confirm, server pagination)
- Audit History (Clear with confirm, server pagination)

## API

### `GET /api/tenants/{slug}/lock/commands`

Query: `page_number` (0-based, default 0), `page_size` (default 25, max 100), optional `sn`.

Response:

```json
{ "items": [], "total": 0, "page": 0, "size": 25 }
```

### `DELETE /api/tenants/{slug}/lock/commands`

Clears completed lock command attempts (`success` / `failed`) for the tenant. Pending and retry rows are preserved so the lock retry scheduler can still resend in-flight commands. Returns `{ "deleted": N }` and records a `commands_clear` audit entry.

### `GET /api/tenants/{slug}/lock/audit`

Same pagination contract as commands.

## Motion

- GSAP only on Overview mount (`gsap.from` stagger on KPI cards; short count-up)
- Policies / Exceptions / Activity: no entrance animation

## Out of scope

- Commands TTL / automatic purge
- Dual-pane or anchor-scroll layouts
- Marketing-page visual rebrand
