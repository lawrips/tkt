# TKT Web Security Design

Ticket: c-web-security-design

## Security Goal

`tkt web` is a local browser interface for TKT-owned state. It must not become a
general local control panel, command runner, git credential surface, or file
browser.

The web process can read and write ticket data through structured TKT
operations. The watch/sync process remains responsible for credentialed git
sync behavior.

## Process Boundary

Allowed:

- Run as a separate process from watch/sync.
- Bind to localhost by default.
- Read TKT project configuration summaries.
- Read configured ticket stores.
- Read TKT journal and mutation-log derived data.
- Write structured ticket mutations.

Not allowed:

- Arbitrary shell execution.
- Arbitrary filesystem browsing.
- Arbitrary file read/write outside TKT-owned paths.
- Keychain, credential helper, environment, or secret dumping.
- Git fetch, pull, push, or remote setup.
- Starting or stopping watch/sync.
- Editing agent instruction files directly.

## Network Binding

Default bind address:

- `127.0.0.1`.

Optional future bind modes require a separate design. V1 should not expose the
server on a LAN interface by accident.

## Authentication

Each server launch generates a random token.

Access model:

- The foreground command prints a URL containing the token.
- API requests must include the token.
- Static app assets may be served without auth only if they contain no data and
  no token. Data endpoints require auth.
- Token failures return a stable JSON 401 response for API routes.

The UI should avoid external assets so tokenized URLs are not leaked through
referrer headers.

## Token Storage And Logs

Foreground mode:

- Token is printed to the terminal as part of the local URL.
- Token is not written to logs.

Background mode:

- If a state file stores the port and token, write it under `~/.tkt/state`.
- Use owner-readable permissions where supported, such as `0600`.
- `tkt web status` should have explicit behavior:
  - Either print a usable URL only when it can read protected state, or
  - Print a masked URL and tell the user to restart the web process.
- Tests should lock in the chosen behavior.

Logs:

- Never log access tokens.
- Never log sensitive request bodies.
- Write request logs at a coarse level only, such as method, route, status, and
  duration.

## CSRF And Origin

Because this is a localhost browser app, token auth is the main protection.

API requests should require one of:

- Authorization header with bearer token.
- Query token for the initial app bootstrap only, then header-based API calls.

Mutation routes should reject missing or invalid tokens before reading request
bodies. If Origin is present, accept localhost origins for the active server
only and reject unexpected origins.

## Allowed Reads

The API may expose:

- Configured project names.
- Sanitized project paths and store paths.
- Store mode and existence/writability status.
- Ticket summaries and details.
- Parse/read diagnostics for ticket files.
- Workflow presence and whether default workflow is in use.
- Journal-derived lifecycle and commit context.
- Watch/sync health summaries based on local pid/log/state checks.
- Sanitized git remote presence or host/repo summaries when safe.

The API must not expose:

- Raw credential-bearing remote URLs.
- `.env`, key, token, or credential files.
- Raw full config dumps when fields might contain secrets.
- Arbitrary path contents requested by the browser.

## Allowed Writes

Allowed writes are structured ticket mutations:

- Create ticket if supported by the route.
- Edit title and frontmatter fields.
- Edit description, design, and acceptance criteria.
- Add note.
- Add/remove dependency.
- Link/unlink tickets.
- Parent updates.

Each write should:

- Resolve tickets through configured TKT stores.
- Use shared ticket service logic.
- Append mutation log entries.
- Include revision metadata and reject stale edits.
- Validate IDs, fields, and allowed enum-like values where the current CLI
  already does or where web needs stronger guarantees.

## Sanitization

Remote URLs:

- Strip username, password, token, and query fragments before display.
- Prefer displaying `remote configured` plus host/repo when parseable.
- Fall back to `remote configured` without raw URL when uncertain.

Paths:

- Display paths may use `~` where possible.
- Do not allow path parameters to select arbitrary roots.
- API route parameters identify projects and tickets, not filesystem paths.

Errors:

- Return concise errors.
- Avoid including raw command output from credentialed operations, because the
  web process should not run those operations.

## Denied Operations

The API surface should not include endpoints for:

- `/api/files/*`
- `/api/shell`
- `/api/git/push`
- `/api/git/pull`
- `/api/git/fetch`
- `/api/serve/start`
- `/api/serve/stop`
- Arbitrary path reads or writes.

Tests should include path traversal attempts and unsupported route checks.

## Stale Edit Protection

Ticket detail responses include revision metadata, such as modtime and/or a
content hash.

Mutation requests include the revision they were based on. If the current
revision differs, the API returns a stable conflict response and does not write.

This prevents browser edits from overwriting concurrent CLI, MCP, or agent
changes.

## Test Requirements

Minimum tests:

- Missing token rejects API request.
- Invalid token rejects API request.
- Valid token allows API request.
- Unexpected origin is rejected for mutation requests.
- Path traversal ticket/project identifiers are rejected.
- Unsupported privileged routes are absent or rejected.
- Stale edit mutation is rejected.
- Token is not written to logs.
- Token-bearing state is written with restrictive permissions where supported.
- Remote URL sanitization strips credentials.
