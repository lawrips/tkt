# tk v2 Contracts (tkv2-contracts-parity)

This document converts `DESIGN-v2.md` into implementation contracts for parity, JSON output, and watcher mutation boundaries.

## 1) v1 Command Parity Matrix

Legend:
- `P0` = required for drop-in replacement
- `P1` = required by design, not present in v1

### Viewing

| Command | Priority | v1 Syntax | Required Flags | Notes |
|---|---|---|---|---|
| show | P0 | `tk show <id>` | none | Partial ID matching preserved |
| ls / list | P0 | `tk ls [filters]` | `--status`, `-t/--type`, `-P/--priority`, `-a/--assignee`, `-T/--tag`, `--parent`, `--group-by` | Default open-only behavior preserved |
| ready | P0 | `tk ready [filters]` | `-a/--assignee`, `-T/--tag`, `--open` | Parent hierarchy checks preserved |
| blocked | P0 | `tk blocked [filters]` | `-a/--assignee`, `-T/--tag` | Open-ticket dependency checks |
| closed | P0 | `tk closed [filters]` | `--limit`, `-a/--assignee`, `-T/--tag` | Default limit = 20 |

### Creating and Editing

| Command | Priority | v1 Syntax | Required Flags | Notes |
|---|---|---|---|---|
| create | P0 | `tk create [title] [options]` | `-d/--description`, `--design`, `--acceptance`, `-t/--type`, `-p/--priority`, `-a/--assignee`, `--id`, `--parent`, `--tags`, `--external-ref` | Interactive create preserved when title omitted |
| edit | P0 | `tk edit <id> [options]` | create flags plus `-s/--status`, `--title` | Field-level edits; no unrequested rewrites |
| add-note | P0 | `tk add-note <id> [text]` | none | Uses stdin when text omitted |
| delete | P0 | `tk delete <id> [id...]` | none | Multi-delete behavior preserved |

### Dependencies and Links

| Command | Priority | v1 Syntax | Required Flags | Notes |
|---|---|---|---|---|
| dep | P0 | `tk dep <id> <dep-id>` | none | Add dependency edge |
| undep | P0 | `tk undep <id> <dep-id>` | none | Remove dependency edge |
| dep tree | P0 | `tk dep tree [--full] <id>` | `--full` | Tree + truncation semantics preserved |
| dep cycle | P0 | `tk dep cycle` | none | Open-ticket cycle detection |
| link | P0 | `tk link <id> <id> [id...]` | none | Symmetric links |
| unlink | P0 | `tk unlink <id> <target-id>` | none | Remove one symmetric edge |

### Query and Analytics

| Command | Priority | v1 Syntax | Required Flags | Notes |
|---|---|---|---|---|
| query | P0 | `tk query [jq-filter]` | none | Filter expression fed into `select()` |
| stats | P0 | `tk stats` | none | Project health rollup |
| timeline | P0 | `tk timeline [--weeks=N]` | `--weeks` | Weekly closure bins |
| workflow | P0 | `tk workflow` | none | Convention/help output |
| migrate-beads | P0 | `tk migrate-beads` | none | Maintained for compatibility |

### v2-only Commands (Design Required)

| Command | Priority | Syntax |
|---|---|---|
| init | P1 | `tk init` |
| migrate | P1 | `tk migrate --central|--local` |
| watch | P1 | `tk watch` |
| recompute | P1 | `tk recompute` |
| epic-view | P1 | `tk epic-view <id>` |
| progress | P1 | `tk progress [--today\|--week]` |
| dashboard | P1 | `tk dashboard` |
| config | P1 | `tk config` |

## 2) Flag Inventory Contract

All v1 flags are required in v2:

| Flag | Commands |
|---|---|
| `--status` | `ls` |
| `-t`, `--type` | `ls`, `create`, `edit` |
| `-P`, `--priority` | `ls` |
| `-a`, `--assignee` | `ls`, `ready`, `blocked`, `closed`, `create`, `edit` |
| `-T`, `--tag` | `ls`, `ready`, `blocked`, `closed` |
| `--parent` | `ls`, `create`, `edit` |
| `--group-by` | `ls` |
| `--open` | `ready` |
| `--limit` | `closed` |
| `-d`, `--description` | `create`, `edit` |
| `--design` | `create`, `edit` |
| `--acceptance` | `create`, `edit` |
| `-p`, `--priority` | `create`, `edit` |
| `-s`, `--status` | `edit` |
| `--title` | `edit` |
| `--id` | `create` |
| `--tags` | `create`, `edit` |
| `--external-ref` | `create`, `edit` |
| `--full` | `dep tree` |
| `--weeks` | `timeline` |

