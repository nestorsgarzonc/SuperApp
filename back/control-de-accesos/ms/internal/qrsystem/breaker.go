// Package qrsystem wraps the integration that issues and reads QR codes (a
// physical reader / external service at the gate) with a circuit breaker.
//
// A dependency that stops answering must not keep requests blocked: every call
// has a timeout, and after enough consecutive failures the circuit OPENS so
// callers get an immediate, controlled error ("use pre-authorization instead")
// instead of waiting. After a cool-down one probe is let through (HALF_OPEN);
// if it succeeds the circuit CLOSES again.
package qrsystem

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type State int

const (
	Closed   State = iota // healthy: calls go through
	Open                  // failing: calls are rejected immediately
	HalfOpen              // cooling down: a single probe call is allowed
)

func (s State) String() string {
	switch s {
	case Open:
		return "OPEN"
	case HalfOpen:
		return "HALF_OPEN"
	default:
		return "CLOSED"
	}
}

// Causes reported in UnavailableError.
const (
	CauseCircuitOpen = "CIRCUIT_OPEN" // rejected without calling the dependency
	CauseTimeout     = "TIMEOUT"      // the dependency didn't answer in time
	CauseError       = "ERROR"        // the dependency answered with an error
	CauseOverloaded  = "OVERLOADED"   // too many calls already in flight (bulkhead)
)

// ErrUnavailable matches (errors.Is) every UnavailableError.
var ErrUnavailable = errors.New("qr system unavailable")

// UnavailableError is the controlled failure callers see instead of hanging.
type UnavailableError struct {
	Cause      string
	RetryAfter time.Duration
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("qr system unavailable (%s)", e.Cause)
}
func (e *UnavailableError) Is(target error) bool { return target == ErrUnavailable }

type Config struct {
	// Failures is how many consecutive failures open the circuit.
	Failures int
	// OpenFor is how long the circuit stays open before allowing a probe.
	OpenFor time.Duration
	// CallTimeout bounds every call to the dependency.
	CallTimeout time.Duration
	// MaxConcurrent bounds calls in flight to the dependency. Without it, a burst
	// arriving before the circuit opens would pile up one goroutine per request,
	// each waiting for the full timeout.
	MaxConcurrent int
}

func (c Config) withDefaults() Config {
	if c.Failures < 1 {
		c.Failures = 3
	}
	if c.OpenFor <= 0 {
		c.OpenFor = 10 * time.Second
	}
	if c.CallTimeout <= 0 {
		c.CallTimeout = 800 * time.Millisecond
	}
	if c.MaxConcurrent < 1 {
		c.MaxConcurrent = 32
	}
	return c
}

type Status struct {
	State      State
	Failures   int
	RetryAfter time.Duration // time left before a probe is allowed (OPEN only)
}

type Breaker struct {
	cfg      Config
	now      func() time.Time
	onChange func(from, to State)

	mu       sync.Mutex
	state    State
	failures int
	openedAt time.Time
	probing  bool // a HALF_OPEN probe is in flight
	inflight int  // calls currently running against the dependency
}

func NewBreaker(cfg Config, onChange func(from, to State)) *Breaker {
	if onChange == nil {
		onChange = func(State, State) {}
	}
	return &Breaker{cfg: cfg.withDefaults(), now: time.Now, onChange: onChange}
}

type transition struct{ from, to State }

// setState records a transition. Callbacks run AFTER the lock is released
// (see fire): a slow or re-entrant callback must never block the breaker.
func (b *Breaker) setState(to State, ts *[]transition) {
	if b.state == to {
		return
	}
	*ts = append(*ts, transition{b.state, to})
	b.state = to
}

func (b *Breaker) fire(ts []transition) {
	for _, t := range ts {
		b.onChange(t.from, t.to)
	}
}

func (b *Breaker) retryAfterLocked() time.Duration {
	left := b.cfg.OpenFor - b.now().Sub(b.openedAt)
	if left < 0 {
		return 0
	}
	return left
}

// admit decides whether a call may proceed. probe is true when it is the
// single HALF_OPEN trial.
func (b *Breaker) admit() (probe bool, err error) {
	var ts []transition
	b.mu.Lock()
	defer func() { b.mu.Unlock(); b.fire(ts) }()
	switch b.state {
	case Open:
		if left := b.retryAfterLocked(); left > 0 {
			return false, &UnavailableError{Cause: CauseCircuitOpen, RetryAfter: left}
		}
		b.setState(HalfOpen, &ts)
		b.probing = true
		b.inflight++
		return true, nil
	case HalfOpen:
		if b.probing {
			return false, &UnavailableError{Cause: CauseCircuitOpen, RetryAfter: time.Second}
		}
		b.probing = true
		b.inflight++
		return true, nil
	}
	// CLOSED: bound the calls in flight. Being overloaded is not the
	// dependency's failure, so it is not counted against it.
	if b.inflight >= b.cfg.MaxConcurrent {
		return false, &UnavailableError{Cause: CauseOverloaded, RetryAfter: time.Second}
	}
	b.inflight++
	return false, nil
}

func (b *Breaker) record(probe, ok bool) {
	var ts []transition
	b.mu.Lock()
	defer func() { b.mu.Unlock(); b.fire(ts) }()
	if probe {
		b.probing = false
	}
	if ok {
		b.failures = 0
		b.setState(Closed, &ts)
		return
	}
	if b.state == HalfOpen {
		b.openedAt = b.now()
		b.setState(Open, &ts) // the probe failed: back to open for another cool-down
		return
	}
	b.failures++
	if b.failures >= b.cfg.Failures {
		b.openedAt = b.now()
		b.setState(Open, &ts)
	}
}

func (b *Breaker) release() {
	b.mu.Lock()
	b.inflight--
	b.mu.Unlock()
}

// Do runs fn under the breaker and the call timeout. A cancellation coming from
// the caller (e.g. the client hung up) is not the dependency's fault and is not
// counted as a failure.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	probe, err := b.admit()
	if err != nil {
		return err
	}
	defer b.release()

	callCtx, cancel := context.WithTimeout(ctx, b.cfg.CallTimeout)
	defer cancel()
	callErr := fn(callCtx)

	if callErr == nil {
		b.record(probe, true)
		return nil
	}
	if ctx.Err() != nil { // the caller gave up; don't blame the dependency
		if probe {
			b.mu.Lock()
			b.probing = false
			b.mu.Unlock()
		}
		return ctx.Err()
	}
	b.record(probe, false)
	cause := CauseError
	if errors.Is(callErr, context.DeadlineExceeded) {
		cause = CauseTimeout
	}
	return &UnavailableError{Cause: cause, RetryAfter: b.cfg.OpenFor}
}

func (b *Breaker) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := Status{State: b.state, Failures: b.failures}
	if b.state == Open {
		s.RetryAfter = b.retryAfterLocked()
		// Cool-down over: the next real call will be the probe. Report it as
		// HALF_OPEN so clients stop treating the system as down and try again;
		// otherwise nobody would ever send the call that closes the circuit.
		if s.RetryAfter == 0 {
			s.State = HalfOpen
		}
	}
	return s
}

// Reset closes the circuit immediately (operator action / tests).
func (b *Breaker) Reset() {
	var ts []transition
	b.mu.Lock()
	defer func() { b.mu.Unlock(); b.fire(ts) }()
	b.failures = 0
	b.probing = false
	b.setState(Closed, &ts)
}
