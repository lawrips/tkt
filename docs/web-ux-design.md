# TKT Web UX Design

Ticket: c-web-ux-design

## Product Role

`tkt web` is a local workbench for TKT state. It is not a project-management
replacement, a command runner, or a hosted collaboration service. The web UI
exists to make TKT's durable ticket memory easier to inspect and curate.

The CLI remains the scripting surface. MCP remains the agent surface. The web UI
is the human visual workspace.

## Primary User Moments

1. Returning to work
   - See the current project, active tickets, blocked tickets, ready tickets,
     and recent activity.
   - Open a ticket and recover context quickly from description, design,
     acceptance criteria, notes, deps, links, and recent commits.

2. Curating ticket memory
   - Update status, title, assignment, priority, body sections, notes, links,
     and dependencies through structured controls.
   - Preserve the ticket file as the source of truth.

3. Checking setup health
   - See whether the current project resolves, where tickets are stored, and
     whether watch/sync appears to be running.
   - Get copy-paste guidance for setup or sync commands.

4. Opening before setup is complete
   - `tkt web` should still load when the current directory is not initialized.
   - The first screen should explain the missing setup and show the next command
     to run, instead of failing before the browser opens.

## Information Architecture

The first screen is a compact workbench:

- Project rail
  - Configured projects.
  - Current project, if resolved.
  - Store mode: local or central.
  - Small health indicator.

- Ticket list
  - Search input.
  - Filter controls for status, type, priority, assignee, tag, blocked, ready,
    and parent.
  - Stable compact rows with status, type, priority, title, and relevant badges.

- Ticket detail
  - Header: title, id, status, type, priority, assignee, parent.
  - Body: description, design, acceptance criteria, extra sections.
  - Context: notes, deps, links, children, lifecycle, recent commits.
  - Actions: save structured edits, add note, link, unlink, dep, undep.

- Health panel
  - Project resolution.
  - Ticket directory and parse health.
  - Central store state when applicable.
  - Watch/sync process status and copy-paste commands.

## Uninitialized And Setup States

When no project resolves:

- Show "No project configured for this directory."
- Show `tkt init` as the primary next step.
- If config exists, list configured projects and their paths.
- If config is missing, explain that TKT can still be used after initialization.
- Do not require ticket APIs to succeed before rendering the shell.

When a store is missing:

- Show the configured project and missing store path in sanitized form.
- Offer guidance such as `tkt init`, `tkt migrate`, or manual inspection.
- Keep other projects accessible.

When doctor is available later:

- Reuse the same health categories and messages.
- The web UI renders checks; it does not run privileged fixes.

## Ticket List Behavior

Default view:

- Current project.
- Open tickets first.
- In-progress and needs-testing easy to reach.
- Search matches id and title initially; body search can be a future extension.

Filters:

- Status: all, open, in_progress, needs_testing, closed.
- Type: bug, feature, task, epic, chore.
- Priority: 0 through 4.
- Assignee.
- Tag.
- Parent.
- Ready and blocked.
- Sort by id, created, modified, priority, title.

Rows should not resize unpredictably on hover or status changes.

## Ticket Detail Behavior

The detail view should make the ticket read like a durable work record:

- Top metadata is dense and editable.
- Description/design/acceptance criteria are first-class sections.
- Notes are clearly separated from durable body sections.
- Dependencies and links show status and title when target tickets exist.
- Missing deps or stale links are visible instead of hidden.
- Recent commits are contextual, not the main content.

## Editing Model

All edits are structured. There is no raw file editor in v1.

Allowed edits:

- Status, type, priority, assignee, parent.
- Title.
- Description, design, acceptance criteria.
- Add note with source attribution.
- Add/remove dependency edge.
- Link/unlink tickets.

Every edit request includes revision metadata from the loaded ticket. If the
ticket changed since it was loaded, the UI shows a conflict state and asks the
user to refresh before saving.

## Parse And Read Diagnostics

Invalid ticket files should not make the whole project unusable.

The list and detail surfaces should show:

- File or ticket identifier when safe.
- Error category, such as missing frontmatter or malformed frontmatter.
- A short remediation hint.

The UI should continue showing all valid tickets.

## Empty And Error States

Required states:

- No configured projects.
- No project resolved for current directory.
- Ticket directory missing.
- No tickets.
- Parse/read warnings.
- Unauthorized or expired token.
- Stale edit conflict.
- Validation error.
- Denied operation.
- API/server error.

Each state should include a next step. For command guidance, show copy-paste
commands instead of running them from the web UI.

## Mobile And Desktop

Desktop:

- Use a three-region workbench where there is enough space.
- Keep filters and project navigation visible.
- Detail can sit beside the list.

Mobile:

- Use a stacked navigation model.
- Ticket list and detail should be separate views.
- Filters should collapse behind a button or drawer.
- Editing controls must avoid horizontal overflow.

## V1 Non-Goals

- Arbitrary project file browsing.
- Arbitrary file editing.
- Running shell commands.
- Starting or stopping watch/sync from the browser.
- Git fetch, pull, push, or remote setup.
- Remote multi-user hosting.
- External CDN or web assets.
- Raw secret-bearing config display.

## Future Extensions

- Keyboard shortcuts.
- Full-text body search.
- Saved filters.
- Rich markdown preview/editor.
- More doctor integration.
- Optional remote binding with a separate security design.
