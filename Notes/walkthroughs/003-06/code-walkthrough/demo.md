# Issue #3 — local D1 candidate discovery walkthrough

*2026-08-26T21:02:57Z by Showboat 0.6.1*
<!-- showboat-id: f37a9e3f-7f98-4141-bf3e-e83d7677f1c3 -->

Sqloid Issue #3 implements `sqloid d1`: discover the single local Wrangler D1 database per Notes/PRD-sqloid.md's D1 discovery section, applying the exact candidate and exclusion rules and handing the sole candidate unchanged to Issue #2's shared opener (internal/connection).

Discovery lives in internal/d1. The exact Wrangler directory is a package constant, inspected non-recursively, and eligibility is purely name-based:

```bash
grep -n 'const Dir' -A 2 /home/chris/sqloid/internal/d1/discovery.go && grep -n 'func eligible' -A 8 /home/chris/sqloid/internal/d1/discovery.go
```

```output
18:const Dir = ".wrangler/state/v3/d1/miniflare-D1DatabaseObject"
19-
20-// ErrNoCandidate reports that the Wrangler D1 directory is absent or contains
66:func eligible(name string) bool {
67-	if strings.Contains(name, "metadata") {
68-		return false
69-	}
70-	if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
71-		return false
72-	}
73-	return strings.HasSuffix(name, ".sqlite")
74-}
```

Table-driven filesystem tests pin every rule — exact case-sensitive .sqlite extension, lowercase metadata exclusion (uppercase Metadata stays eligible), -wal/-shm sidecars, nested files, alternate layouts, and zero/one/multiple cardinality:

```bash
cd /home/chris/sqloid && go test ./internal/d1/ -count=1 -v -run 'TestDiscover' 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s$//'
```

```output
=== RUN   TestDiscoverSoleCandidate
--- PASS: TestDiscoverSoleCandidate (0.00s)
=== RUN   TestDiscoverZeroCandidates
=== RUN   TestDiscoverZeroCandidates/directory_absent_entirely
=== RUN   TestDiscoverZeroCandidates/directory_exists_but_empty
=== RUN   TestDiscoverZeroCandidates/only_metadata_file
=== RUN   TestDiscoverZeroCandidates/only_sidecar_files
=== RUN   TestDiscoverZeroCandidates/only_wrong-case_extension
=== RUN   TestDiscoverZeroCandidates/only_nested_sqlite
=== RUN   TestDiscoverZeroCandidates/alternate_layout_directory_only
--- PASS: TestDiscoverZeroCandidates (0.00s)
=== RUN   TestDiscoverMultipleCandidates
=== RUN   TestDiscoverMultipleCandidates/two_plain_candidates
=== RUN   TestDiscoverMultipleCandidates/uppercase_Metadata_does_not_match_lowercase_rule
--- PASS: TestDiscoverMultipleCandidates (0.00s)
=== RUN   TestDiscoverExclusionsStillLeaveSoleCandidate
=== RUN   TestDiscoverExclusionsStillLeaveSoleCandidate/metadata_and_sidecars_ignored
=== RUN   TestDiscoverExclusionsStillLeaveSoleCandidate/uppercase_metadata_substring_is_not_the_lowercase_exclusion
=== RUN   TestDiscoverExclusionsStillLeaveSoleCandidate/nested_ignored_while_top-level_survives
--- PASS: TestDiscoverExclusionsStillLeaveSoleCandidate (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/d1	
```

The handoff: internal/cli.RunD1 requests the sole candidate from internal/d1 and passes that path unchanged to the shared internal/connection opener — there is no D1-specific validation or SQLite-opening path.

```bash
cd /home/chris/sqloid && sed -n '16,20p' internal/cli/d1.go && go test ./internal/cli/ -count=1 -run 'TestRunD1PassesSoleCandidateUnchangedToSharedOpener|TestD1EndToEndOpensSoleDiscoveredCandidate' -v 2>&1 | grep -E '^(--- |ok |FAIL)' | sed 's/[0-9]\+\.[0-9]\+s$//'
```

```output
// other handler error.
func RunD1() error {
	return runD1With(d1.Discover, connection.Session)
}

--- PASS: TestRunD1PassesSoleCandidateUnchangedToSharedOpener (0.00s)
--- PASS: TestD1EndToEndOpensSoleDiscoveredCandidate (0.01s)
ok  	github.com/chris/sqloid/internal/cli	
```

Manual proof on a mixed fixture: one exact .sqlite candidate plus lowercase-metadata, sidecar, wrong-case, nested, and alternate-layout files that must all be ignored. Successful startup is silent (exit 0).

```bash
cd "$(mktemp -d)" && mkdir -p .wrangler/state/v3/d1/miniflare-D1DatabaseObject/nested ".wrangler/state/v3/wrangler-state/v2/d1/miniflare-D1DatabaseObject" && python3 -c "
import sqlite3
c=sqlite3.connect(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite\")
c.execute(\"CREATE TABLE t(id INTEGER)\"); c.commit()
c2=sqlite3.connect(\".wrangler/state/v3/wrangler-state/v2/d1/miniflare-D1DatabaseObject/alternate.sqlite\")
c2.execute(\"CREATE TABLE t(id INTEGER)\"); c2.commit()
open(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite-wal\",\"w\").close()
open(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite-shm\",\"w\").close()
open(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/db-metadata.sqlite\",\"w\").close()
open(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/ABC.SQLITE\",\"w\").close()
open(\".wrangler/state/v3/d1/miniflare-D1DatabaseObject/nested/deep.sqlite\",\"w\").close()
" && go build -C /home/chris/sqloid -o /tmp/sqloid-demo ./cmd/sqloid && find .wrangler -type f | sort && /tmp/sqloid-demo d1; echo "exit=$?"
```

```output
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite-shm
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/abc123.sqlite-wal
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/ABC.SQLITE
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/db-metadata.sqlite
.wrangler/state/v3/d1/miniflare-D1DatabaseObject/nested/deep.sqlite
.wrangler/state/v3/wrangler-state/v2/d1/miniflare-D1DatabaseObject/alternate.sqlite
exit=0
```
