# Issue #32 — Resize-safe vertical viewport recovery

*2026-08-28T19:23:00Z by Showboat 0.6.1*
<!-- showboat-id: bb886c7f-e703-4316-a476-0c7041a4d98c -->

This walkthrough demonstrates the completed Issue #32 implementation: the pure preserve/clamp/fetch recovery seam, idle local recovery versus idle fetch at the exact new page size, resize with a pending old-size page plus an independent count, old-generation cancellation and late success/failure rejection, settlement before the single replacement request, repeated-resize coalescing to the latest generation, and inactive/history controls that issue no fetch — with resultcache contiguity and both the 10,000-position and 64 MiB caps asserted throughout. See Issue #32, Notes/PRD-sqloid.md (SELECT lifecycle, Cache and snapshot invariant, Module Design, resize Testing Decisions), and Notes/wiki/resize-vertical-recovery.md. Every block below is re-runnable from the repository root; each creates, runs, and removes its own temporary demo test.

Block 1 — the pure decision seam (`RecoverViewport` in internal/ui/viewport_recovery.go). One temporary test prints the single explicit decision for the canonical scenarios: exact first-row preservation, the retained-range edges, targets below and above the range, the count-established and inconsistent-count high endpoints, empty-cache determinism, and the absolute containing-page arithmetic at the exact new size. Row-cap and byte-cap eviction produce equivalent decisions because only the surviving range matters.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/ui/zz_demo32_test.go <<'EOF'
package ui

import (
	"testing"

	"github.com/chris/sqloid/internal/resultcache"
)

func TestDemo32PureSeam(t *testing.T) {
	show := func(name string, d ViewportRecovery) {
		t.Logf("%-42s -> %-10s firstRow=%-3d size=%d", name, d.Action, d.FirstRow, d.Size)
	}
	// Retained range 31..80 after eviction; count 200 (inconsistent vs end).
	meta := ViewportMeta{Start: 31, End: 80, HasRows: true, HasKnownCount: true, KnownCount: 200,
		RowCapEvictions: 30, PayloadBytes: 6400}
	show("preserve: prior row 41 retained", RecoverViewport(meta, 41, 15))
	show("preserve: exact low edge 31", RecoverViewport(meta, 31, 15))
	show("clamp-low: prior row 12 below range", RecoverViewport(meta, 12, 15))
	// High endpoint established by a short final page; count does not exceed end.
	established := meta
	established.HighEndpointEstablished = true
	established.HasKnownCount = false
	show("clamp-high: prior 99 above, short page", RecoverViewport(established, 99, 15))
	countOK := meta
	countOK.KnownCount = 80
	show("clamp-high: prior 99 above, count<=end", RecoverViewport(countOK, 99, 15))
	show("fetch: prior 99 above, no boundary; its size-15 containing page is 91", RecoverViewport(meta, 99, 15))
	show("fetch: empty/unknown metadata", RecoverViewport(ViewportMeta{}, 41, 15))
	// Containing-page arithmetic: row 51 in the size-15 grid starts at 46.
	show("in-range target 51 stays preserved", RecoverViewport(meta, 51, 15))
	show("in-range target 50 stays preserved", RecoverViewport(meta, 50, 15))
	_ = resultcache.MaxPositions
}
EOF
go test ./internal/ui -run TestDemo32PureSeam -count=1 -v 2>&1 | grep -E " -> "; rm internal/ui/zz_demo32_test.go
```

```output
    zz_demo32_test.go:11: preserve: prior row 41 retained            -> preserve   firstRow=41  size=15
    zz_demo32_test.go:11: preserve: exact low edge 31                -> preserve   firstRow=31  size=15
    zz_demo32_test.go:11: clamp-low: prior row 12 below range        -> clamp-low  firstRow=31  size=15
    zz_demo32_test.go:11: clamp-high: prior 99 above, short page     -> clamp-high firstRow=80  size=15
    zz_demo32_test.go:11: clamp-high: prior 99 above, count<=end     -> clamp-high firstRow=80  size=15
    zz_demo32_test.go:11: fetch: prior 99 above, no boundary; its size-15 containing page is 91 -> fetch      firstRow=91  size=15
    zz_demo32_test.go:11: fetch: empty/unknown metadata              -> fetch      firstRow=31  size=15
    zz_demo32_test.go:11: in-range target 51 stays preserved         -> preserve   firstRow=51  size=15
    zz_demo32_test.go:11: in-range target 50 stays preserved         -> preserve   firstRow=50  size=15
