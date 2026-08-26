---
name: tui-behavior
description: Rules for implementing Sqloid's Bubble Tea event handling and asynchronous terminal behavior.
---

## Bubble Tea behavior

- Treat the model as the authoritative UI state. `Update` applies messages and returns commands; `View` renders state and performs no I/O or state mutation.
- Perform database, filesystem, timer, and other blocking work in `tea.Cmd` functions, never directly in `Update` or `View`.
- Return typed messages containing the operation identity and complete result needed by `Update`. Validate execution, request, and viewport-generation identities before a response mutates state.
- Keep key handling contextual and preserve the precedence, cancellation, focus restoration, and undersized-terminal contracts in the PRD. Do not add global key handling that bypasses overlays or focused inputs.
- Make in-flight state explicit and gate duplicate operations. Cancellation must update visible state immediately, signal the owned operation, and safely discard stale or late responses.
- Keep side effects out of render helpers. Given the same model and terminal dimensions, rendering must be deterministic.
- Preserve semantic state across resize. Recompute layout from current dimensions and clamp viewports without losing builder, focus, selection, history, or request identity.
- Never use sleeps to coordinate UI tests. Drive `Update` with messages and assert the resulting model, command, and rendered view.

## References

- [Bubble Tea basics](https://github.com/charmbracelet/bubbletea/tree/main/tutorials/basics)
- [Bubble Tea commands](https://github.com/charmbracelet/bubbletea/blob/main/tutorials/commands/README.md)
