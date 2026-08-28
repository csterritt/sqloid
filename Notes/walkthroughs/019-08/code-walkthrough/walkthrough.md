# Issue #19: Authoritative runnable-state feedback

*2026-08-27T15:28:07Z by Showboat 0.6.1*
<!-- showboat-id: baccb510-529f-48a8-9c8f-8eda8d9dfa9f -->

Issue #19 delivers the authoritative runnable-state feedback per the Runnable-State Contract, Builder and Display Interaction, Global Key Precedence, QueryBuilder, UI, and Testing Decisions sections of Notes/PRD-sqloid.md: a pure UI-independent runnable report across all four commands, general whole-value clearing, and base-context Enter gating. Every artifact lives under this approved directory: _demo19/main.go is the runnable demonstration program. Section 1 proves the table-driven report suite covering every SELECT, UPDATE, DELETE, and INSERT prerequisite plus the common gates, with multi-failure cases returning only the first visual field and one reason.

```bash
go test ./internal/querybuilder -count=1 -run 'TestRunnable' 2>&1 | grep -E '^(ok|FAIL)'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

Section 2: runnable demonstration program, part 1 — SELECT and UPDATE reports in visual order. Note the multi-failure case reporting only Column(s) (first visual field) despite an open draft and invalid Limit; the exact Limit reason; and the SQL-NULL choice versus a typed TEXT NULL submission (the Value choice stays runnable with a TEXT bound parameter).

```bash
go run ./Notes/walkthroughs/019-08/code-walkthrough/_demo19 2>&1 | sed -n '/== 1/,/== 3/p' | head -32
```

```output
== 1. SELECT runnable reports in visual order ==
missing command                                INVALID [Command] select a command
no table                                       INVALID [Table] select a table
empty projection                               INVALID [Column(s)] select at least one column
open WHERE draft (with empty projection)       INVALID [Column(s)] select at least one column
multi-failure: projection+draft+limit -> first visual only INVALID [Column(s)] select at least one column
mixed agg/nonagg without GROUP BY              INVALID [GROUP BY] every nonaggregate projected column must be grouped
wildcard + GROUP BY                            INVALID [GROUP BY] the wildcard cannot be used together with GROUP BY
stale grouped column after refresh             INVALID [GROUP BY] a grouped column no longer exists
stale ORDER BY after refresh                   INVALID [ORDER BY] the ordered expression is no longer offered
invalid Limit abc                              INVALID [Limit] Limit must be an integer from 1 to 9223372036854775807
zero Limit                                     INVALID [Limit] Limit must be an integer from 1 to 9223372036854775807
empty Limit (unbounded) + submitted empty TEXT WHERE RUNNABLE
fully valid SELECT                             RUNNABLE

== 2. UPDATE runnable reports ==
no SET assignments                             INVALID [SET] add at least one SET assignment
duplicate SET columns (first matching completed) INVALID [SET] SET columns must be unique
incomplete Value/NULL choice                   INVALID [SET] complete the choice for column name
unsubmitted Value entry                        INVALID [SET] submit a value for column name
SQL-NULL choice complete                       RUNNABLE
typed TEXT NULL submission (Value choice)      RUNNABLE
                                               bound kind=TEXT text="NULL" (not SQL NULL)
open WHERE draft behind complete SET           INVALID [WHERE] complete the open value prompt
valid UPDATE                                   RUNNABLE

== 3. DELETE runnable reports ==
```

Section 3: DELETE and INSERT. DELETE needs only an eligible table (optional complete WHERE); INSERT blocks zero-insertable-column tables with the exact PRD wording, treats unbegun prompts and pending choices as incomplete, accepts submitted empty TEXT, and validates the all-omit state that later emits DEFAULT VALUES.

```bash
go run ./Notes/walkthroughs/019-08/code-walkthrough/_demo19 2>&1 | sed -n '/== 3/,/== 5/p' | head -18
```

```output
== 3. DELETE runnable reports ==
eligible table, absent WHERE                   RUNNABLE
complete WHERE                                 RUNNABLE
incomplete WHERE draft                         INVALID [WHERE] complete the open value prompt

== 4. INSERT runnable reports ==
zero insertable columns (blobs_only)           INVALID [INSERT] table has no insertable columns
view is never an INSERT target                 INVALID [Table] select a table
prompts not begun -> first insertable incomplete INVALID [INSERT] complete the choice for column id
begun, all incomplete (id first)               INVALID [INSERT] complete the choice for column id
id NULL; name still incomplete                 INVALID [INSERT] complete the choice for column name
name Value unsubmitted                         INVALID [INSERT] submit a value for column name
name Value = submitted empty TEXT              INVALID [INSERT] complete the choice for column score
all prompts complete                           RUNNABLE
valid all-omit state (DEFAULT VALUES later)    RUNNABLE