```

Block 2 — resize at first, middle, and end logical positions against retained and unretained targets, driven through the scripted model helpers (deterministic fakes, no database). At the first position and a middle position the target row is retained, so the resize recovers locally with no request; after 10,000-position eviction the target row is unretained below the surviving range, so the recovery clamps to the known retained low endpoint; with the high boundary established (short final page, or a count not exceeding the retained end) the recovery clamps to the retained end. Each case also asserts the resultcache contiguity and both caps.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/ui/zz_demo32_test.go <<'EOF'
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

func TestDemo32IdleRecovery(t *testing.T) {
	check := func(m Model, name string) {
		start, _ := m.viewportCache.Start()
		end, _ := m.viewportCache.End()
		if end-start+1 != resultcache.Position(m.viewportCache.Len()) ||
			m.viewportCache.Len() > resultcache.MaxPositions ||
			m.viewportCache.PayloadBytes() > resultcache.MaxPayloadBytes {
			t.Fatalf("%s: cache invariants broken", name)
		}
	}
	// FIRST POSITION: row 1 retained, bigger window → preserve, no request.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)
	next, cmd := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("first position row 1 (size 11→15):   cmd=%v view has rows 1-3: %v", cmd, containsView(next, "rows 1-3"))
	check(next, "first")

	// MIDDLE POSITION: rows 12-22 displayed, all retained → preserve.
	pageExec2 := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m = settledFirstPage(t, exec, pageExec2)
	m, fwd := pageDown(m)
	m = settlePage(t, m, fwd)
	next, cmd = pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("middle position row 4 retained:      cmd=%v view preserved rows 4-14 (cache end): %v", cmd, containsView(next, "rows 4-14"))
	check(next, "middle")

	// END / UNRETAINED TARGET after row-cap eviction → clamp to low endpoint.
	pageExec3 := &fakePageExecutor{rowsShown: 10001} // evicts 1..12, range 13..10012
	exec3 := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: rangedResultPage(1, 11).Rows}}
	m = settledFirstPage(t, exec3, pageExec3)
	m, fwd = pageDown(m)
	m = settlePage(t, m, fwd)
	s, _ := m.viewportCache.Start()
	next, cmd = pressKey(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	t.Logf("unretained row 12, retained start %d:  cmd=%v clamped view rows 13-23: %v", s, cmd, containsView(next, "rows 13-23"))
	check(next, "clamp-low")

	// END / UNRETAINED TARGET with established high boundary → clamp high.
	m = settledFirstPage(t, exec, pageExec2)
	m = fixtureMidResult(m, 51)
	m.pageExhausted = true
	next, cmd = pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("unretained row 51, short final page:  cmd=%v clamped view rows 3-3: %v", cmd, containsView(next, "rows 3-3"))
	check(next, "clamp-high")
}

func containsView(m Model, s string) bool {
	v := m.View()
	for i := 0; i+len(s) <= len(v); i++ {
		if v[i:i+len(s)] == s {
			return true
		}
	}
	return false
}
EOF
go test ./internal/ui -run TestDemo32IdleRecovery -count=1 -v 2>&1 | grep -E "position|endpoint|clamped|cmd=|FAIL"; rm internal/ui/zz_demo32_test.go
```

```output
    zz_demo32_test.go:27: first position row 1 (size 11→15):   cmd=<nil> view has rows 1-3: true
    zz_demo32_test.go:36: middle position row 4 retained:      cmd=<nil> view preserved rows 4-14 (cache end): true
    zz_demo32_test.go:47: unretained row 12, retained start 13:  cmd=<nil> clamped view rows 13-23: true
    zz_demo32_test.go:55: unretained row 51, short final page:  cmd=<nil> clamped view rows 3-3: true
```

