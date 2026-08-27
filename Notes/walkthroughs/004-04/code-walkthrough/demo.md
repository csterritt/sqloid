# Issue #4 — exact D1 discovery diagnostics walkthrough

*2026-08-26T21:35:21Z by Showboat 0.6.1*
<!-- showboat-id: a9b96978-baf0-42b5-8e54-260461d8e67b -->

Sqloid Issue #4 (per Notes/PRD-sqloid.md, D1 discovery section) defines the exact process-facing diagnostics for `sqloid d1` discovery failures. internal/d1 keeps only typed outcomes; internal/cli owns the mapping; the shared opener and any database creation are bypassed on every failure.

The mapping code in internal/cli/d1.go:

```bash
grep -n 'zeroCandidateHint\|func mapDiscoveryDiagnostic' -A 14 /home/chris/sqloid/internal/cli/d1.go
```

```output
10:// zeroCandidateHint is the exact second diagnostic line required by Issue #4
11-// and the D1 discovery section of Notes/PRD-sqloid.md: it names the expected
12-// working-directory-relative Wrangler path and gives explicit-open recovery
13-// guidance. It appears only for zero-candidate outcomes.
14:const zeroCandidateHint = "Expected " + d1.Dir + "; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly."
15-
16-// RunD1 is the D1 startup handler for Handlers.D1: it requests the sole
17-// candidate path from internal/d1's exact-rule discovery, maps every typed
18-// discovery failure onto its exact Issue #4 diagnostic without calling the
19-// opener, and passes a discovered path unchanged to the shared Issue #2
20-// pre-open validation, read-write open, and schema-probe flow in
21-// internal/connection. There is deliberately no D1-specific validation or
22-// SQLite-opening path here.
23-//
24-// Discovery failures carry typed outcomes from internal/d1 and are mapped
25-// here to the exact Issue #4 diagnostics before Main renders them verbatim
26-// with exit status 1; no database target is created for any of them.
27-func RunD1() error {
28-	return runD1With(d1.Discover, connection.Session)
--
49:func mapDiscoveryDiagnostic(err error) error {
50-	switch {
51-	case errors.Is(err, d1.ErrNoCandidate):
52:		return errors.New(d1.ErrNoCandidate.Error() + "\n" + zeroCandidateHint)
53-	case errors.Is(err, d1.ErrMultipleCandidates):
54-		// The sentinel text is lower-case by Go convention; the PRD-mandated
55-		// diagnostic capitalizes the first word, so it is spelled out here.
56-		return errors.New("There is more than one SQLite database in .wrangler")
57-	default:
58-		return err
59-	}
60-}
```

Golden process tests pin exact stderr bytes, line counts, hint presence/absence, exit status 1, and non-creation over every Issue #4 fixture:

```bash
cd /home/chris/sqloid && go test ./internal/cli -count=1 -run 'TestRunD1DiscoveryFailureMapsExactDiagnosticAndSkipsOpener|TestD1DiscoveryFailureProcessBehavior|TestD1DiscoveryUnreadableDirectoryProcessBehavior' 2>&1 | sed 's/[0-9]\+\.[0-9]\+s$//; s/(cached)//; s/[[:space:]]*$//'
```

```output
ok  	github.com/chris/sqloid/internal/cli
```

Manual demo — missing Wrangler directory: exactly two stderr lines, stdout silent, exit status 1.

```bash
mkdir -p /tmp/d1demo-missing && cd /tmp/d1demo-missing && /tmp/sqloid-bin d1; echo "exit=$?"
```

```output
no candidate database found in .wrangler
Expected .wrangler/state/v3/d1/miniflare-D1DatabaseObject; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.
exit=1
```

Empty directory — identical two-line zero-candidate output, with the expected path named plus the explicit sqloid sqlite <file> recovery hint.

```bash
mkdir -p /tmp/d1demo-empty/.wrangler/state/v3/d1/miniflare-D1DatabaseObject && cd /tmp/d1demo-empty && /tmp/sqloid-bin d1; echo "exit=$?"
```

