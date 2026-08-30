# Issue #054 Code Walkthrough: Contextual Help and Overlay Precedence

*2026-08-29T23:04:53Z by Showboat 0.6.1*
<!-- showboat-id: 4528bb6a-f571-4e79-9652-29ff125438c4 -->

Issue #54 (PRD Notes/PRD-sqloid.md, 'Global Key Precedence and Context/Action Matrix'): one ordered non-quit dispatcher — terminal state, contextual help overlay, top overlay, focused input/search, request restriction, base context, too-small suspension — consumes each key exactly once with no lower-layer leakage. This walkthrough drives representative and overlapping keys through every context, inserts literal '?' in builder text, searchable popups, and picker search, shows no-op overlay cases, opens base/WHERE/result/terminal help and proves exact opener restoration, and Esc-cancels help, popups, validation, estimate, confirmation, picker, overwrite, and save-error states. Universal q/Ctrl+C quit confirmation and quit's one-overlay suspension are owned by Issue #55 and are only excluded here.

```bash
go test -count=1 ./internal/ui -run 'TestKeyPrecedenceMatrix' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestKeyPrecedenceMatrix
    --- PASS: TestKeyPrecedenceMatrix/outcome-unknown_terminal
    --- PASS: TestKeyPrecedenceMatrix/searchable_popup
    --- PASS: TestKeyPrecedenceMatrix/scroll-only_popup
    --- PASS: TestKeyPrecedenceMatrix/focused_value_prompt
    --- PASS: TestKeyPrecedenceMatrix/first_page_pending
    --- PASS: TestKeyPrecedenceMatrix/noncancellable_write_phase_pending
    --- PASS: TestKeyPrecedenceMatrix/ordinary_base_builder
    --- PASS: TestKeyPrecedenceMatrix/too-small_screen
ok  	github.com/chris/sqloid/internal/ui	
```

The table-driven precedence matrix routes every non-quit key through the scripted contexts above, asserting exactly one consuming layer, no commands from lower layers, and untouched history/save/export invariants. Next, the literal-versus-contextual '?' routing and the no-op overlay cases.

```bash
go test -count=1 ./internal/ui -run 'TestQuestionMark' -v 2>&1 | grep -E '^(    --- (PASS|FAIL)|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestQuestionMarkInsertsLiterallyInFocusedInputs
    --- PASS: TestQuestionMarkInsertsLiterallyInFocusedInputs/universal_value_prompt_with_mid-buffer_cursor
    --- PASS: TestQuestionMarkInsertsLiterallyInFocusedInputs/searchable_popup_search
    --- PASS: TestQuestionMarkInsertsLiterallyInFocusedInputs/picker_filename_input
--- PASS: TestQuestionMarkOpensNonstackingContextualHelp
    --- PASS: TestQuestionMarkOpensNonstackingContextualHelp/builder_base
    --- PASS: TestQuestionMarkOpensNonstackingContextualHelp/where_field_focused
    --- PASS: TestQuestionMarkOpensNonstackingContextualHelp/settled_result_view
--- PASS: TestQuestionMarkIsNoOpInOverlays
    --- PASS: TestQuestionMarkIsNoOpInOverlays/preparation_modal
    --- PASS: TestQuestionMarkIsNoOpInOverlays/scroll-only_popup
    --- PASS: TestQuestionMarkIsNoOpInOverlays/quit_confirmation
    --- PASS: TestQuestionMarkIsNoOpInOverlays/too-small_screen
--- PASS: TestQuestionMarkNeverLeaksIntoLowerBaseAction
ok  	github.com/chris/sqloid/internal/ui	
```

```bash
go test -count=1 ./internal/ui -run 'TestRequired|TestReducedTerminalHelpContentAndOpenerState' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestRequiredWhereHelpContent
--- PASS: TestRequiredResultCountHelpContent
--- PASS: TestReducedTerminalHelpContentAndOpenerState
ok  	github.com/chris/sqloid/internal/ui	
```

Live demo: a scripted tour drives representative and overlapping keys through the real model in every context, printing the exact state the ordered dispatcher produces. The demo test is written into the package, executed, and removed by the same block, so verification re-runs it reproducibly.

```bash
cp /tmp/zz_demo54_test.go internal/ui/zz_demo54_test.go && go test -count=1 ./internal/ui -run TestWalkthrough54Tour -v 2>&1; rm -f internal/ui/zz_demo54_test.go
```