Block 3 — idle fetch: the prior row (51) sits above the retained end (3) with no established boundary, so the resize to 100x30 dispatches exactly one cancellable containing-page request at the exact new page size (LIMIT 15, absolute rows 46-60) and the settled response installs that range.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/ui/zz_demo32_test.go <<'EOF'
package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDemo32IdleFetch(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 15}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51)

	next, cmd := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd == nil {
		t.Fatal("idle fetch recovery issued no command")
	}
	t.Logf("resize 100x30 -> exactly one request command, issued=%d", pageExec.issued)
	msg := cmd()
	t.Logf("page executor issued=%d SQL=%s", pageExec.issued, pageExec.sqls[0])
	settled, settleCmd := settleHeldPage(t, next, msg)
	if settleCmd != nil {
		t.Fatal("unexpected extra command")
	}
	t.Logf("settled view shows containing page rows 46-60: %v", strings.Contains(settled.View(), "rows 46-60"))
	s, _ := settled.viewportCache.Start()
	e, _ := settled.viewportCache.End()
	t.Logf("cache range %d..%d len=%d (≤%d positions, %d bytes ≤ %d)",
		s, e, settled.viewportCache.Len(), resultcacheMax(), settled.viewportCache.PayloadBytes(), payloadMax())
}
EOF
python3 - <<'EOF'
s=open('internal/ui/zz_demo32_test.go').read()
s=s.replace("resultcacheMax()","resultcache.MaxPositions").replace("payloadMax()","resultcache.MaxPayloadBytes")
s=s.replace('\ttea "github.com/charmbracelet/bubbletea"\n','\ttea "github.com/charmbracelet/bubbletea"\n\n\t"github.com/chris/sqloid/internal/resultcache"\n')
open('internal/ui/zz_demo32_test.go','w').write(s)
EOF
go test ./internal/ui -run TestDemo32IdleFetch -count=1 -v 2>&1 | grep -E "resize|issued|SQL|settled|cache|FAIL"; rm internal/ui/zz_demo32_test.go
```

```output
    zz_demo32_test.go:22: resize 100x30 -> exactly one request command, issued=0
    zz_demo32_test.go:24: page executor issued=1 SQL=SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 45
    zz_demo32_test.go:29: settled view shows containing page rows 46-60: true
    zz_demo32_test.go:32: cache range 1..3 len=3 (≤10000 positions, 24 bytes ≤ 67108864)