```output
no candidate database found in .wrangler
Expected .wrangler/state/v3/d1/miniflare-D1DatabaseObject; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.
exit=1
```

Candidate-free directory (only a metadata-excluded name) — still exactly the same two lines.

```bash
mkdir -p /tmp/d1demo-free/.wrangler/state/v3/d1/miniflare-D1DatabaseObject && touch /tmp/d1demo-free/.wrangler/state/v3/d1/miniflare-D1DatabaseObject/state-metadata.sqlite && cd /tmp/d1demo-free && /tmp/sqloid-bin d1; echo "exit=$?"
```

```output
no candidate database found in .wrangler
Expected .wrangler/state/v3/d1/miniflare-D1DatabaseObject; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.
exit=1
```

Multiple candidates — only the exact single line, with no expected-path or explicit-open hint.

```bash
mkdir -p /tmp/d1demo-multi/.wrangler/state/v3/d1/miniflare-D1DatabaseObject && touch /tmp/d1demo-multi/.wrangler/state/v3/d1/miniflare-D1DatabaseObject/first.sqlite /tmp/d1demo-multi/.wrangler/state/v3/d1/miniflare-D1DatabaseObject/second.sqlite && cd /tmp/d1demo-multi && /tmp/sqloid-bin d1; echo "exit=$?"
```

```output
There is more than one SQLite database in .wrangler
exit=1
```

Unreadable directory (mode 000) — the same zero-candidate two lines; permission cases cannot run as root, which the golden tests skip explicitly.

```bash
mkdir -p /tmp/d1demo-unreadable/.wrangler/state/v3/d1/miniflare-D1DatabaseObject && chmod 000 /tmp/d1demo-unreadable/.wrangler/state/v3/d1/miniflare-D1DatabaseObject && cd /tmp/d1demo-unreadable && /tmp/sqloid-bin d1; rc=$?; echo "exit=$rc"; chmod 755 /tmp/d1demo-unreadable/.wrangler/state/v3/d1/miniflare-D1DatabaseObject
```

```output
no candidate database found in .wrangler
Expected .wrangler/state/v3/d1/miniflare-D1DatabaseObject; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.
exit=1
```

Non-creation proof: every failure leaves the working tree untouched — no database target is created and internal/connection.Session is never invoked on a discovery failure (the golden tests assert opener non-invocation via the injected seam, plus before/after snapshots of the whole working directory).

```bash
mkdir -p /tmp/d1demo-nocreate && cd /tmp/d1demo-nocreate && find . | sort > /tmp/before.txt && /tmp/sqloid-bin d1 2>/dev/null; find . | sort > /tmp/after.txt && diff /tmp/before.txt /tmp/after.txt && echo 'working tree unchanged; no database created'
```

```output
working tree unchanged; no database created
```

Also pinning stderr vs stdout ownership: the diagnostics go only to stderr. And a final full-suite verification:

The diagnostics go only to stderr:

```bash
cd /tmp/d1demo-missing && /tmp/sqloid-bin d1 2>/dev/null; echo 'nothing above came from stdout'
```

```output
nothing above came from stdout
```

```bash
cd /home/chris/sqloid && gofmt -l internal/cli; go vet ./... && echo 'vet OK'
```

```output
vet OK
```

Summary of the Issue #4 contract: zero-candidate failures (missing, unreadable, empty, or candidate-free .wrangler/state/v3/d1/miniflare-D1DatabaseObject) emit exactly two stderr lines — 'no candidate database found in .wrangler' plus 'Expected <path>; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.' — multiple candidates emit only 'There is more than one SQLite database in .wrangler' with no hint; every discovery failure exits 1 silently on stdout, bypasses internal/connection, and creates no database. References: Issue #4 (Notes/issues/004-exact-d1-discovery-diagnostics.md) and Notes/PRD-sqloid.md §D1 discovery.
