# tkt

tkt is a lightweight, git-backed tool for managing features and bugs with AI coding agents.

I was inspired by the elegant simplicity of the excellent [wedow/ticket](https://github.com/wedow/ticket) tool. Over time I found myself needing a different feature set, a personalized workflow and a level of automation. So I built tkt from the ground up to give me that.

tkt is somewhat opinionated and tailored to how I work and the main tool I use for feature / bug management. It's written in Go and I used it in my workflow with AI agents to develop it. As of Feb 2026, it has backwards compatibility with ticket, supporting the original command line args and frontmatter. Easy to switch back.

These are some additions that have been helpful for me:

- **TUI** — optional terminal UI for viewing, basic editing (status, priority etc) and monitoring (Bubble Tea)
- **Central ticket store** — tickets live in `~/.tickets`, keeping working directories and commit history clean
- **Service mode** — pushes ticket changes to the central repo, auto-closes tickets from commits and maintains an append-only journal, simplifying manual bookkeeping
- **Doctor** — safe local setup checks for config, project registration, central store, workflow, serve/watch, and agent instructions
- **Web control plane** — optional localhost-only browser workbench for browsing, editing, and checking ticket health
- **Composite views** — epic-view, context, dashboard, progress — all computed over the same underlying data
- **MCP server** — agents interact through typed tool schemas instead of CLI string parsing, to improve command use and source attribution

## Disclaimer
tkt is a personal project built for my own workflow. MIT license - it's shared as-is in case others find it useful. If you choose to use it, back up any existing tickets (e.g. .tickets) or other data.

## Setup

```bash
brew install go  # if needed
go install github.com/lawrips/tkt/cmd/tkt@latest
```

Then open your AI coding agent (Claude Code, Codex, etc.) and tell it:

```
Run `tkt agent-setup` and follow the instructions to set up tkt on this system
```

It will register MCP, and configure everything. You can also follow [setup.md](internal/cli/setup.md) manually.

After that, in each project you want to use tkt in run:

```
tkt init
```


## Quick Start

```bash
tkt help              # see all commands
tkt doctor            # audit setup health
tkt tui               # optional terminal UI
tkt web               # optional local browser workbench
tkt workflow          # user-editable workflow guide from ~/.tkt/workflow.md
```

There are CLI commands for creating and editing tickets, but I just ask Claude / Codex to do it. The TUI and web workbench are optional human interfaces; the CLI, MCP, central store, and service mode do not require either UI to be running.

## Features

Some of the main categories of features I built for this tool:

### TUI

`tkt tui` opens an optional interactive terminal interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). Three views:

- **Board** — kanban columns grouped by status (open, in_progress, needs_testing, closed)
- **Detail** — full ticket view with description, notes, deps, linked commits
- **Epic** — hierarchical tree of parent and child tickets

Keyboard shortcuts for common mutations: `s` status, `p` priority, `a` assignee, `t` type, `d` deps, `n` add note, `c` create, `x` delete, `e` open in $EDITOR. `/` to filter across any view.

### Central Ticket Store

By default, `tkt init --store central` puts tickets in `~/.tickets/<project>` — a separate git repo from your project. This means no `.tickets` directory in your working tree and no ticket file churn in your project's commit history. The service daemon handles git add/commit/push for the ticket store.

You can also use `--store local` to keep tickets in `.tickets/` inside your project directory, which is how wedow/ticket works.

**Note:** tkt is currently designed for single-machine use. The central store at `~/.tickets` is local and can be auto-pushed to git, but has only been designed currently for a single machine. There's no built-in multi-device sync. This is a future consideration.

#### Custom Store Location

There is an option to set the `TKT_ROOT` environment variable to override the default `~/.tickets` path for the central store. May help for sandboxed environments where `$HOME` may not resolve to the expected location.

```bash
export TKT_ROOT=/workspace/tickets  # must be an absolute path
```

`TKT_ROOT` only affects the central ticket store — config and state remain at `~/.tkt`. If using the background daemon (`tkt serve start`), ensure `TKT_ROOT` is set in the environment when the daemon is started.

