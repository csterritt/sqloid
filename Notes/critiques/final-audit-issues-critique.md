# Issues Critique: Final-Audit Remediation Set (Issues 57–88)

Reviewer role: skeptical senior engineer / requirements-and-issues reviewer.
Scope: **issues-only review** of `Notes/issues/057-*.md` … `Notes/issues/088-*.md`.
Parent PRD: `Notes/PRD-sqloid.md`.
Source audit: `Notes/critiques/final-audit.md`.

> **Numbering note (2026-08-30):** This critique preserves the issue numbers that existed when it was written. The remediation issues were subsequently reordered for sequential implementation. Old → current: 57→57, 58→61, 59→71, 60→72, 61→73, 62→80, 63→64, 64→77, 65→65, 66→78, 67→74, 68→75, 69→66, 70→88, 71→63, 72→67, 73→58, 74→59, 75→60, 76→68, 77→70, 78→76, 79→62, 80→82, 81→81, 82→83, 83→84, 84→69, 85→79, 86→85, 87→86, 88→87. All issue references below use the old numbers.

This is not a code review or a task-generation pass. It evaluates the 32 issues as
work items: completeness, correctness of the stated fix, testability of acceptance
criteria, dependency/sequencing soundness, traceability, and best-practice/quality
problems.

---

## 1. Context and provenance

Issues 57–88 are a **1:1 remediation set** for `final-audit.md`. The mapping is exact:

| Audit finding | Issue | Audit finding | Issue |
|---|---|---|---|
| Critical 1 (no TUI composition) | 57 | Medium 3 (gate SELECT renderers) | 69 |
| High 1 (COMMIT → outcome-unknown) | 58 | Medium 4 (full DoD in CI) | 70 |
| High 2 (later-page offset) | 59 | Medium 5 (short-write cause) | 71 |
| High 3 (page metadata at settlement) | 60 | Medium 6 (stale INSERT prompts) | 72 |
| High 4 (short first page → high endpoint) | 61 | Medium 7 (stat EACCES/EPERM) | 73 |
| High 5 (active-export completeness) | 62 | Medium 8 (encode relative DSN) | 74 |
| High 6 (save overwrite race) | 63 | Medium 9 (pre-lease cancellation) | 75 |
| High 7 (completeness low endpoint) | 64 | Medium 10 (space key in inputs) | 76 |
| High 8 (SELECT projection staleness) | 65 | Medium 11 (grid separator packing) | 77 |
| High 9 (count/cache inconsistency) | 66 | Medium 12 (history limit-failure) | 78 |
| Medium 1 (UTF-8 maximal subparts) | 67 | Low 1 (rollback hook conn) | 79 |
| Medium 2 (history UTF/byte warnings) | 68 | Low 2 (malformed revalidation) | 80 |
|  |  | Low 3 (deep-copy BLOB) | 81 |
|  |  | Low 4 (Revalidation.Valid) | 82 |
|  |  | Low 5 (rowid enum) | 83 |
|  |  | Low 6 (filename arrow cases) | 84 |
|  |  | Low 7 (traversal Limit fields) | 85 |
|  |  | Low 8 (finite-REAL token) | 86 |
|  |  | Low 9 (Display doc) | 87 |
|  |  | Low 10 (stale scripts) | 88 |

**Coverage is complete and faithful.** Every audit finding became exactly one issue, and
no issue invents scope outside the audit. I independently re-verified several anchor
claims: no production `.go` file imports `internal/ui` (only tests/demos do), confirming
Issue 57; `RowidApplicable = iota` exists unused, confirming Issue 83; the two stale
scripts and every referenced source file exist. The set is well written — precise,
string-exact where it matters (e.g. `Result truncated: 64 MiB cache limit`, the row-`N`
diagnostics), and consistently equipped with manual + automated verification and
regression/control acceptance criteria.

The problems below are therefore about **decomposition, sequencing, and a few
scope/traceability gaps**, not about missing coverage.

---

## 2. Major issues

### 2.1 Issues 65 and 69 overlap and duplicate the SELECT-renderer gate

