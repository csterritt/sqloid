// Externally cancellable started-request handles for SELECT page and count
// work (Issue #28), per the Identities and state plus Errors and cancellation
// bounds sections of Notes/PRD-sqloid.md. Each started request wraps Issue
// #6's Request lifecycle on its own dedicated leased connection, so a caller
// such as internal/ui can request connection-scoped cancellation from another
// goroutine while the work runs, and every result still flows through the
// Issue #26 health and cancellation-wins classification. Cancellation never
// force-closes a connection: settlement releases the lease, and the physical
// connection stays healthy and reusable for subsequent work.

package connection

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chris/sqloid/internal/result"
)

// StartedRequest is one running database request on a dedicated leased
// connection whose cancellation is requested from outside the work
// goroutine. The lifecycle is exactly: start (one of the Start methods) →
// Cancel at most effectively once, from any goroutine → the work settles and
// the lease is released automatically → Wait returns the classified result.
// Cancel is idempotent, never settles the request, and never force-closes
// the connection; Wait blocks until the in-flight work truly settles, so
// callers can hold visible cancelling state until every targeted request
// settles and defer any replacement dispatch until then.
type StartedRequest struct {
	req     *Request
	done    chan RequestResult
	settled chan struct{}
}

// startRequest leases a dedicated connection, begins a Request on it, and
// runs op in a separate goroutine. When op returns, the request settles with
// its outcome (Issue #6 classification, with post-error health verification
// taking precedence exactly as RunRequest does), releases the lease via
// Close, and delivers the result. A lease-acquisition failure pre-settles
// the request with the failed classification so callers always observe
// exactly one settlement through Wait. The op runs on the request's own
// derived context, so Cancel reaches it as a connection-scoped interrupt.
func (db *DB) startRequest(parent context.Context, op func(ctx context.Context, conn *sql.Conn) error) *StartedRequest {
	s := &StartedRequest{
		done:    make(chan RequestResult, 1),
		settled: make(chan struct{}),
	}

	lease, err := db.Lease(parent)
	if err != nil {
		var he *HealthError
		if errors.As(err, &he) {
			s.done <- RequestResult{Outcome: OutcomeFailed, Health: he}
		} else {
			s.done <- RequestResult{Outcome: OutcomeFailed, Err: err}
		}
		close(s.settled)
		return s
	}

	request := lease.BeginRequest(parent)
	s.req = request
	go func() {
		opErr := op(request.Context(), lease.Conn())
		outcome := request.Settle(opErr)
		closeErr := request.Close()
		var res RequestResult
		switch outcome {
		case OutcomeSuccess:
			res = RequestResult{Outcome: OutcomeSuccess, Err: closeErr}
		case OutcomeCancelled:
			res = RequestResult{Outcome: OutcomeCancelled, Err: closeErr}
		default:
			cause := firstErr(opErr, closeErr)
			var he *HealthError
			if errors.As(db.VerifyHealth(), &he) {
				res = RequestResult{Outcome: OutcomeFailed, Err: cause, Health: he}
			} else {
				res = RequestResult{Outcome: OutcomeFailed, Err: cause}
			}
		}
		s.done <- res
		close(s.settled)
	}()
	return s
}

// Cancel requests cancellation of the started request. It is idempotent and
// safe from any goroutine: only the first call dispatches the connection-
// scoped interrupt, and no call settles the request or closes the
// connection.
func (s *StartedRequest) Cancel() {
	if s.req != nil {
		s.req.Cancel()
	}
}

// State reports the visible lifecycle state of the underlying request: a
// lease-acquisition failure pre-settles, so the state is Settled there;
// otherwise Cancelling stays observable from Cancel until true settlement.
func (s *StartedRequest) State() RequestState {
	if s.req == nil {
		return StateSettled
	}
	return s.req.State()
}

// Settled reports whether the started request reached terminal
// classification and released its lease.
func (s *StartedRequest) Settled() bool {
	if s.req == nil {
		return true
	}
	return s.req.Settled()
}

// SettledChan exposes settlement as a receive-only channel so callers can
// wait for all-request settlement in selects without timing sleeps.
func (s *StartedRequest) SettledChan() <-chan struct{} { return s.settled }

// StartedPageRequest is one running first-page or later-page SELECT on its
// own dedicated connection with externally requested cancellation.
type StartedPageRequest struct {
	started *StartedRequest
	page    *result.Page // written by the work goroutine before settlement
}

// StartFirstPage runs one first-page SELECT — the statement and parameters
// must come from QueryBuilder's rendering seam — as an externally
// cancellable request on a dedicated leased connection.
func (db *DB) StartFirstPage(parent context.Context, statement string, params []any) *StartedPageRequest {
	s := &StartedPageRequest{}
	s.started = db.startRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeFirstPage != nil {
			db.beforeFirstPage(ctx, conn) // test-only barrier seam (see DB doc)
		}
		p, err := runFirstPage(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		s.page = p
		return nil
	})
	return s
}

// StartPage runs one later-page SELECT exactly like StartFirstPage: one
// complete page statement from QueryBuilder's page API on a dedicated
// cancellable leased connection.
func (db *DB) StartPage(parent context.Context, statement string, params []any) *StartedPageRequest {
	s := &StartedPageRequest{}
	s.started = db.startRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		p, err := runFirstPage(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		s.page = p
		return nil
	})
	return s
}

// Cancel requests cancellation of the page request; idempotent and
// connection-scoped, never force-closing the connection.
func (s *StartedPageRequest) Cancel() { s.started.Cancel() }

// State reports the page request's visible lifecycle state.
func (s *StartedPageRequest) State() RequestState { return s.started.State() }

// SettledChan exposes settlement as a receive-only channel.
func (s *StartedPageRequest) SettledChan() <-chan struct{} { return s.started.SettledChan() }

// Wait blocks until the page request truly settles — cancellation is only a
// request, so the returned result is the terminal classification — then
// returns the typed page (non-nil exactly on success) and the classified
// result whose Err preserves any cause and whose Health carries deletion or
// replacement classification.
func (s *StartedPageRequest) Wait() (*result.Page, RequestResult) {
	res := <-s.started.done
	return s.page, res
}

// StartedCountRequest is one running complete-SELECT result count on its own
// dedicated connection with externally requested cancellation.
type StartedCountRequest struct {
	started *StartedRequest
	total   int64 // written by the work goroutine before settlement
}

// StartCount runs one complete-SELECT result count — the statement and
// parameters must come from QueryBuilder's rendering seam — as an externally
// cancellable request on a dedicated leased connection.
func (db *DB) StartCount(parent context.Context, statement string, params []any) *StartedCountRequest {
	s := &StartedCountRequest{}
	s.started = db.startRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeCount != nil {
			db.beforeCount(ctx, conn) // test-only barrier seam (see DB doc)
		}
		n, err := runCount(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		s.total = n
		return nil
	})
	return s
}

// Cancel requests cancellation of the count request; idempotent and
// connection-scoped, never force-closing the connection.
func (s *StartedCountRequest) Cancel() { s.started.Cancel() }

// State reports the count request's visible lifecycle state.
func (s *StartedCountRequest) State() RequestState { return s.started.State() }

// SettledChan exposes settlement as a receive-only channel.
func (s *StartedCountRequest) SettledChan() <-chan struct{} { return s.started.SettledChan() }

// Wait blocks until the count request truly settles, then returns the
// counted total (meaningful exactly on success) and the classified result.
func (s *StartedCountRequest) Wait() (int64, RequestResult) {
	res := <-s.started.done
	return s.total, res
}
