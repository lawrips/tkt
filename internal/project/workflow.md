# tkt Workflow

## Ticket lifecycle

`open` -> `in_progress` -> `needs_testing` -> `closed`

## Commit format

Reference tickets in commit messages using bracket refs:

`Tickets: [my-ticket-id]`

To auto-close a ticket on commit:

`Closes: [my-ticket-id]`

If the background service is running (`tkt serve start`), it watches git log,
links commits to tickets, and closes tickets from `Closes: [...]`.

## Defaults

- Statuses: `open`, `in_progress`, `needs_testing`, `closed`
- Types: `bug`, `feature`, `task`, `epic`, `chore`
- Priorities: `0` (critical) to `4` (backlog)

## Conventions

- Set status to `in_progress` when starting work.
- Set status to `needs_testing` when implementation is done.
- Include acceptance criteria for implementation tickets.
- Keep the dependency graph acyclic.
- Use parent tickets for epics and phases.
