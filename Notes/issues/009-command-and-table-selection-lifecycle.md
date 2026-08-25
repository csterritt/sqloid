## Issue 9: Command and table selection lifecycle

**Type**: AFK
**Blocked by**: Issue 7, Issue 8

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver the initial builder path from Command through Table. Apply the command-change clearing and eligibility rules in **Builder and Display Interaction**, including the special view-to-write transition.

### How to verify

- **Manual**: Start the TUI, select S/U/D/I, choose tables and views, revisit Command, and switch among read/write commands.
- **Automated**: Scripted model tests assert initial focus, one-key command selection, focus advancement, downstream clearing, and table retain/clear behavior.

### Acceptance criteria

- [ ] Given initial startup, then Command is focused and one S/U/D/I key selects the command and advances to Table.
- [ ] Given a command replacement, then downstream command-specific state is cleared and only an eligible table is retained.
- [ ] Given a selected view and a switch to a write command, then the table selection is cleared and Table is focused, while the eligible table list remains populated with eligible ordinary and virtual tables.

### User stories addressed

- User story 17: Select a command with one keypress
- User story 18: Replace a command and clear dependent fields
- User story 19: Clear views while retaining eligible write tables

---