### Service Mode

`tkt serve start` runs a background daemon that polls your project's git log on an interval (default 30s). It looks for ticket IDs in commit messages using bracket refs (e.g. `[my-ticket-id]`).

When it finds a match, it appends an entry to an append-only journal linking the commit to the ticket. If the commit message contains `Closes: [ticket-id]`, the ticket is automatically set to closed.

The journal lives at `~/.tkt/state/<project>/journal.log`. It's the source for commit-linked views like `progress` and `context`. Journal timestamps also enable time-spent tracking via `tkt lifecycle <id>`, which shows status transitions and durations.

Manage the daemon with `tkt serve start`, `tkt serve stop`, `tkt serve status`, and `tkt serve logs`.

### Doctor

`tkt doctor` audits local setup without changing files, starting services, or running credentialed git operations. It checks global config, current project resolution, ticket store health, workflow guidance, central-store git state, serve/watch status, and known agent instruction files. Use `tkt doctor --json` for agent-readable output.

Doctor gives copy-paste remediation commands, but it does not apply fixes itself.

### Web Control Plane

`tkt web` is an optional browser workbench for humans who want to inspect and edit tickets outside the terminal. You do not need it for the CLI, MCP tools, central ticket store, or `tkt serve` sync/watch workflow.

The web interface runs from the same Go binary; there is no separate app server to install. By default it binds only to `127.0.0.1:7420`, serves embedded HTML/CSS/JS, generates a per-launch token, and prints an authenticated local URL.

Use it to browse tickets in list or board views, inspect context, edit structured fields, add notes, manage deps/links, review project analytics, and view the same setup health surfaced by `tkt doctor`.

The web process is separate from `tkt serve`. Starting the web workbench does not start sync/watch, and stopping it does not stop sync/watch. It does not run sync, push, pull, fetch, arbitrary shell commands, arbitrary filesystem browsing, or start/stop the serve/watch process. It can run before `tkt init`; in an uninitialized repo it shows setup guidance and any configured projects it can safely read.

Commands:

```bash
tkt web              # foreground localhost server, prints an authenticated URL
tkt web start        # background web process
tkt web status       # web process status only
tkt web stop         # stops only the web process
tkt web logs         # web process logs only
```

Use `--addr=127.0.0.1:0` when you want an ephemeral port instead of the stable default. For remote access, keep the server bound to localhost and use SSH port forwarding, for example `ssh -L 7420:127.0.0.1:7420 <host>`.

### Composite Views

These are read-only views computed over ticket files and the commit journal:

- **`tkt epic-view <id>`** — parent ticket with all children, their statuses, and linked commits
- **`tkt context <id>`** — full working context: parent, dependency status, linked tickets, children, recent commits
- **`tkt dashboard`** — CLI project summary: in-progress, blocked, ready, and recent commits
- **`tkt progress`** — closed tickets and commit links within a time window (today or this week)
- **`tkt stats`** — counts by status, type, and priority
- **`tkt timeline`** — weekly closed-ticket counts, bucketed by close time when available

### MCP Server

`tkt mcp` starts a stdio JSON-RPC server implementing the [Model Context Protocol](https://modelcontextprotocol.io). There are 22 tools (14 read, 8 write) with typed schemas — agents discover available operations and their parameters automatically.

Read tools include `list`, `show`, `context`, `epic_view`, `dashboard`, `progress`, `stats`, `ready`, `blocked`, `closed`, `dep_tree`, `lifecycle`, `timeline`, and `workflow`. Write tools include `create`, `edit`, `delete`, `add_note`, `dep`, `undep`, `link`, and `unlink`.

Every tool accepts an optional `project` field. Omit it to use the project resolved from the MCP server's working directory, or pass an exact registered project name to work with another configured project from the same server process. Project selection is revalidated on every call; unknown names and path-shaped values are rejected. Successful JSON results include `resolved_project` and never include ticket-store paths.

Write operations require a `source` field identifying the caller (e.g. "claude", "codex", "human"). This gives you an audit trail — you can see which agent or human made each change.


## License

MIT — see [LICENSE](LICENSE).