```output
=== RUN   TestWalkthrough54Tour
[too-small]  suspended=true helpOpen=false cmd==nil=true
[terminal]   reducedHelp=true selectionID=0
--- outcome-unknown terminal reduced help ---
Outcome unknown — the write's final state could not be proven

UPDATE outcome unknown: the commit did not resolve (disk I/O error); the statement reported 3 rows affected, which does not prove persistence
SQL: UPDATE "users" SET "email" = 'new' WHERE "id" = 5
Rows affected reported by the statement: this does not prove persistence

Ctrl+P / Ctrl+N query history · Ctrl+E / Ctrl+Y result history · ? help · q or Ctrl+C quits (status 1)

Reduced help — only these actions are available:

Ctrl+P / Ctrl+N   select an older/newer query from history
Ctrl+E / Ctrl+Y   select an older/newer result from history
Ctrl+S            save the selected or most recent query (as SQL)
Ctrl+X            export the selected tabular result; non-tabular is rejected
Esc               dismiss help
q or Ctrl+C       quit immediately (status 1)

Only these in-memory actions are available.
[terminal]   dismissed=true selectionPreserved=true noDatabaseAction=true
[popup]      search="?" helpOpen=false popupOpen=true
[popup]      ctrl+p cmd==nil=true search="?" popupOpen=true historyLen=0
[picker]     filenameHasQ=true helpOpen=false open=true
[value]      buffer="1?2" cursor=2 helpOpen=false
[prep modal] open=true helpOpen=false writePending=false
[scroll pop] open=true helpOpen=false
[pending]    notice="Running… — press Ctrl+W to cancel" cmd==nil=true
[pending]    ctrl+w cancelling=true cmd==nil=false
[builder]    helpKind="builder" focus=2
--- builder help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────────╮  │
││Query builder help                                                        │  │
││                                                                          │  │
││Tab / Shift+Tab     move between builder fields                           │  │
││Enter               open the focused field's popup, or run a valid query  │  │
││Backspace/Delete    clear the focused field's committed value             │  │
││Ctrl+P / Ctrl+N     browse query history (idle only)                      │  │
││Ctrl+E / Ctrl+Y     browse result history (idle only)                     │  │
││Ctrl+S / Ctrl+X     save the last query / export the active result        │  │
││Ctrl+W              cancel an active cancellable request                  │  │
││Esc                 clear a displayed error                               │  │
│╰──────────────────────────────────────────────────────────────────────────╯  │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│  Command: SELECT                                                             │
│  Table: users                                                                │
│> Column(s): *                                                                │
│  Where: "email" = ?                                                          │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help                                                                
[builder]    dismissed=true exactRestore=true
[where]      helpKind="where"
--- WHERE help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────╮      │
││WHERE help                                                            │      │
││                                                                      │      │
││A typed token spelled NULL binds as literal TEXT, never as SQL NULL.  │      │
││To test SQL NULL directly, use the IS NULL or IS NOT NULL operator.   │      │
││Ordinary comparisons and LIKE do not match rows where the column      │      │
││actually holds NULL.                                                  │      │
││'%' and '_' keep their SQLite wildcard meaning inside LIKE values.    │      │
│╰──────────────────────────────────────────────────────────────────────╯      │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│  Command: SELECT                                                             │
│  Table: users                                                                │
│  Column(s):                                                                  │
│> Where:                                                                      │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help                                                                
[where]      dismissed=true exactRestore=true
[result]     helpKind="result" resultAlive=true
--- result-count help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────╮      │
││Result count help                                                     │      │
││                                                                      │      │
││The count covers the complete executed SELECT, including your Limit:  │      │
││it is not a table count and not a pre-Limit row count.                │      │
││It runs as an independent autocommit read, so it may drift from the   │      │
││rows currently shown or cached.                                       │      │
││The count never clamps fetched pages or the retained result cache.    │      │
│╰──────────────────────────────────────────────────────────────────────╯      │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│> Command: SELECT                                                             │
│  Table: users                                                                │
│  Column(s): *                                                                │
│  Where: "email" = ?                                                          │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help                                                                
[result]     dismissed=true resultPreserved=true
[esc twice]  popupClosed=true cmd1==nil=true cmd2==nil=true baseIntact=true
--- PASS: TestWalkthrough54Tour (0.01s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.018s
```

The tour shows the terminal → top overlay → focused input/search → request restriction → base ordering end to end: too-small suspension consumes '?', the terminal's reduced help lists only its in-memory actions (no database suggestion), searchable-popup and picker searches insert '?' literally while overlapping Ctrl+P is consumed with no command, the preparation modal and scroll-only popup treat '?' as a no-op, pending Enter is rejected with exact feedback while cancellable Ctrl+W cancels, and each base help opens over an exact opener snapshot whose Esc restore leaves focus, scroll, fingerprint, and the settled result intact.

```bash
go test -count=1 ./internal/ui -run 'TestTopOverlayEscRestoration' -v 2>&1 | grep -E '^(    --- (PASS|FAIL)|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestTopOverlayEscRestoration
    --- PASS: TestTopOverlayEscRestoration/contextual_help_over_a_settled_result
    --- PASS: TestTopOverlayEscRestoration/searchable_popup
    --- PASS: TestTopOverlayEscRestoration/scroll-only_popup
    --- PASS: TestTopOverlayEscRestoration/multi-select_popup_keeps_completed_selections
    --- PASS: TestTopOverlayEscRestoration/stale_validation_retry
    --- PASS: TestTopOverlayEscRestoration/pending_schema_validation
    --- PASS: TestTopOverlayEscRestoration/destructive_preparation_modal
    --- PASS: TestTopOverlayEscRestoration/directory_picker
    --- PASS: TestTopOverlayEscRestoration/filename_entry_inside_the_picker
    --- PASS: TestTopOverlayEscRestoration/export_warnings
    --- PASS: TestTopOverlayEscRestoration/overwrite_confirmation
    --- PASS: TestTopOverlayEscRestoration/inline_save_failure
    --- PASS: TestTopOverlayEscRestoration/reduced_terminal_help
ok  	github.com/chris/sqloid/internal/ui	
```

