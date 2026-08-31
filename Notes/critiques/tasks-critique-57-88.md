# Tasks Critique — Sqloid Final-Audit Remediation Set (Tasks 57–88)

Reviewer: skeptical senior engineer, **tasks-only** review.
Inputs: `Notes/PRD-sqloid.md` (authoritative), `Notes/issues/057-*.md … 088-*.md`,
`Notes/tasks/057-*.md … 088-*.md`, and the prior `Notes/critiques/final-audit.md`,
`Notes/critiques/final-audit-issues-critique.md`.
Scope: the 32 **task files** for issues 57–88 — their decomposition, RED/GREEN
discipline, dependency/sequencing correctness, testability, traceability, and
best-practice quality. This is not a re-review of the PRD, the issues, or the code;
where I inspected the code it was only to verify that the tasks' prescribed anchors
(files/functions/fields) actually exist.

---

## Summary

This is a **high-quality, implementation-ready task set** that faithfully decomposes
each remediation issue into a RED → GREEN (→ RED/GREEN…) → DOCUMENT → CODE WALKTHROUGH
chain. The tasks are unusually well-grounded: every anchor I spot-checked is real
(`RowidApplicable = iota` is genuinely an untyped constant; the `tea.KeyLeft/KeyRight`
arms in `applyPickerFilenameKey` are genuinely unreachable because `handlePickerKey`
toggles format on `"left"/"right"` first; `Finalization.CountCacheInconsistent`,
`TraversalFacts.HasLimit/Limit`, `result.RealToken` vs `querybuilder.realToken`,
`applySelectSettled` in `first_select.go` all exist as described). The task set also
**incorporates the earlier issues-critique's sequencing recommendations**: the 65/66
report-vs-renderer split is clean, 72 is sequenced before 73 on the shared
`applySelectSettled` surface, the classifier cluster is chained 77→78→79→80, 63 precedes
64, and 074 now covers F0–F4 four-byte subparts.

The problems are therefore **not about coverage**. They cluster around three themes:

1. **Cross-issue ordering is expressed only in prose, never in the structured
   `Depends on` field** (which says `none` on almost every RED "Task 1" even when the
   prose says "Begin only after Issue #X"). This is the most consequential defect for
   any AFK/parallel scheduler.
2. **The single highest-risk deliverable — the headless-TUI harness in Tasks 57 and
   88 — is left undecided and internally contradictory** ("build the shipped binary"
   *and* "Bubble Tea test harness" are different mechanisms), and no harness dependency
   exists in the repo yet.
3. A few **shared-surface collisions and proportionality/methodology issues** (Tasks 75
   and 76 uncoordinated; Task 57 oversized; source/AST "tests" that fight the PRD's
   testing philosophy).

Findings are prioritized: **P0** = fix before implementation starts; **P1** = fix soon;
**P2** = nice-to-have.

---

## Strengths worth preserving

- **Faithful incorporation of the prior issues-critique.** The decomposition/sequencing
  defects flagged against the issues were resolved in the tasks: 65 is report-only and 66
  owns all SELECT-renderer gating; 72's `applySelectSettled`/`applyPageSettled` edit is
  explicitly "coordinate this shared surface before Issue #73"; 77→78→79→80 is a stated
  linear classifier chain; 63 → 64 preserves the short-write handling; 074 covers both
  E0–EF and F0–F4.
- **Anchors verified accurate.** Tasks name real files, functions, and struct fields
  that currently exist, so the GREEN steps are actionable rather than aspirational.
- **Excellent RED discipline that targets the audit's root cause.** RED tasks repeatedly
  *forbid* the exact anti-patterns that let the original bugs hide: "following the real
  `SelectSettledMsg` identity/update path rather than assigning `ResultView` fields
  directly" (072), "induce an actual driver `tx.Commit()` failure … rather than passing a
  preclassified `WriteResult`" (061), "make the new assertions fail for the current
  shallow copies" (081), and pervasive stale/cancelled/wrong-generation inertness controls.
- **Consistent "implement only enough to make Task 1 pass" restraint** in every GREEN
  step, with explicit control/regression rows guarding against over-correction.