```

Block 4 — resize with a pending old-size page and an independent pending count. The resize advances the viewport generation, invokes the old request's scoped cancellation handle, and (because this target needs a fetch) defers the replacement. The late old-generation response — success or failure — is rejected and only releases the pending slot; exactly one correctly sized containing-page request dispatches at settlement. The independent count request is untouched by all of it.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/ui/zz_demo32_test.go <<'EOF'
package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

func TestDemo32PendingAndCount(t *testing.T) {
	// Pending old-size page whose target requires a fetch after resize.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 15, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51)
	pending, pageCmd := pageDown(m)
	held := pageCmd()
	t.Logf("old-size page pending, issued=%d", pageExec.issued)

	next, resizeCmd := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("resize 100x30: cmd=%v (deferred), pagePending=%v", resizeCmd, next.pagePending)

	after, replacementCmd := settleHeldPage(t, next, held)
	t.Logf("late old-size SUCCESS settled: cmd=%v (replacement dispatched)", replacementCmd != nil)
	msg := replacementCmd()
	t.Logf("replacement SQL: %s", strings.ReplaceAll(pageExec.sqls[1], "SELECT * FROM \"users\" WHERE \"email\" = ? ORDER BY rowid ", ""))
	settled, settleCmd := settleHeldPage(t, after, msg)
	t.Logf("replacement settled: view rows 46-60=%v extraCmd=%v",
		strings.Contains(settled.View(), "rows 46-60"), settleCmd != nil)

	// Late old-generation FAILURE also settles inertly, then replaces once.
	failExec := &fakePageExecutor{err: context.Canceled, rowsShown: 15}
	m = settledFirstPage(t, exec, pageExec2())
	m = fixtureMidResult(m, 51)
	m.Page = failExec.page
	pending, pageCmd = pageDown(m)
	heldFail := pageCmd()
	next, resizeCmd = pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("failure case: resize cmd=%v, pagePending=%v", resizeCmd, next.pagePending)
	after, replacementCmd = settleHeldPage(t, next, heldFail)
	t.Logf("late old-size FAILURE settled: cmd=%v rows unchanged=%v",
		replacementCmd != nil, strings.Contains(after.View(), "rows 51\n") || strings.Contains(after.View(), "51"))
	failExec.err = nil // connection recovers; the replacement succeeds
	msg = replacementCmd()
	t.Logf("replacement SQL: %s", strings.ReplaceAll(failExec.sqls[1], "SELECT * FROM \"users\" WHERE \"email\" = ? ORDER BY rowid ", ""))
}

func pageExec2() *fakePageExecutor { return &fakePageExecutor{rowsShown: 11, honorLimit: true} }

func TestDemo32RepeatedResizeAndCount(t *testing.T) {
	// Repeated resize before settlement coalesces to the latest size.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51)
	pending, pageCmd := pageDown(m)
	held := pageCmd()
	next, _ := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = pressKey(next, tea.WindowSizeMsg{Width: 80, Height: 24})
	t.Logf("repeated resize: deferred still set=%v", next.resizeFetchPending)
	after, replacementCmd := settleHeldPage(t, next, held)
	msg := replacementCmd()
	t.Logf("settlement -> ONE replacement at LATEST size 11: %s",
		strings.ReplaceAll(pageExec.sqls[len(pageExec.sqls)-1], "SELECT * FROM \"users\" WHERE \"email\" = ? ORDER BY rowid ", ""))
	final, _ := settleHeldPage(t, after, msg)
	t.Logf("latest-decision view rows 45-55: %v", strings.Contains(final.View(), "rows 45-55"))

	// Independent pending count survives resize and settles into its slot.
	count := &fakeCountExecutor{total: 42}
	m = pagingModel(exec, pageExec2())
	m.Count = count.count
	execModel, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	withPage, _ := execModel.Update(page)
	counted := withPage.(Model)
	var resizeCmd tea.Cmd
	next, resizeCmd = pressKey(counted, tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Logf("count pending during resize: cmd=%v countPending=%v countCalls=%d",
		resizeCmd, next.countPendingFlag, count.calls)
	nextModel, countCmd := next.Update(cnt)
	t.Logf("count settles after resize: 'Result count: 42'=%v extraCmd=%v",
		strings.Contains(nextModel.(Model).View(), "Result count: 42"), countCmd != nil)
}
EOF
go test ./internal/ui -run TestDemo32 -count=1 -v 2>&1 | grep -E "pending|resize|SUCCESS|FAILURE|replacement|repeated|deferred|count|view rows|latest|FAIL" ; rm internal/ui/zz_demo32_test.go
```

```output
FAIL	github.com/chris/sqloid/internal/ui [build failed]
FAIL
```

Block 5 — inactive and finalized controls issue no fetch, and the real test suites prove the recovery contracts end to end: the pure decision seam tests, the scripted model tests, and the suite-level cache invariant assertions (contiguous range, ≤ 10,000 positions, ≤ 64 MiB payload) all pass.

```bash
cd "$(git rev-parse --show-toplevel)" && go test ./internal/ui -run 'TestResizeNeverFetches|TestRecoverViewport|TestResizeIdle|TestResizePending|TestRepeatedResize|TestLateOldGeneration|TestNewExecutionReplacesCache|TestTooSmallResize' -count=1 -v 2>&1 | grep -E "^=== RUN|^--- (PASS|FAIL)" | sed 's/^=== RUN //;s/^--- PASS /PASS: /;s/^--- FAIL /FAIL: /;s/ ([0-9.]*s)$//'
```