```bash
cp /tmp/zz_demo54_test.go internal/ui/zz_demo54_test.go && go test -count=1 ./internal/ui -run TestWalkthrough54Tour -v 2>&1 | sed -E 's/ ?\(?[0-9]+\.[0-9]+s\)?[[:space:]]*$//; s/[[:space:]]+$//'; rm -f internal/ui/zz_demo54_test.go
```

```output
=== RUN   TestWalkthrough54Tour
[too-small]  suspended=true helpOpen=false cmd==nil=true
[terminal]   reducedHelp=true selectionID=0
--- outcome-unknown terminal reduced help ---
Outcome unknown — the write's final state could not be proven

UPDATE outcome unknown: the commit did not resolve (disk I/O error); the statement reported 3 rows affected, which does not prove persistence
SQL: UPDATE "users" SET "email" = 'new' WHERE "id" = 5
Rows affected reported by the statement: this does not prove persistence

Ctrl+P / Ctrl+N query history · Ctrl+E / Ctrl+Y result history · ? help · q or Ctrl+C quits (status 1)

Reduced help — only these actions are available:

Ctrl+P / Ctrl+N   select an older/newer query from history
Ctrl+E / Ctrl+Y   select an older/newer result from history
Ctrl+S            save the selected or most recent query (as SQL)
Ctrl+X            export the selected tabular result; non-tabular is rejected
Esc               dismiss help
q or Ctrl+C       quit immediately (status 1)

Only these in-memory actions are available.
[terminal]   dismissed=true selectionPreserved=true noDatabaseAction=true
[popup]      search="?" helpOpen=false popupOpen=true
[popup]      ctrl+p cmd==nil=true search="?" popupOpen=true historyLen=0
[picker]     filenameHasQ=true helpOpen=false open=true
[value]      buffer="1?2" cursor=2 helpOpen=false
[prep modal] open=true helpOpen=false writePending=false
[scroll pop] open=true helpOpen=false
[pending]    notice="Running… — press Ctrl+W to cancel" cmd==nil=true
[pending]    ctrl+w cancelling=true cmd==nil=false
[builder]    helpKind="builder" focus=2
--- builder help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────────╮  │
││Query builder help                                                        │  │
││                                                                          │  │
││Tab / Shift+Tab     move between builder fields                           │  │
││Enter               open the focused field's popup, or run a valid query  │  │
││Backspace/Delete    clear the focused field's committed value             │  │
││Ctrl+P / Ctrl+N     browse query history (idle only)                      │  │
││Ctrl+E / Ctrl+Y     browse result history (idle only)                     │  │
││Ctrl+S / Ctrl+X     save the last query / export the active result        │  │
││Ctrl+W              cancel an active cancellable request                  │  │
││Esc                 clear a displayed error                               │  │
│╰──────────────────────────────────────────────────────────────────────────╯  │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│  Command: SELECT                                                             │
│  Table: users                                                                │
│> Column(s): *                                                                │
│  Where: "email" = ?                                                          │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help
[builder]    dismissed=true exactRestore=true
[where]      helpKind="where"
--- WHERE help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────╮      │
││WHERE help                                                            │      │
││                                                                      │      │
││A typed token spelled NULL binds as literal TEXT, never as SQL NULL.  │      │
││To test SQL NULL directly, use the IS NULL or IS NOT NULL operator.   │      │
││Ordinary comparisons and LIKE do not match rows where the column      │      │
││actually holds NULL.                                                  │      │
││'%' and '_' keep their SQLite wildcard meaning inside LIKE values.    │      │
│╰──────────────────────────────────────────────────────────────────────╯      │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│  Command: SELECT                                                             │
│  Table: users                                                                │
│  Column(s):                                                                  │
│> Where:                                                                      │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help
[where]      dismissed=true exactRestore=true
[result]     helpKind="result" resultAlive=true
--- result-count help ---
╭──────────────────────────────────────────────────────────────────────────────╮
│╭──────────────────────────────────────────────────────────────────────╮      │
││Result count help                                                     │      │
││                                                                      │      │
││The count covers the complete executed SELECT, including your Limit:  │      │
││it is not a table count and not a pre-Limit row count.                │      │
││It runs as an independent autocommit read, so it may drift from the   │      │
││rows currently shown or cached.                                       │      │
││The count never clamps fetched pages or the retained result cache.    │      │
│╰──────────────────────────────────────────────────────────────────────╯      │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│> Command: SELECT                                                             │
│  Table: users                                                                │
│  Column(s): *                                                                │
│  Where: "email" = ?                                                          │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help
[result]     dismissed=true resultPreserved=true
[esc twice]  popupClosed=true cmd1==nil=true cmd2==nil=true baseIntact=true
--- PASS: TestWalkthrough54Tour
PASS
ok  	github.com/chris/sqloid/internal/ui
```