- **Cross-issue "preserve behavior" callouts** (63↔64, 77↔78, 79↔80, 72↔73) reduce the
  merge/regression risk that shared-file edits create.

---

## P0 — Fix before implementation starts

### P0-1. Cross-issue prerequisites live only in prose; the structured `Depends on` field is misleading

Inside each task file the `Depends on` field is scoped **only to sibling task ordinals**
(`none`, `1`, `2`, …). Genuine **inter-issue** blocking is expressed only in RED-step
prose, and inconsistently:

- Task 073/1: `Depends on: none`, but prose says "Begin only after Issue #72 has
  preserved settlement metadata."
- Task 075/1: `Depends on: none`, prose "Begin only after Issues #72 and #74."
- Task 076/1: `Depends on: none`, prose "Begin only after Issues #71 and #72."
- Task 078/1: `Depends on: none`, prose "Begin only after Issue #77."
- Task 079/1: `Depends on: none`, prose "Begin only after Issue #78."
- Task 080/1: `Depends on: none`, prose "Begin only after Issues #73 and #79."
- Task 083/1: `Depends on: none`, prose "Begin only after Issue #82 has landed."
- Task 066/1: `Depends on: none`, prose "After Issue #65 is complete."
- Task 088/1: `Depends on: none`, prose "Begin only after Issues #57 through #87 have landed."

And the phrasing itself varies: some tasks say "Begin only after Issue #X," the classifier
cluster uses a "Nth change in the #77→#78→#79→#80 sequence" note, while **Task 064 never
states a start-gate at all** — it only says "Retain all Issue #53 stage failures and Issue
#63 … behavior," even though Issue 64 is explicitly *blocked by* 63 and rewrites the same
`WriteAtomic` boundary.

Because the parent issues *do* carry `Blocked by` fields, this information was **available
and dropped** at the task layer. Any scheduler (or human) that trusts the structured field
and reads `Depends on: none` will start ~10 task files out of order, guaranteeing rework on
the exact shared surfaces the tasks otherwise took care to coordinate (`applySelectSettled`,
`snapshot_metadata.go`, `history.Classify`, `WriteAtomic`).

