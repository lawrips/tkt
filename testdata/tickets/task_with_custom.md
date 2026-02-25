---
id: pa-5c46
status: open
deps: []
links: [pa-8b12]
created: 2026-02-24T12:00:00Z
type: task
priority: 2
assignee: lawrence
parent: pa-ff45
tags: [backend, api]
external_ref: gh-101
custom_field: keep-me
custom_map:
  nested: true
---
# Add SSE connection handling

Implement reconnection-safe SSE streaming.

## Design

Use heartbeat pings and jittered retry.

## Acceptance Criteria

1) Survives transient network blips
2) Includes integration test
