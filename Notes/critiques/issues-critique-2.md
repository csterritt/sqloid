# Issues Critique 2 — Sqloid v1

Reviewer: skeptical senior engineer, issues-only review.

Inputs:

- `Notes/PRD-sqloid.md` (authoritative)
- `Notes/Ideas.md`
- `Notes/skills/critique-issues/SKILL.md`
- all 56 files in `Notes/issues/`
- `Notes/critiques/issues-critique.md`, used only to distinguish resolved findings from regressions in the revised issue set

Scope: the issues themselves—their completeness, sequencing, dependencies, acceptance criteria, verification, logic, and ownership—not the PRD and not implementation code.

---

## Executive summary

The revised issue set is materially better than the version reviewed previously. It now has an early integration tracer, early cancellation infrastructure, query-history append before the first real execution, deletion/replacement terminal ownership, a release-capability owner, non-finite grid rendering, and corrected dependencies around query saving and quit cleanup. The 90 PRD user stories remain represented, and most high-risk requirements have an identifiable owner.

However, the revision introduced one **hard dependency cycle**:

- Issue 41 is blocked by Issue 43.
- Issue 43 is blocked by Issue 41.

No implementation order can satisfy that graph. In addition, Issue 24 still claims responsibility for estimate and write cancellation before those workflows exist, the terminal-state issues require save/export behavior that is implemented later, and Issue 49 omits the outcome-unknown terminal workflow from its blockers. These are implementation-stalling issues rather than cosmetic concerns.

The issue set should not be treated as implementation-ready until the P0 findings are resolved.

Priority definitions used below:

- **P0**: blocks or makes the implementation plan internally impossible; fix before implementation.
- **P1**: substantial sequencing, coverage, or verification weakness; fix before the affected area is implemented.
- **P2**: quality and precision improvement; fix when editing the relevant issue.

---

## Strengths

1. **The previous critique was acted on rather than merely acknowledged.** The added `005b`, `008b`, `017b`, `019b`, `023b`, `040b`, and `050` issues address real ownership and sequencing gaps.
2. **The first actual execution now has query-history support.** Issue 17b places stable IDs, append timing, consecutive suppression, and the 20-entry cap before Issue 19.
3. **Cancellation infrastructure is available before schema validation.** Issue 5b breaks the former circular sequencing problem between validation and the later user-facing cancellation work.
4. **The first grid now owns the representation it needs.** Issue 19 includes deduplicated names, finite REAL rendering, invalid UTF-8 replacement, visible controls, and BLOB display instead of waiting until Issue 41.
5. **Previously missing cross-cutting ownership now exists.** Issue 40b owns deletion/replacement terminal behavior, and Issue 50 owns the assembled release/driver capability gate.
6. **Several dependency corrections are sound.** In particular, Issue 42 now depends on result history, Issue 49 depends on SELECT/write cleanup, and Issue 33 explicitly depends on the shared WHERE flow.
7. **The issue template remains consistent.** Every issue identifies type, blockers, PRD parent, build scope, verification, acceptance criteria, and user stories.
8. **The test strategy is generally strong.** The issue set repeatedly calls for deterministic fake-Connection tests, SQLite integration tests, synchronization barriers rather than sleeps, exact-byte serializer tests, and the required manual rendering matrix.

---

## P0 — Fix before implementation

### P0-1. Issues 41 and 43 form a direct dependency cycle

`041-deduplicated-names-and-exact-typed-value-rendering.md` says:

- `Blocked by: Issue 19, Issue 43`

`043-immutable-result-export-capture-and-warnings.md` says:

- `Blocked by: Issue 30, Issue 32, Issue 41`

Therefore 41 cannot start until 43 completes, while 43 cannot start until 41 completes. This contradicts the prior critique's resolution note, which says Issue 41 was repurposed to extract the rendering logic for later exporters.

Issue 43 does not need to precede that extraction. It captures immutable rows and metadata for export; Issue 41 supplies the shared names/value representation that Issue 43 and the serializers consume.

**Recommendation:** remove Issue 43 from Issue 41's blockers. Keep Issue 43 blocked by Issue 41. The resulting intended chain is:

`19 → 41 → 43 → 44/45/46...`

Also amend Issue 41's wording so it does not claim that exporters already consume the module before Issues 44 and 45 exist. Its acceptance should establish the reusable API and grid migration; Issues 44 and 45 should prove CSV/JSON consumption.