**Recommendation:** give each task file an explicit machine-readable cross-issue
prerequisite (e.g. a `Blocked by issues:` header, or allow `Depends on` to reference
`#65/1` style external task IDs). At minimum, standardize on one prose form ("Begin only
after Issue #X") and add the missing gate to Task 064.

### P0-2. The headless-TUI harness (Tasks 57 & 88) is undecided and self-contradictory — and it is the critical path

Task 57 is the blocker that makes almost every other issue's manual/end-to-end
verification executable, and its automated proof (Tasks 57/5–6) plus the CI gate (Task
88/1) hinge on driving a Bubble Tea program unattended in headless CI. The tasks describe
this as **"a pseudo-terminal *or* an equivalent Bubble Tea input/output harness"** and,
in the same breath, require both:

- "**Build or invoke the shipped command** against a real temporary SQLite database" and
  "removing or bypassing the TUI composition causes it to fail" (⇒ a subprocess driven
  through a **PTY**), *and*
- "a **Bubble Tea test harness**" (⇒ `teatest`, which runs the program **in-process** and
  precisely **cannot** exercise the *built binary*).

These are different mechanisms with different guarantees, and the tasks never choose.
I confirmed the repo has **no `teatest` or PTY dependency today**, and `cmd/sqloid/main.go`
does not yet call `tea.NewProgram`/`ui.New`, so this is net-new, high-risk infrastructure
being specified by an "or." Leaving the choice open at the task layer means the RED author
in Task 57/5 cannot write a deterministic failing test, and Task 88/1's "invoke that
existing test … against the built `sqloid` binary" may be unsatisfiable if Task 57 chose
the in-process harness.

**Recommendation:** decide now, in Task 57, and make Task 88 consistent. Given Task 88
explicitly wants "building the actual `sqloid` binary" and the audit's "fail when
production composition is bypassed" requirement, commit to **PTY-driving-the-built-binary**
(e.g. `creack/pty`) as the CI capability gate, and relegate any in-process `teatest`
coverage to a clearly separate, non-binary fast test. Name the chosen library and add it as
a task deliverable so the dependency is vetted rather than improvised.

---

## P1 — Fix soon

### P1-1. Tasks 75 and 76 edit the same finalization/projection seams with no coordination

Both extend snapshot metadata through the identical finalization→projection plumbing:

- Task 75 edits `internal/ui/active_select.go`, `snapshot_metadata.go`,
  `internal/ui/result_history.go` (invalid-UTF + byte-cap warnings).
- Task 76 edits `internal/ui/active_select.go`, `snapshot_metadata.go`,
  `result_history.go`, plus `internal/history/snapshot.go`/`result_entry.go` (typed
  `LimitFailure`).

Their blockers don't order them relative to each other (75 ← {72,74}; 76 ← {71,72}), and
neither task references the other. This is precisely the uncoordinated shared-surface
pattern that the tasks correctly *avoided* for 72/73 and the 77–80 cluster. Whichever lands
second will merge-conflict on `Finalization`/`SnapshotMetadata` and the `projectHistoryEntry`
seam, and could silently drop the other's metadata field.

**Recommendation:** sequence 75 before 76 (or vice-versa) and add a "coordinate the shared
finalization/projection metadata seam with Issue #7x" note to both, mirroring the 72/73
language.

### P1-2. Task 57 is oversized and bundles separable, differently-risked deliverables

Task 57 packs eight subtasks: the composition root + real adapters (1–2), terminal
lifecycle/shutdown ordering (3–4), the headless binary capability test + harness (5–6), and
doc/walkthrough (7–8). Because 57 gates the manual/e2e verification of essentially every
other issue in the set, its size directly inflates the whole set's critical-path risk, and
it mixes low-risk wiring (composition root) with the highest-risk item in the entire program
(P0-2's harness).

**Recommendation:** split 57 so the **composition root + lifecycle/shutdown** (which
unblocks 58–87's manual/e2e verification) can land and be depended on *before* the
**headless capability harness** work is finished. This also lets the harness decision
(P0-2) be resolved without blocking every other issue.

### P1-3. Source/AST "tests" conflict with the PRD's stated testing philosophy; Task 69's RED→REFACTOR pairing is the weakest

The PRD's Testing Decisions say tests should "target external contracts rather than private
structure." Several tasks instead assert **code structure**:

- Task 69/1: "the repository-appropriate focused **source/AST assertion** … that
  `applyPickerFilenameKey` itself has no `tea.KeyLeft` or `tea.KeyRight` switch arms; this
  assertion must fail against the current dead branches."
- Task 85/1: "an architecture/**source-contract test** … the `strconv.FormatFloat` plus
  `.0` suffix algorithm appears only in the canonical result implementation."
- Task 84/1: "the **source-contract assertion** fails against the current alignment-only
  constant" (`RowidApplicable`).

For genuinely unobservable changes (removing unreachable code, de-duplicating an
implementation) there is no behavioral test, so a structural guard is *defensible* — but it
should be framed as a **lint/`go vet`/deadcode/grep build-check**, not a unit test used as
the primary RED gate. Task 69 is the sharpest case: its Task 1 is `RED` but its Task 2 is
`REFACTOR` (not `GREEN`), and the only thing that "fails then passes" is the AST assertion —
i.e. there is no behavioral red-green at all. The behavioral safety net (Left/Right still
toggle format; every remaining filename edit still works) is the real value; the AST check
is brittle ceremony.

**Recommendation:** keep the behavioral routing/regression tests as the RED/GREEN gate,
and demote the structural checks to a documented lint/vet step (or a single grep-based CI
assertion) rather than an AST unit test. Relabel Task 69/2 accordingly.

### P1-4. Task 73 specifies Page-Down suppression only for the *empty* first page, not the *short-but-nonempty* one

Task 73 sets `pageExhausted` for "fewer rows than that retained size, including zero," which
correctly establishes the high endpoint for **both** short and empty first pages. But its
Page-Down-suppression requirement is scoped only to the empty case: "an accepted **empty**
response makes Page Down a no-op" / "Page Down after an **empty** first page issues no
duplicate request." A short-but-nonempty first page has *also* reached the high endpoint, so
Page Down there should likewise be a no-op — yet no RED case pins that.

**Recommendation:** extend Task 73's RED matrix (and the GREEN guard) to require Page Down
to be a no-op after any `pageExhausted` first page, short or empty, not just empty.

### P1-5. The task-type taxonomy is undefined and inconsistently applied

Across the set the `Type` field takes at least six values: `RED`, `GREEN`, `DOCUMENT`,
`CODE WALKTHROUGH`, plus `REFACTOR` (069/2, 079/1, 087/1) and `CONFIG` (088/1). Only
RED/GREEN are (presumably) defined by the `code-writing/always-do-red-green` skill;
`REFACTOR` and `CONFIG` are never defined in-repo, and their verification expectations
differ (e.g. 079's `REFACTOR` relies on an *unchanged* truth table as its safety net; 088's
`CONFIG` has only manual inspection + a walkthrough negative demo). Task 069 also pairs a
`RED` with a `REFACTOR` rather than a `GREEN`, breaking the otherwise-uniform pattern.

**Recommendation:** define the allowed task types and their verification obligations once
(in the tasks' generating skill or a header), and either justify or normalize the
`RED`/`REFACTOR` pairing in 069.

---

## P2 — Nice-to-have

### P2-1. No task→acceptance-criteria traceability

Task files translate each issue's numbered acceptance criteria into RED/GREEN "Output"
lines but never map back to them (no "satisfies AC #1–5"). Coverage therefore has to be
re-derived by hand. I spot-verified two ACs the earlier issues-critique had flagged as
originally missing and both *are* covered — Issue 57 AC5 ("startup failure prints exactly
one stderr line, exits 1, no file") lands in Tasks 57/3–4, and Issue 68's popup/base-context
regression lands in Task 68/3 — but this is implicit. A short AC-to-task map per file would
make the DoD auditable.

### P2-2. The issues' *manual* verification steps have no owning task

Each issue lists Manual verification (launch the TUI, page beyond the first page, etc.).
The tasks convert only the *automated* verification into RED/GREEN and fold demonstration
into the CODE WALKTHROUGH tasks — which themselves depend on Issue 57. For AFK execution
the agent cannot perform manual TUI steps, so those checks are effectively deferred to the
walkthroughs and the Issue 88 CI gate. That's a reasonable model, but it should be stated
explicitly (e.g. "manual verification is discharged by the 057-gated walkthrough") so it
isn't mistaken for missing coverage.

### P2-3. Full walkthrough + dated wiki-log ceremony is disproportionate for the trivial issues

Every issue, including doc-only Issue 86 ("fix a comment"), script deletion Issue 87, and
enum cleanup Issue 84, mandates a full `showboat` CODE WALKTHROUGH plus a DOCUMENT task that
ingests into the wiki and appends a dated `log.md` entry. For mechanical/no-behavior-change
work this is heavy overhead relative to value. Consider allowing a lightweight
"note + verification output" artifact for `REFACTOR`/`DOCUMENT`-only issues instead of a
full walkthrough.

---

## Prioritized recommendations

1. **P0-1:** Put cross-issue prerequisites in a structured field on every task file;
   standardize the prose; add the missing start-gate to Task 064.
2. **P0-2:** Decide the headless-TUI harness in Task 57 (recommended: PTY over the built
   binary), name the dependency, and make Task 88 consistent; remove the "binary *or*
   in-process harness" ambiguity.
3. **P1-1 / P1-2:** Sequence-and-cross-reference Tasks 75/76; split Task 57 so the
   composition root lands ahead of the harness work.
4. **P1-3 / P1-5:** Demote source/AST assertions to lint/build checks (fix Task 69's
   RED→REFACTOR pairing); define the `REFACTOR`/`CONFIG` task types.
5. **P1-4:** Extend Task 73 to suppress Page Down after any exhausted first page, not just
   the empty one.
6. **P2:** Add per-file AC-to-task maps; state that manual verification is discharged via
   the 057-gated walkthroughs; consider lighter artifacts for trivial issues.

With P0-1 and P0-2 resolved (structured cross-issue ordering and a decided, dependency-vetted
TUI harness), this is a release-ready, well-decomposed task set that accurately and
testably operationalizes the final-audit remediation issues.
