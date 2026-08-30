#!/bin/sh
# Canonical Sqloid release-capability suite gate (Issue #56 Task 1).
#
# This is the ONE command that selects all and only the integrated
# release-blocking capability tests, from internal/connection,
# internal/ui, and internal/history. Both the Linux and macOS CI jobs
# invoke this identical script from a clean checkout with the pinned
# module graph, so any modernc.org/sqlite dependency change (go.mod/go.sum)
# is gated by the same evidence on both platforms.
#
# Vetted pin (the only version accepted by this gate):
#   modernc.org/sqlite v1.57.0  (exact, direct, no replace directive)
#
# Semantics: any setup, test, timeout, or race failure fails this script
# with a non-zero exit status and blocks release. There are no skips,
# continue-on-error wrappers, retries, platform exclusions, or conditional
# relaxations. The race detector runs under cgo; production remains the
# pure-Go modernc.org/sqlite driver.
set -eu
cd "$(dirname "$0")/.."
exec env CGO_ENABLED=1 go test -race -count=1 -timeout 20m \
	./internal/connection ./internal/ui ./internal/history