### P0-2. Issue 24 claims cancellation wiring for workflows that do not exist yet

`024-scoped-ctrl-w-cancellation-and-bounded-settlement.md` is blocked by Issues 5b, 22, and 23, but its scope says it implements cancellation for:

- schema validation,
- estimation,
- SELECT,
- write phases.

At that point in the plan:

- schema validation exists in Issue 18,
- SELECT requests exist,
- estimation is not introduced until Issues 36–37,
- write phases are not introduced until Issues 38–39.

Issue 38 then depends on Issue 24. If Issue 24's definition of done really includes write-phase integration, it cannot complete before Issue 38; if it completes without that integration, its stated scope and acceptance are false. Estimation has the same sequencing problem, though without the explicit reverse dependency.

Issue 5b correctly owns reusable context/interrupt infrastructure. The remaining application work needs to be assigned where the actual state machines exist.

**Recommendation:** narrow Issue 24 to SELECT page/count cancellation, shared UI settlement behavior, and reusable application-level cancellation helpers. Assign:

- schema-validation Ctrl+W integration to Issue 18,
- estimate Ctrl+W integration to Issue 37,
- beginning/executing write cancellation to Issue 38,
- post-boundary non-interrupt behavior to Issue 39.

Alternatively, split Issue 24 into phase-specific issues placed after the relevant workflows. Do not leave acceptance that can only be satisfied by future issues.

### P0-3. Terminal workflows depend on save/export features that are absent from their graphs, and Issue 49 omits Issue 40

Issue 40's verification requires browsing entries, saving SQL, and attempting export in the outcome-unknown terminal state. It is blocked only by Issues 32 and 39.

Issue 40b likewise requires Ctrl+S and Ctrl+X behavior in deletion/replacement terminal states but is blocked only by Issues 6 and 32.

The actual action implementations arrive later:

- Issue 42 implements Ctrl+S targeting and SQL serialization.
- Issue 43 implements Ctrl+X targeting, immutable capture, and non-tabular rejection.
- Issues 46–47 implement the picker, overwrite, and save completion paths.

As written, Issues 40 and 40b cannot run their own stated manual/automated verification when they are reached. They can implement terminal-state entry and gating, but not the complete in-memory save/export workflow they claim to deliver.

There is a second omission: Issue 49 promises immediate status-1 quit in terminal states, but its blockers include Issue 40b and not Issue 40. Thus its dependency graph covers deletion/replacement terminal states but not the outcome-unknown terminal state.

**Recommendation:**

1. Split Issues 40/40b into terminal-state entry/navigation and later terminal save/export integration, or move their complete integration after Issues 42–47.
2. Add explicit coverage for both positive and negative terminal export paths: an earlier selected tabular snapshot exports; a selected outcome/error/write-summary entry rejects without opening a picker.
3. Define the empty-result-history fallback for deletion/replacement detected before any result exists.
4. Add Issue 40 as a blocker of Issue 49, not only Issue 40b.

---

## P1 — Fix before the affected area

### P1-1. Filename order is no longer a valid implementation order

The prior critique and its resolution treated filename order as the implementation order and placed `b` issues to insert work at specific points. The revised graph violates that convention:

- Issue 23b is located between Issues 23 and 24 but is blocked by Issues 38 and 39.
- Issue 41 points forward to Issue 43 and, before P0-1 is fixed, cycles with it.

Even after fixing the cycle, an agent processing files in filename order will stop at Issue 23b or implement it without its prerequisites.

**Recommendation:** rename/reposition Issue 23b after Issue 39 (for example, `039b-write-phase-in-flight-feedback.md`) or explicitly declare that blocker topology—not filename order—is authoritative and provide a generated topological order. Given the existing naming scheme and prior decisions, renaming is less error-prone.

Issue 23b should also state whether it extends a feedback/restriction abstraction created by Issue 23. If reuse is required, add Issue 23 as a blocker; if not, explain why the two implementations will not diverge.

### P1-2. Concurrent requests precede the identity and execution-gating invariants that make them safe

Issue 20 launches the first page and count concurrently. Issue 22 later introduces execution IDs, request IDs, and viewport generations. Issue 23 later prevents Enter/history/save/export from stacking work while requests are pending.

The PRD says every response may mutate state only when its execution/request/generation is current. Introducing concurrency first means Issue 20 either:

