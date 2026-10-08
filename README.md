# souna-harness

Agent harness for sounacode: sessions, agent loop, tool orchestration,
providers, and subagents. Zero UI dependencies — the TUI consumes it
through a typed event channel.

## Architecture

- `internal/harness` — the agent loop and event seam. Imports nothing UI.
- `internal/provider` — config-driven provider registry, keys via env.
- `internal/tools` — tool implementations (bash, read, edit, grep, glob).
- `internal/session` — session store and persistence.
- `cmd/pillead` — headless driver, proves the harness has no UI dependency.

Direction of dependency is one way: the TUI imports the harness, the
harness never imports the TUI.