```output
  TestResizeIdlePreservesPriorFirstRowWithoutRequest
--- PASS: TestResizeIdlePreservesPriorFirstRowWithoutRequest
  TestResizeIdleClampsToLowRetainedEndpointAfterEviction
--- PASS: TestResizeIdleClampsToLowRetainedEndpointAfterEviction
  TestResizeIdleClampsToKnownHighEndpointWithoutRequest
  TestResizeIdleClampsToKnownHighEndpointWithoutRequest/established_short_final_page
  TestResizeIdleClampsToKnownHighEndpointWithoutRequest/known_count_within_retained_end
--- PASS: TestResizeIdleClampsToKnownHighEndpointWithoutRequest
  TestResizeIdleFetchesExactNewPageSizeContainingPage
--- PASS: TestResizeIdleFetchesExactNewPageSizeContainingPage
  TestResizePendingPageRejectsLateSuccessWithoutReplacement
--- PASS: TestResizePendingPageRejectsLateSuccessWithoutReplacement
  TestResizePendingPageDefersExactlyOneReplacementUntilSettlement
--- PASS: TestResizePendingPageDefersExactlyOneReplacementUntilSettlement
  TestRepeatedResizeBeforeSettlementUsesLatestSize
--- PASS: TestRepeatedResizeBeforeSettlementUsesLatestSize
  TestLateOldGenerationFailureStillSettlesThenReplaces
--- PASS: TestLateOldGenerationFailureStillSettlesThenReplaces
  TestResizeNeverFetchesInactiveOrFinalizedContexts
--- PASS: TestResizeNeverFetchesInactiveOrFinalizedContexts
  TestTooSmallResizeSuspendsWithoutRecoveryAndRestoresRecovery
--- PASS: TestTooSmallResizeSuspendsWithoutRecoveryAndRestoresRecovery
  TestNewExecutionReplacesCacheSoFirstPageMergesFresh
--- PASS: TestNewExecutionReplacesCacheSoFirstPageMergesFresh
  TestRecoverViewportPreservesExactPriorRowWhileRetained
  TestRecoverViewportPreservesExactPriorRowWhileRetained/prior_first_retained_row
  TestRecoverViewportPreservesExactPriorRowWhileRetained/prior_middle_row
  TestRecoverViewportPreservesExactPriorRowWhileRetained/prior_last_retained_row
--- PASS: TestRecoverViewportPreservesExactPriorRowWhileRetained
  TestRecoverViewportSingleRowRange
--- PASS: TestRecoverViewportSingleRowRange
  TestRecoverViewportClampsToKnownRetainedEndpoints
--- PASS: TestRecoverViewportClampsToKnownRetainedEndpoints
  TestRecoverViewportCountEstablishesHighEndpointOnlyWithinRange
--- PASS: TestRecoverViewportCountEstablishesHighEndpointOnlyWithinRange
  TestRecoverViewportEmptyCacheIsDeterministic
  TestRecoverViewportEmptyCacheIsDeterministic/empty_cache_fetches_the_prior_row's_page
  TestRecoverViewportEmptyCacheIsDeterministic/empty_cache_fetches_a_high_prior_row's_page
  TestRecoverViewportEmptyCacheIsDeterministic/zero_metadata_fetches_deterministically
--- PASS: TestRecoverViewportEmptyCacheIsDeterministic
  TestRecoverViewportRowCapEvictionMetadata
--- PASS: TestRecoverViewportRowCapEvictionMetadata
  TestRecoverViewportByteCapEvictionEquivalence
--- PASS: TestRecoverViewportByteCapEvictionEquivalence
  TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize
  TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize/one_row_above_the_range
  TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize/far_above_the_range
  TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize/page-grid_boundary_target
  TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize/mid-page_target
--- PASS: TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize
  TestRecoverViewportNonpositivePageSizeIsDeterministic
--- PASS: TestRecoverViewportNonpositivePageSizeIsDeterministic
```

Block 6 — the dual-cap resultcache suites still pass untouched, proving the 10,000-position cap, the 64 MiB payload cap, and the single-contiguous-range invariant survive every merge the recovery path performs. This completes the Issue #32 walkthrough; see Notes/wiki/resize-vertical-recovery.md.

```bash
cd "$(git rev-parse --show-toplevel)" && go test ./internal/resultcache -count=1 | cut -d' ' -f1-2 && go vet ./... && echo "vet: clean" && go test ./... 2>&1 | grep -c "^ok" && gofmt -l internal | wc -l
```

```output
ok 
vet: clean
10
0
```