The audit deliberately split two concerns: **High #8** = add projection-staleness
validation *to the runnable report*; **Medium #3** = *gate the SELECT-family renderers*
on that report. The issues did not preserve that split cleanly.

- Issue 65 ("What to build") already says: *"Gate `SelectSQL`, `PageSQL`, and therefore
  `CountSQL` through that authoritative report and the SELECT command …"* and its
  AC #3 asserts the three renderers *"emit no executable SQL."*
- Issue 69 is **entirely** about gating those same three renderers on the runnable
  report, and lists Issue 65 as its blocker.

So the renderer-gating work is specified in both, with near-identical acceptance
criteria. Whoever implements 65 will either (a) do 69's job too, leaving 69 an empty
no-op PR, or (b) do half of it, leaving an ambiguous seam. This is a real
decomposition defect.

**Recommendation:** rescope Issue 65 to the *report-only* change (add
`validateProjection`/`RunFieldProjection` to `reportSelect`; assert `Runnable=false`
and specific feedback) and move **all** renderer-gating language and acceptance
criteria into Issue 69. Issue 65's AC #3 and the "Gate `SelectSQL`/`PageSQL`/`CountSQL`"
sentence should be deleted from 65 and owned solely by 69.

### 2.2 A cluster of issues silently share `history.Classify` / `TraversalFacts`, but none declare the dependency

Four issues edit the same completeness machinery, and **all four are marked
"Blocked by: None — can start immediately"**:

- **Issue 64** changes `history.Classify` semantics (adds `!ReachedLow` → partial;
  relaxes `complete` for empty results).
- **Issue 66** adds `Finalization.CountCacheInconsistent`, which flows into
  `TraversalFacts` and is *read by* `Classify` to forbid `complete`.
- **Issue 85** *removes* `HasLimit`/`Limit` from `TraversalFacts`.
- **Issue 62** wants "one shared helper used by active export and
  `appendFinalizedResultEntry`" so the two classification paths cannot drift.

These are not independent. They edit the same struct (`TraversalFacts`) and the same
classifier, and 62's "shared helper" goal is undermined if 64/66 land afterward and
touch only one of the two paths. Running them "in parallel" (as the AFK/None-blocker
labeling invites) will cause merge conflicts and, worse, semantic races where one
issue's tests assume a `Classify` behavior another issue just changed.

**Recommendation:** establish an explicit order and record it in `Blocked by`. A sound
sequence is **64 → 66 → 85 → 62** (settle classifier semantics, then add the
inconsistency input, then remove dead fields, then unify the two producers on the final
shape). At minimum, cross-reference the four issues so an implementer knows they touch a
shared surface.

### 2.3 First-page settlement issues (60 and 61) edit the same functions without coordination

Both Issue 60 and Issue 61 modify `applySelectSettled` in
`internal/ui/first_select.go` (and 60 also `paging.go`). Issue 60 copies
`ByteTruncated`/`LimitFailure` on settlement; Issue 61 retains the requested page size
and sets `pageExhausted` on settlement. They are complementary but touch the same
narrow code path and neither references the other. Issues 68 and 78 (both correctly
`Blocked by: Issue 60`) then build further metadata persistence on top of that same
path.

**Recommendation:** mark Issue 61 and Issue 60 as coordinated (or sequence one before
the other) so the shared `applySelectSettled` change is authored once rather than
conflict-merged twice.

---

## 3. Moderate issues

### 3.1 Manual verification steps across the whole set presuppose Issue 57, which is unbuilt