## 3) JSON Contract (`--json`)

All output-producing commands must support `--json`. CLI default remains human-readable ASCII.

### Common envelope

```json
{
  "meta": {
    "command": "ls",
    "project": "my-project",
    "generated_at": "2026-02-25T00:00:00Z",
    "version": "v2"
  },
  "data": {}
}
```

### Shared types

```json
{
  "Ticket": {
    "id": "string",
    "title": "string",
    "status": "open|in_progress|needs_testing|closed",
    "type": "bug|feature|task|epic|chore",
    "priority": "integer(0-4)",
    "assignee": "string|null",
    "parent": "string|null",
    "deps": ["string"],
    "links": ["string"],
    "tags": ["string"],
    "created": "RFC3339 timestamp",
    "external_ref": "string|null",
    "description": "string",
    "design": "string",
    "acceptance_criteria": "string",
    "notes": [{"at": "RFC3339 timestamp", "text": "string"}]
  },
  "TicketSummary": {
    "id": "string",
    "title": "string",
    "status": "open|in_progress|needs_testing|closed",
    "type": "bug|feature|task|epic|chore",
    "priority": "integer(0-4)"
  },
  "CommitLink": {
    "sha": "string",
    "ticket": "string",
    "repo": "string",
    "ts": "RFC3339 timestamp",
    "msg": "string",
    "author": "string",
    "action": "ref|close"
  }
}
```

### Command -> data schema mapping

| Command | `data` schema |
|---|---|
| `show` | `Ticket` |
| `ls`, `ready`, `blocked`, `closed` | `{ "items": ["TicketSummary"], "total": "int" }` |
| `create`, `edit` | `Ticket` |
| `delete` | `{ "deleted": ["string"], "not_found": ["string"] }` |
| `add-note` | `{ "ticket_id": "string", "note": { "at": "RFC3339", "text": "string" } }` |
| `dep`, `undep`, `link`, `unlink` | `{ "ticket_id": "string", "updated_field": "deps|links", "values": ["string"] }` |
| `dep tree` | `{ "root": "string", "nodes": [{ "id": "string", "status": "string", "children": ["string"] }] }` |
| `dep cycle` | `{ "cycles": [["string"]] }` |
| `query` | `{ "items": ["Ticket"], "filter": "string|null" }` |
| `stats` | `{ "counts": { "open": "int", "in_progress": "int", "needs_testing": "int", "closed": "int" }, "by_type": {}, "by_priority": {} }` |
| `timeline` | `{ "weeks": [{ "week_start": "YYYY-MM-DD", "closed_count": "int" }] }` |
| `workflow` | `{ "content": "string" }` |
| `epic-view` | `{ "epic": "TicketSummary", "children": ["TicketSummary"], "deps": [{ "from": "string", "to": "string" }], "commits": ["CommitLink"] }` |
| `progress` | `{ "window": "today|week", "closed": ["TicketSummary"], "commit_links": ["CommitLink"], "status_changes": [{ "ticket_id": "string", "from": "string", "to": "string", "at": "RFC3339" }] }` |
| `dashboard` | `{ "summary": {}, "in_progress": ["TicketSummary"], "blocked": ["TicketSummary"], "ready": ["TicketSummary"], "recent_commits": ["CommitLink"] }` |
| `config` | `{ "projects": {}, "resolved_project": "string|null" }` |

## 4) Watcher Mutation Boundary Contract

Agent-owned mutations:
- Create/edit/delete ticket files
- Manage title/description/design/acceptance/notes/deps/links/tags/assignee/priority/parent
- Explicit status edits outside automation

Watcher-owned mutations:
- Append commit link rows to `~/.tk/state/<project>/commits.jsonl`
- Auto-close status update on commit keywords (`closes`/`fixes`) when enabled
- Central store git commit/push automation

Watcher is prohibited from:
- Rewriting ticket body sections
- Editing deps/links/parent/metadata not required for auto-close
- Mutating notes for commit linkage (commit linkage stays in side index)

## 5) Intentional Divergences from v1

1. Output rendering can change while preserving semantics
- Human-readable formatting (table/tree colors/layout) may improve in v2.

2. `--json` becomes first-class
- v1 relied on `query` JSONL for scripting; v2 standardizes `--json` on all output commands.

3. Side index for commits
- v1 workflows added commit references into ticket notes manually; v2 stores relationships in journal files.

4. Automation boundaries are explicit
- Status may be auto-closed by watcher. All other ticket content remains manual/agent-driven.

5. New operational commands
- `init`, `migrate`, `watch`, `recompute`, `epic-view`, `progress`, `dashboard`, `config` are new and not expected to match v1 behavior.
