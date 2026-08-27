// Request lifecycle: driver-independent cancellable request infrastructure
// shared by every Sqloid database request, per Issue #6 and the Identities
// and state plus Errors and cancellation bounds sections of
// Notes/PRD-sqloid.md.

package connection

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// nextRequestID assigns each Request its process-unique identity. IDs are
// never reused, so a stale response can never be mistaken for the response
// of a later request with the same identity.
var nextRequestID atomic.Uint64

// Outcome is the terminal classification of a settled Request. Exactly one
// outcome is produced per request, at settlement, and it never changes
// afterwards.
type Outcome int

const (
	// OutcomeSuccess means the work completed before any cancellation was
	// requested and its result may be adopted by the caller.
	OutcomeSuccess Outcome = iota + 1
	// OutcomeCancelled means cancellation was requested for the request:
	// either before the work settled, in which case a success result is
	// discarded and reclassified as cancelled, or because the work itself
	// failed with a cancellation error. Failure results are never upgraded
	// to success by cancellation.
	OutcomeCancelled
	// OutcomeFailed means the work failed for a reason other than
	// cancellation, such as a database or driver error.
	OutcomeFailed
)

// String renders the human-facing name of the outcome used in tests and
// diagnostics.
func (o Outcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeFailed:
		return "failed"
	default:
		return "Outcome(" + itoa(int(o)) + ")"
	}
}

// RequestState is the visible lifecycle state of a Request. The state is
// cancellable until the in-flight operation actually settles: Cancelling
// remains observable from the moment Cancel is invoked until Settle marks
// the request settled, regardless of how quickly or slowly the underlying
// work ends.
type RequestState int

const (
	// StateRunning means the request has been started and no cancellation
	// has been requested yet.
	StateRunning RequestState = iota + 1
	// StateCancelling means cancellation was requested and the request has
	// not settled yet. This is the state a UI renders as `cancelling…`.
	StateCancelling
	// StateSettled means the request reached its terminal classification;
	// its lease is releasable and no further work may run on it.
	StateSettled
)

// String renders the human-facing name of the state used in tests and
// diagnostics.
func (s RequestState) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateCancelling:
		return "cancelling"
	case StateSettled:
		return "settled"
	default:
		return "RequestState(" + itoa(int(s)) + ")"
	}
}

// Request is one cancellable database request running on a dedicated
// leased connection. It owns a derived cancellable context, a process-unique
// identity, and the connection-scoped interrupt dispatch for the lease.
//
// The lifecycle is exactly: BeginRequest → (Cancel at most once, from any
// goroutine) → one Settle → Close. Settle may be called at most once with
// the final error of the in-flight work; Close releases the lease and is
// idempotent. A Request must not be copied after first use.
//
// Ownership: the caller owns the lease until it calls Close; between
// BeginRequest and Close no other request can acquire that lease, so no
// replacement work can run on the same physical connection before
// settlement. Cancellation never force-closes the connection: it only
// requests an interrupt; the physical connection remains valid and reusable
// for subsequent work after settlement.
type Request struct {
	id     uint64
	lease  *Lease
	ctx    context.Context
	cancel context.CancelFunc

	// interrupt dispatches the connection-scoped interrupt request for the
	// leased physical connection. It is invoked at most once, by the first
	// Cancel call. Leases begin without a hook: with the pinned modernc
	// driver, cancelling the request context is itself the connection-
	// scoped interrupt (the driver maps context cancellation to
	// sqlite3_interrupt on exactly the leased connection). Tests install a
	// hook to observe or fake the dispatch.
	interrupt func()

	mu        sync.Mutex
	cancelled bool
	settled   bool
	outcome   Outcome
	closeErr  error
	closed    bool
}

// BeginRequest starts a cancellable request that exclusively owns l until
// Close. The returned Request derives its context from parent; cancelling
// parent also cancels the request, and the first Cancel or Settle call
// cancels the derived context exactly once.
func (l *Lease) BeginRequest(parent context.Context) *Request {
	ctx, cancel := context.WithCancel(parent)
	return &Request{
		id:        nextRequestID.Add(1),
		lease:     l,
		ctx:       ctx,
		cancel:    cancel,
		interrupt: l.interruptFn,
	}
}

// ID returns the process-unique identity of this request. Identities are
// assigned in increasing order and never reused.
func (r *Request) ID() uint64 { return r.id }

// Context returns the request's dedicated cancellable context. Work must be
// executed through this context so that cancellation reaches the underlying
// driver as a connection-scoped interrupt; callers must not cancel it
// directly — use Cancel or Settle.
func (r *Request) Context() context.Context { return r.ctx }

// State reports the visible lifecycle state. Cancelling stays observable
// until Settle records settlement, even if the in-flight work has already
// finished but Settle has not been called yet.
func (r *Request) State() RequestState {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.settled:
		return StateSettled
	case r.cancelled:
		return StateCancelling
	default:
		return StateRunning
	}
}

// Settled reports whether the request reached its terminal classification.
func (r *Request) Settled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settled
}

// Cancel requests cancellation of the request. It is idempotent: only the
// first call has any effect, and it dispatches the connection-scoped
// interrupt exactly once. Cancel never settles the request and never closes
// the connection; visible state becomes Cancelling until Settle runs.
func (r *Request) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelled || r.settled {
		return
	}
	r.cancelled = true
	r.cancel()
	if r.interrupt != nil {
		r.interrupt()
	}
}

// Settle records the terminal result of the in-flight work and classifies
// it. Settle must be called exactly once, after the work goroutine finishes;
// it cancels the request context so no further work can start on it.
//
// Classification rules: success (err == nil) that arrives after Cancel was
// requested is discarded and classified as OutcomeCancelled; cancellation
// errors (context.Canceled) are classified as OutcomeCancelled; any other
// error is OutcomeFailed; uncancelled success is OutcomeSuccess. Settlement
// happens only once — a second call returns the stored outcome unchanged.
func (r *Request) Settle(err error) Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelled && !r.settled {
		r.cancel()
	}
	if r.settled {
		return r.outcome
	}
	r.settled = true
	switch {
	case r.cancelled && err == nil:
		r.outcome = OutcomeCancelled
	case errors.Is(err, context.Canceled):
		r.outcome = OutcomeCancelled
	case err == nil:
		r.outcome = OutcomeSuccess
	default:
		r.outcome = OutcomeFailed
	}
	return r.outcome
}

// Close releases the leased connection back to the pool. Close must be
// called after Settle (lease release occurs only after settlement) and is
// safe to call more than once; the first call's release error is retained
// and reported by every call.
func (r *Request) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.cancel()
	r.closeErr = r.lease.Release(context.Background())
	return r.closeErr
}

// Run executes op synchronously through the request's context and settles
// the request with its result, returning the outcome. It is the convenience
// form of BeginRequest → run → Settle for callers that do not need to cancel
// from a second goroutine while op runs.
func (r *Request) Run(op func(ctx context.Context) error) Outcome {
	return r.Settle(op(r.ctx))
}

// itoa renders small non-negative integers in tests and diagnostics without
// pulling fmt into this file's hot paths.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