Nearly every issue's **Manual** verification tells the reviewer to launch/drive the TUI
("open the export picker …", "type and edit spaces in WHERE/SET values …", "page beyond
the first result page …", "finalize a SELECT and browse its history …"). But per Issue
57 and the audit, the shipped binary does not currently start the TUI at all. Until
Issue 57 lands, **none of those manual steps are executable**. The automated,
package-level tests can run without 57, but the manual columns cannot.

None of these issues note the implicit dependency. This is a genuine traceability gap:
the labeling "Blocked by: None — can start immediately" is true for the *unit tests*
but false for the *manual verification* of most issues.

**Recommendation:** either (a) add a note to the affected issues that manual/end-to-end
verification is gated on Issue 57, or (b) explicitly state that these issues are
verified at the package/seam level pre-57 and re-verified end-to-end post-57. This also
argues for landing Issue 57 first regardless of its "None" blocker.

### 3.2 Issues 57 and 70 both "add" the binary/TUI integration test

Issue 57 says *"Add a production-level binary test that proves a valid database enters
the TUI …"*. Issue 70 says *"Add a real built-binary/TUI integration test that exercises
production composition …"* and lists 57 as its blocker. These describe the same test.
70 should be scoped to **wiring 57's test into CI** (plus repository-wide
`test/build/vet`), not re-adding it. As written, ownership of the test is duplicated.

