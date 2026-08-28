# Issue #16 Code Walkthrough: Ordered Projection Editing and Deduplication

*2026-08-27T10:04:34Z by Showboat 0.6.1*
<!-- showboat-id: 4fe0475a-de05-487b-bf34-331085457f45 -->

Issue #16 builds ordered projection editing and deduplication on top of the Issue #15 Column(s) path (see Notes/PRD-sqloid.md, Builder and Display Interaction plus the keyboard context matrix). Every transition below runs against an internal/querybuilder QueryBuilder for a users table with visible columns id, email, and a hidden created_at; the tiny driver is Notes/walkthroughs/016-06/_demo16/main.go.

```bash
go build -o /tmp/demo16 ./Notes/walkthroughs/016-06/_demo16 && /tmp/demo16 order
```

```output
after COUNT(*) accepted directly: [COUNT(*)]
insertion order across Value/Count/Min/Max/Avg/Sum: [COUNT(*) email id(COUNT) email(MIN) id(MAX) email(AVG) id(SUM)]
```

Entries stay append-only in insertion order: the same column email appears with Value, MIN, and AVG while id carries COUNT, MAX, SUM - distinct aggregates on one column, same aggregate on different columns, no sorting. Next, exact-duplicate rejection.

```bash
/tmp/demo16 duplicate
```

```output
before duplicate attempt: [email(AVG)]
rejected duplicate requests reopen: false
state after duplicate (email,Avg): [email(AVG)]
(id,Value) duplicate leaves identical pairs: true
later distinct append still lands last: [id id(MIN)]
```

An exact repeated (column, aggregate) pair - including the zero plain-Value aggregate on (id, Value) - is a full no-op: entries unchanged, no reopen requested, focus-transition data untouched, without reordering anything, while a later distinct append still lands last.

```bash
/tmp/demo16 sentinel
```

```output
sentinel coexisting with email(Min): [COUNT(*) email(MIN)]
direct duplicate-sentinel reopen request: false
state after direct duplicate-sentinel transition: [COUNT(*) email(MIN)]
```

The bare COUNT(*) sentinel coexists with later named aggregates even though the popup hides it once any entry exists; invoking the identical sentinel transition directly outside that conditional UI path changes nothing at all.

```bash
/tmp/demo16 wildcard
```

```output
populated state: [COUNT(*) id(COUNT) email(MIN)]
after wildcard selection (atomic replacement): [*]
beside-wildcard appends blocked: sentinel=true named=true
re-accepted wildcard still sole: true
removing sole wildcard empties: true
restored candidates: * , COUNT(*)
```

Wildcard selection from any populated state atomically clears every named and sentinel entry and becomes the sole projection; nothing may append beside it until removal empties the projection, after which QueryBuilder itself restores the Issue #15 empty candidate sequence (* first, COUNT(*) second) - no UI patching.

```bash
/tmp/demo16 remove
```

```output
press 1 (Backspace/Delete): [COUNT(*) id(MIN)]
press 2 (Backspace/Delete): [COUNT(*)]
press 3 (Backspace/Delete): []
press 4 (Backspace/Delete): []
empty popup candidates: * , COUNT(*)
```

The immutable RemoveLatestProjection transition is what the base Column(s) field routes Backspace/Delete through in internal/ui/model.go: one latest entry per press walking backward through email, id(MIN), then the bare sentinel, an exact unchanged no-op when already empty, and focus never moving off Column(s).

Key scope: Backspace/Delete inside a focused Column(s) search or the scroll-only aggregate popup remain governed by the reusable Issue #12 popup contract (search text editing with full reset semantics, no committed-entry deletion) - internal/ui/popup.go consumes those keys before base-context handling. Scripted coverage proves the whole contract end to end:

```bash
go test -count=1 ./internal/querybuilder -run 'TestInsertionOrderPreservedAcrossAggregatesOnSharedColumns|TestExactDuplicateNamedPairIsRejectedNoOp|TestDuplicateSentinelTransitionDirectlyIsNoOp|TestWildcardReplacesWholeListAtomicallyAndIsSole|TestMalformedIdentitiesCannotBypassInvariants|TestRemoveLatestProjectionRemovesOnlyLatestPreservingOrder'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

```bash
go test -count=1 ./internal/ui -run 'TestBackspaceDeletesRemoveExactlyLatestWalkingBackward|TestRemovingSoleWildcardEmptiesAndRestoresEmptyCandidates|TestPopupEditingKeysKeepPopupContract'
```

```output
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

internal/ui/projection_editing_test.go drives Update itself: repeated rounds remove exactly the latest entry (COUNT(*), id(MIN), email(Value)) preserving order and focus; removing the sole wildcard reopens Column(s) with wildcard first and COUNT(*) second; the same keys in a searchable Column(s) popup only edit/restore search text, and in the scroll-only aggregate popup they delete nothing while mode, opener, candidates, and open state stay untouched. Every code block above is reproducible via showboat verify.