- implements temporary response handling that Issue 22 replaces,
- silently implements part of Issue 22,
- or permits stale responses during the intermediate milestone.

This is most dangerous for a slow count returning after a newer execution, and for a count failure racing a still-pending first page.

**Recommendation:** split minimal execution/request identity and pending-execution gating ahead of Issue 20, leaving page viewport generations and broader stale-response cases in Issue 22. At minimum, Issue 20 must accept a count/page response only for the current execution and current request, and its tests should include a slow old count returning after replacement work.

### P1-3. Issue 50 does not assemble all mandatory driver-upgrade cancellation evidence

Issue 50 is a good addition, but its integrated cancellation list covers page/count interruption, isolation, late success, CPU bounds, and lock bounds. The PRD's mandatory pinned-modernc release/dependency-upgrade blocker also explicitly requires:

- cancelling schema validation with no history,
- cancelling estimation with no history,
- pre-COMMIT cancellation rollback,
- proof that post-boundary Ctrl+W issues no interrupt.

Issue 50 includes the last two transaction items but omits schema-validation and estimate cancellation from its assembled scope. Its journal section also omits the mandatory external-writer delay/error case stated in the PRD's high-risk coverage.

Per-issue tests are not enough because Issue 50 exists specifically to ensure the complete gate is assembled and run on both platforms for every release/driver upgrade.

**Recommendation:** add schema-validation cancellation, estimate cancellation, no-history assertions for both, and the external-writer delay/error journal case to Issue 50's scope and acceptance. Its CI-gate test inventory should be traceable one-for-one to PRD high-risk items 2 and 3.

### P1-4. The disposable tracer has no owned removal criterion

Issue 8b explicitly says its hardcoded query path is disposable and replaced when Issue 19 lands. Issue 19 does not require removal of that path, and no cleanup issue owns it.

A tracer that remains callable creates two execution paths: one bypassing validation/history/cancellation and one conforming to the product lifecycle. Even unreachable dead code would leave a misleading alternate architecture.

**Recommendation:** add to Issue 19's acceptance criteria that the hardcoded Issue 8b execution path is removed or fully replaced, with only the production builder/validation path remaining. Reusable integration-test fixtures may remain, but not a second runtime path.

### P1-5. Issue 28's acceptance criteria are too vague for exact safety diagnostics and persistent metadata

Issue 28's prose and automated verification mention distinct page/value over-limit failures, but its acceptance criterion merely says “the correct distinct message” for either case. The PRD requires two exact messages:

- `result page exceeds the 64 MiB v1 limit at row N`
- `result value exceeds the 64 MiB v1 limit at row N`

The PRD also requires byte-cap eviction to record `truncated-by-byte-cap` and show exactly `Result truncated: 64 MiB cache limit` in both the result header and export flow. Issue 28 says only that the warning is “recorded,” while Issue 29's acceptance criteria do not explicitly prove persistence of the byte-cap flag into snapshots.

**Recommendation:** give page and value failures separate acceptance criteria with exact strings and one-based positions. Add an explicit metadata criterion for `truncated-by-byte-cap`, then require Issue 29/43 tests to prove that finalized snapshots and export warnings retain it.

### P1-6. Terminal export acceptance covers rejection better than allowed export

Issues 40 and 40b emphasize the Ctrl+X rejection path for selected non-tabular terminal entries. The PRD is broader: after Ctrl+E/Y selects an older tabular snapshot, Ctrl+X must export that snapshot even in a terminal session because no database work is needed.

Issue 43's generic “idle tabular selection” criterion may cover this indirectly, but neither terminal issue proves it, and their current dependency ordering prevents integrated verification anyway.

**Recommendation:** add explicit terminal-state acceptance for:

- selecting an older tabular snapshot and exporting it without database access,
- selecting a non-tabular entry and receiving the exact rejection without opening a picker,
- empty result history,
- query-history selection changing Ctrl+S's in-memory target without enabling database work.

---

## P2 — Precision and maintainability improvements

### P2-1. Issues 17b and 31 overlap ownership of query-history append

Issue 17b correctly introduces stable IDs, append timing, normalized consecutive suppression, and oldest-first eviction. Issue 31 later says “Implement” the same 20-entry stable-ID history and repeats append/suppression responsibilities.