Separately, **neither 57 nor 70 addresses how a Bubble Tea program is driven in a
headless CI environment** (PTY allocation, or the `teatest`/`tea` test harness). "Drive
a real temporary SQLite database through startup into Bubble Tea" is non-trivial without
a terminal. This is the single biggest implementation risk in the set and deserves an
explicit note or acceptance criterion (e.g. "the integration test uses a pseudo-terminal
/ teatest harness and runs unattended in CI").

**Recommendation:** in Issue 70, change "Add a real built-binary/TUI integration test"
to "run the Issue 57 binary/TUI integration test in CI on Linux and macOS," and add a
verification note about PTY/teatest so the test is actually CI-runnable.

### 3.3 Issue 63: the "no-replace" vs "temp-file-plus-rename" design tension is under-specified

Issue 63 requires both:
- an "atomic no-replace primitive" for unconfirmed new-file saves, **and**
- "Preserve destination-local temporary-file atomicity."

These are in tension: the existing atomicity comes from `os.Rename` over the target,
which *always replaces*. A no-replace guarantee for the new-file case cannot use
rename-over-existing; it needs `O_CREATE|O_EXCL` (or `link`/`renameat2(RENAME_NOREPLACE)`).
The issue gestures at this ("supported-platform atomic no-replace primitive") but never
defines the platform scope (the PRD targets Linux/macOS only) nor reconciles the two
mechanisms in the acceptance criteria. An implementer could reasonably read this two
ways.

**Recommendation:** state the platform scope (Linux/macOS) and name the intended
mechanism (e.g. `O_EXCL` create for the unconfirmed path; rename only after a confirmed,
unchanged inspection), and add an acceptance criterion that the temp file still lives in
the destination directory in both paths.

### 3.4 Issue 67 scopes the maximal-subpart fix to E0–EF only

The audit's Medium #1 called out two shapes: valid U+FFFD mis-flagged once any malformed
byte appears (general), and the `maximalSubpart` E0–EF three-byte gap (specific). Issue
67 faithfully covers both, but restricts the multibyte fix to **E0–EF three-byte
leads**. It says nothing about **F0–F4 four-byte leads**, which have the analogous
"valid lead + valid continuation(s) + invalid/missing later byte" maximal-subpart
subtlety. If the same `maximalSubpart` routine mishandles four-byte sequences, this
issue would leave that latent.

**Recommendation:** either broaden the acceptance criteria to cover F0–F4 four-byte
maximal subparts, or add an explicit statement that four-byte handling was verified
correct and is intentionally out of scope.

---

## 4. Minor issues

### 4.1 Issue 73 mis-cites User Story 90

Issue 73 fixes the **stat/readable-check** stage (EACCES/EPERM mapped to "missing") and
cites US3, US7, and **US90**. But US90 is specifically about **mode=rw open-stage**
failures ("distinguish permission denied and read-only filesystem from other raw
OS/driver causes") — a *different* validation stage that the audit found already
correct. US3 ("missing, unreadable … rejected") is the accurate story here. Citing US90
conflates two stages. Drop US90 from Issue 73 (or replace with a note that the stat
stage is distinct from the mode=rw stage of US90).

### 4.2 Issue 83 could also correct the untyped const block

The remediation ("remove `RowidApplicable`, start at `iota + 1`") is right, but note the
current block declares its constants **without the `RowidCapability` type** — they are
untyped ints. A thorough fix should type the block (`RowidHas RowidCapability = iota + 1`)
so the enum values carry the intended type. Worth folding into the same issue since it is
the same three lines.

### 4.3 The issues are heavily implementation-prescriptive (brittleness)

Every issue names exact files, functions, struct fields, and in some cases exact code
expressions (e.g. Issue 64's `complete := … && (high == 0 || meta.ReachedLow) &&
fullRetention`). For AFK/autonomous execution this is helpful, but it couples the issue
text to the current code layout: the referenced functions/fields must still exist when
the issue is worked, and several issues touch the same files (see §2.2–§2.3), so the
*first* issue to land can invalidate later issues' line-level references. This is a
quality trade-off worth acknowledging in the set's intro, and another reason to sequence
the shared-surface clusters.

### 4.4 A few missing regression acceptance criteria

- **Issue 57** rewrites `main`/composition but has no acceptance criterion that
  **startup *failures*** still print exactly one stderr line and exit 1 without creating
  a file (the behavior Issues 2–4/73/74 depend on). Add a regression AC.
- **Issue 76** (space key) verifies value/filename insertion but has no AC that
  **popup-search and base-context** space behavior is *unchanged* (the "verify" prose
  mentions it, but it is not an acceptance criterion).

### 4.5 Issue 87 is inherently low-verifiability

Issue 87 is documentation-only and its automated verification is conditional ("add
documentation-oriented assertions only if the repository already enforces comment text").
That is honest, but it means the issue's core change (comment wording) has no hard gate.
Acceptable for a Low, but consider adding a checklist item that a reviewer confirms the
`Display()` doc no longer references CSV/JSON exporters, so completion is at least
manually gated.

### 4.6 "Type: AFK" is undocumented and uniformly applied

Every issue is `Type: AFK` with almost every one `Blocked by: None`. The label is never
defined in-repo, and its uniform "None" blocker under-represents the real coordination
dependencies identified in §2. Not blocking, but the blocker field should reflect §2.1–§2.3.

---

## 5. Strengths worth preserving

- **Exact 1:1 traceability** to the audit and clean forward traceability to PRD user
  stories (each issue lists the stories it satisfies).
- **String-exact acceptance criteria** for user-visible text and diagnostics, which
  makes them objectively testable.
- **Strong test discipline**: issues repeatedly forbid tests that mutate `ResultView`
  or inject pre-classified results directly, insisting failures be driven through the
  real settlement/driver path (Issues 58, 59, 60, 68, 78). This directly targets the
  audit's core complaint that existing green tests hid real defects.
- **Good control/regression coverage** in most acceptance criteria (e.g. 58 AC#4, 59
  AC#4, 61 AC#4, 66 AC#3–4, 75 AC#3), guarding against over-correction.
- **Correct existing dependencies** where declared: 68→60, 78→60, 69→65, 70→57 are all
  sound (the gaps are the *missing* dependencies in §2, not wrong ones).

---

## 6. Prioritized recommendations

1. **Fix the 65/69 overlap** (§2.1): make 65 report-only, 69 renderer-gating-only.
2. **Sequence the `Classify`/`TraversalFacts` cluster** 64 → 66 → 85 → 62 and record it
   in `Blocked by` (§2.2); coordinate 60/61 on `applySelectSettled` (§2.3).
3. **Land Issue 57 first** and annotate the set that manual/end-to-end verification is
   gated on it (§3.1); de-duplicate the integration test between 57 and 70 and add a
   PTY/teatest note so it is CI-runnable (§3.2).
4. **Tighten Issue 63** with platform scope + named no-replace mechanism (§3.3) and
   **Issue 67** with F0–F4 four-byte coverage or an explicit out-of-scope note (§3.4).
5. **Minor cleanups**: drop the US90 cite from 73 (§4.1); type the enum block in 83
   (§4.2); add the missing regression ACs to 57 and 76 (§4.4).

With the decomposition and sequencing corrections above, this is a high-quality,
release-ready issue set that fully and accurately operationalizes the final audit.
