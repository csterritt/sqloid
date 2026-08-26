## Issue 55: Universal quit confirmation and exact restoration

**Type**: AFK
**Blocked by**: Issue 21, Issue 28, Issue 34, Issue 41, Issue 42, Issue 43, Issue 45, Issue 46, Issue 53, Issue 54

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement q/Ctrl+C quit behavior across every nonterminal context, temporarily suspending one overlay and restoring the latest exact state on cancellation. Accepted quit must invoke the applicable schema-validation, SELECT, write, or preparation cleanup; terminal states exit immediately with status 1.

### How to verify

- **Manual**: Open/cancel/confirm quit from every context, especially in-flight schema validation, estimate, overwrite, active SELECT, write phases, too-small screen, and terminal states.
- **Automated**: Full matrix tests assert identical q/Ctrl+C confirmation, Ctrl+C confirmation behavior, Esc/n restoration, settled-behind-quit updates, no leakage, schema-validation no-history cancellation/settlement, lifecycle cleanup, and terminal immediate exit.

### Acceptance criteria

- [ ] Given any enabled nonterminal context, then q and Ctrl+C open the same confirmation while preserving its exact suspended context.
- [ ] Given cancellation, then focus, overlay/search/viewport, estimate or overwrite state, immutable copy, and selection restore to their latest exact values without key leakage.
- [ ] Given accepted quit during pre-execution schema validation, then cancellation is requested, settlement completes before exit, any late success remains cancelled, and neither query nor result history is appended.
- [ ] Given confirmation, then required request/transaction cleanup finishes before exit; given a terminal state, q/Ctrl+C exits immediately with status 1.

### User stories addressed

- User story 10: Quit consistently without abandoning cleanup
- User story 11: Restore exact suspended context when quit is cancelled
- User story 86: Restore latest estimate or overwrite workflow without key leakage

---
