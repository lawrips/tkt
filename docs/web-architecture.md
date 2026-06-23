# TKT Web Architecture

Ticket: c-web-server-architecture

## Decision Summary

`tkt web` will be implemented in the existing Go binary with no required user
runtime beyond the binary itself.

Architecture decisions:

- Add a shared service layer in `internal/app`.
- Add HTTP server and embedded assets in `internal/web`.
- Add CLI lifecycle commands in `internal/cli`.
- Use plain embedded HTML, CSS, and JavaScript for v1.
- Do not add a JavaScript package manager or frontend build chain in v1.
- Keep watch/sync in its existing process model.

## Package Boundaries

### `internal/app`

Shared application service layer for TKT operations.

Responsibilities:

- Resolve configured projects and ticket directories without CLI context.
- List projects and expose current-project resolution state.
- List tickets with records and parse/read diagnostics.
- Load ticket detail with related records and journal context.
- Perform structured ticket mutations.
- Append mutation log entries.
- Generate and validate revision metadata for optimistic concurrency.

Dependencies:

- May depend on `internal/engine`, `internal/project`, and `internal/ticket`.
- Must not depend on `internal/cli`, `internal/mcp`, or `internal/web`.

Migration strategy:

- Web uses `internal/app` from the start.
- CLI and MCP can be migrated incrementally.
- New web behavior must not duplicate mutation code that should live in
  `internal/app`.

### `internal/web`

Local HTTP server and embedded browser app.

Responsibilities:

- Generate or accept per-launch token.
- Serve embedded static assets.
- Enforce API auth and origin checks.
- Expose structured JSON API endpoints.
- Call `internal/app` services for all TKT reads and writes.
- Return stable JSON errors.

Dependencies:

- May depend on `internal/app`, `internal/project`, and low-level standard
  library packages.
- Must not depend on `internal/cli` to avoid circular command concerns.
- Must not call git sync commands or watch/sync process controls.

### `internal/cli`

Command registration and process lifecycle.

Responsibilities:

- Register `web` command and subcommands.
- Exempt `web` from the current initialization gate.
- Start foreground web process.
- Start/stop/status/logs for the background web process.
- Store web pid/log/state separately from serve/watch.

Dependencies:

- May depend on `internal/web`.
- Must not expose watch/sync controls through web handlers.

## Command Model

Commands:

- `tkt web`
  - Foreground server.
  - Equivalent to `tkt web run`.
  - Prints authenticated localhost URL.

- `tkt web run`
  - Explicit foreground subcommand.
  - Used by `tkt web start`.

- `tkt web start`
  - Starts a background web process.
  - Writes separate pid, log, and state files.

- `tkt web stop`
  - Stops only the web process.

- `tkt web status`
  - Reports only the web process.
  - Does not start or stop watch/sync.

- `tkt web logs`
  - Shows only web logs.

The existing `serve` command remains the watch/sync process. It does not expose
HTTP by default.

## Initialization Behavior

Current CLI behavior gates most commands through `requiresInit`. `web` must be
added to the exempt list so it can render setup guidance before `tkt init`.

When no project resolves:

- `internal/app` returns a project resolution object with status `uninitialized`
  or `unresolved`.
- The web API still serves session, setup, and configured-project summaries.
- Ticket routes return structured setup errors instead of crashing or falling
  through to arbitrary paths.

## API Contract

All data endpoints are under `/api`.

### `GET /api/session`

Returns:

- server version
- authenticated status
- current working directory display string
- resolved project, if any
- setup status

### `GET /api/projects`

Returns configured projects:

- project name
- configured path display
- store mode
- ticket directory display
- ticket directory status
- current/resolved flag
- parse/read warning count when known

### `GET /api/projects/{project}/tickets`

Query params:

- status
- type
- priority
- assignee
- tag
- parent
- search
- sort
- ready
- blocked

Returns:

- items
- total
- diagnostics

Each item includes:

- id
- title
- status
- type
- priority
- assignee
- tags
- parent
- modified timestamp
- revision

### `GET /api/projects/{project}/tickets/{id}`

Returns:

- ticket frontmatter fields
- title
- description
- design
- acceptance criteria
- other sections
- notes section
- deps with resolved summaries
- links with resolved summaries
- children summaries
- recent commits
- revision
- diagnostics

### `PATCH /api/projects/{project}/tickets/{id}`

Request:

- revision
- source
- fields object
- body sections object

Allowed fields:

- title
- status
- type
- priority
- assignee
- parent
- tags
- description
- design
- acceptance_criteria

Response:

- updated ticket detail

Conflict:

- status `409`
- code `stale_revision`
- message with refresh guidance

### `POST /api/projects/{project}/tickets/{id}/notes`

Request:

- revision
- source
- text

### Link And Dependency Routes

Routes:

- `POST /api/projects/{project}/tickets/{id}/deps`
- `DELETE /api/projects/{project}/tickets/{id}/deps/{depID}`
- `POST /api/projects/{project}/tickets/{id}/links`
- `DELETE /api/projects/{project}/tickets/{id}/links/{targetID}`

All mutation routes require a valid token and revision where the source ticket
is modified.

## Error Model

JSON error shape:

```json
{
  "error": {
    "code": "stale_revision",
    "message": "Ticket changed on disk. Refresh before saving.",
    "details": {}
  }
}
```

Common codes:

- `unauthorized`
- `forbidden_origin`
- `not_found`
- `validation_error`
- `project_unresolved`
- `parse_error`
- `stale_revision`
- `internal_error`

## Asset Strategy

Use `go:embed` in `internal/web/assets.go`.

Asset files:

- `internal/web/assets/index.html`
- `internal/web/assets/styles.css`
- `internal/web/assets/app.js`

No build step is required. No external fonts, scripts, CSS, or CDN assets are
loaded in v1.

## Revision Strategy

Each ticket response includes a revision object:

- modtime in RFC3339Nano, when available
- content hash for the ticket file

Mutations compare the submitted revision against the current file revision. If
either value differs, the service rejects the write.

This protects web edits from overwriting concurrent CLI, MCP, or agent changes.

## Parse Diagnostics

`ticket.List` currently skips invalid files. The service layer needs a
parse-aware list path:

- read `.md` entries
- attempt `ticket.LoadRecord`
- append successful records
- append diagnostics for errors

Diagnostics should include:

- safe file name
- error code when recognizable
- message

The web UI should show valid tickets even when diagnostics exist.

## Health Data

Initial health data is local and non-credentialed:

- project resolution status
- config present or missing
- workflow default or custom
- ticket directory exists/writable
- central store path exists and is a git repo when applicable
- watch/sync pid appears running
- sync blocked marker exists when applicable
- remote configured yes/no without raw credential-bearing URL

No health route performs git fetch, pull, push, or remote setup.

## Tests

Service tests:

- project resolution in initialized and uninitialized directories
- local and central ticket directory resolution
- parse diagnostics
- ticket detail related data
- mutation logging
- stale revision rejection

HTTP tests:

- token required
- invalid token rejected
- valid token accepted
- unexpected origin rejected for mutation
- stable JSON errors
- no path traversal

CLI tests:

- `web` registered
- `web` exempt from initialization gate
- foreground argument parsing
- background pid/log/state paths are separate from serve/watch
- stale pid/state behavior

Asset tests:

- index route serves embedded app
- static assets are embedded
- no external assets in v1