The intended distinction is navigation/restoration/selected-entry fallback, but the current wording makes Issue 31 look like a second implementation of the append store.

**Recommendation:** rewrite Issue 31 as “extend the Issue 17b history store with Ctrl+P/N navigation, immutable restoration, and selected-entry eviction fallback.” Its tests should prove the earlier append contract remains unchanged rather than re-owning it.

### P2-2. Issue 11's “visibly stale” result is not objectively specified

Issue 11 requires a prior schema list to remain “visibly stale,” but neither its acceptance criteria nor verification identifies the exact indicator. A model can technically retain stale data while providing no useful user warning.

**Recommendation:** specify the status text or stable semantic UI state that marks stale schema data, and assert that the stale indicator remains until retry succeeds or the flow is cancelled/terminated.

### P2-3. Issue 16's LIMIT acceptance should name invalid boundary classes

Issue 16 says only integers from 1 through max int64 are accepted. That logically excludes bad values, but the PRD and test philosophy call out zero, malformed input, and overflow separately. These are common parser mistakes and deserve explicit acceptance.

**Recommendation:** add one criterion covering empty-as-unbounded and separate invalid examples for zero, negative, malformed, and int64 overflow, with focus/reason behavior delegated to Issue 17.

### P2-4. Issue 44 should state the intentional CSV lossiness explicitly

Issue 44 says NULL and empty text follow the documented policy, and its golden tests mention empty fields, but it does not state the important consequence: both serialize to the same empty CSV field. This is an intentional PRD limitation, not an implementation accident.

**Recommendation:** add an acceptance criterion proving NULL and empty TEXT both emit an empty CSV field while JSON preserves their distinction.

### P2-5. Issue 40b should define terminal entry with no prior result

Issue 40b says the most recent result-history entry is selected initially. It does not define the valid early-session case where deletion/replacement is detected before any execution has produced a result.

**Recommendation:** specify that terminal messaging remains visible, result selection is empty/base, in-memory actions report their normal no-target feedback, no missing backing entry is rendered, and q/Ctrl+C still exits status 1.

---

## Findings from the previous critique that are now resolved

The following earlier findings should be considered closed, subject to the new issues above:

1. **Rendering after the first grid:** Issue 19 now owns initial grid deduplication and value rendering.
2. **Late query-history append:** Issue 17b now precedes Issue 19.
3. **Validation before cancellation infrastructure:** Issue 5b now precedes Issue 18.
4. **Write feedback mixed into SELECT feedback:** Issue 23 is narrowed and Issue 23b owns write-phase feedback, though its placement still needs correction.
5. **Missing deletion/replacement terminal owner:** Issue 40b now exists.
6. **Missing release-capability owner:** Issue 50 now exists, though its integrated inventory needs expansion.
7. **Missing save-to-result-history dependency:** Issue 42 now depends on Issue 32.
8. **Missing quit cleanup dependencies:** Issue 49 now depends on active SELECT and write cleanup; only Issue 40 remains missing.
9. **Implicit rowid-order fallback:** Issue 21 now owns it.
10. **Thin identity/runnable acceptance:** Issues 6, 17, and 22 were expanded.
11. **Unspecified non-finite REAL grid rendering:** Issue 19b now defines it.
12. **No early integration milestone:** Issue 8b now provides one, with removal ownership still outstanding.

---

## Recommended correction order

1. Break the Issue 41 ↔ 43 cycle.
2. Narrow/split Issue 24 so each phase owns cancellation only after that phase exists.
3. Restructure Issues 40/40b around the later save/export implementations and add Issue 40 to Issue 49's blockers.
4. Move/rename Issue 23b so filename order and blocker order agree.
5. Introduce minimal execution/request identity before Issue 20 concurrency.
6. Expand Issue 50 to exactly mirror every mandatory release/driver-upgrade capability case.
7. Add tracer removal and exact cache/oversize acceptance criteria.
8. Apply the P2 wording and edge-case improvements.

---

## Bottom line

The issue set has strong PRD traceability, good test instincts, and substantially improved ownership. It is close to a workable implementation plan, but it is not currently executable as a dependency graph because of the 41/43 cycle. Fixing that cycle alone is not sufficient: Issue 24 and the terminal workflows also need their scope aligned with when their dependencies actually exist. Once the P0 findings and the filename-order mismatch are corrected, the remaining findings are mostly acceptance-hardening rather than architectural redesign.