== 5. whole-value clearing (Backspace and Delete share one transition) ==
```

Section 4: whole-value clearing. Both Backspace and Delete route through one shared immutable transition (the scripted UI tests drive both keys; the demo shows the transition itself): a completed value-taking WHERE reopens as an incomplete draft preserving its column and operator, invalid and valid Limit text restores the valid unbounded state, absent/empty fields are exact no-ops, and the shared forward-compatible UPDATE/INSERT Value transition keeps the Value choice while dropping the submission — without claiming the Issue #37/#39 end-to-end write flows are complete.

```bash
go run ./Notes/walkthroughs/019-08/code-walkthrough/_demo19 2>&1 | sed -n '/== 5/,$p'
```

```output
== 5. whole-value clearing (Backspace and Delete share one transition) ==
WHERE cleared: committed=false drafting=true -> report [WHERE] complete the open value prompt
  preserved: column="name" operator==; submission gone=true
WHERE absent no-op: unchanged=true
Limit '9' cleared: input="" value accepted=false runnable=true
Limit 'abc' cleared: input="" runnable=true
empty Limit no-op: unchanged=true
UPDATE Value cleared: choice=Value submitted=false -> report submit a value for column name
INSERT Value cleared: choice=Value submitted=false -> report submit a value for column id
  (UPDATE/INSERT end-to-end flows land in Issues #37/#39; the transitions are shared now)
```

Section 5: scripted model evidence — Backspace/Delete clearing on focused base fields (empty no-ops, preserved structural choices, popup/value-prompt key scope, Issue #16 projection removal untouched) via the immutable transition tests and scripted UI tests.

```bash
go test ./internal/querybuilder -count=1 -run 'TestClear' 2>&1 | grep -E '^(ok|FAIL)'; go test ./internal/ui -count=1 -run 'TestBackspaceAndDelete|TestClearing|TestPopupAndPrompt|TestProjectionRemoval' 2>&1 | grep -E '^(ok|FAIL)'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

Section 6: scripted (model, msg) -> (model, cmd) evidence for Enter gating. Invalid data (multi-failure SELECT focused on Limit, representative duplicate-SET UPDATE, incomplete-WHERE DELETE, zero-insertable INSERT) consumes Enter and moves focus to the report's first invalid visual field with the exact inline reason — including Limit showing exactly 'abc — Limit must be an integer from 1 to 9223372036854775807' — and returns no validation, estimation, execution, or history command. Runnable data in base context emits exactly one PreExecutionRequestedMsg (the pre-execution seam; nothing executes in this issue).

```bash
go test ./internal/ui -count=1 -run 'TestEnter' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)' | head -20
```

```output
=== RUN   TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight
--- PASS: TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight (0.00s)
=== RUN   TestEnterOnMultiFailureMovesFocusToFirstInvalidField
--- PASS: TestEnterOnMultiFailureMovesFocusToFirstInvalidField (0.00s)
=== RUN   TestEnterOnInvalidLimitFocusesLimitWithExactReason
--- PASS: TestEnterOnInvalidLimitFocusesLimitWithExactReason (0.00s)
=== RUN   TestEnterOnInvalidWriteStatesFocusesWriteTargets
=== RUN   TestEnterOnInvalidWriteStatesFocusesWriteTargets/duplicate_SET_columns
=== RUN   TestEnterOnInvalidWriteStatesFocusesWriteTargets/incomplete_DELETE_WHERE
=== RUN   TestEnterOnInvalidWriteStatesFocusesWriteTargets/zero_insertable_columns
--- PASS: TestEnterOnInvalidWriteStatesFocusesWriteTargets (0.00s)
=== RUN   TestEnterOnRunnableDataEmitsOnlyPreExecutionSeam
--- PASS: TestEnterOnRunnableDataEmitsOnlyPreExecutionSeam (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

Section 7: runnable data in higher-precedence contexts. Popup, focused text/search input, the stale-refresh overlay, a pending request, and the too-small suspended screen each consume Enter with their own behavior and no runnable action, per the Global Key Precedence table.

```bash
go test ./internal/ui -count=1 -run 'TestRunnableDataInHigherPrecedence|TestRunnableDataWithPendingRequest|TestRunnableDataTooSmall' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestRunnableDataInHigherPrecedenceContextsConsumesEnterLocally (0.00s)
--- PASS: TestRunnableDataWithPendingRequestConsumesEnter (0.00s)
--- PASS: TestRunnableDataTooSmallConsumesEnter (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

Verified against Issue #19 and the Runnable-State Contract of Notes/PRD-sqloid.md. References: internal/querybuilder/runnable.go, write_state.go, whole_value.go; internal/ui/runnable_feedback.go, model.go; tests runnable_test.go, whole_value_test.go, whole_value_clearing_test.go, runnable_feedback_test.go; wiki page runnable-state-feedback.md.
