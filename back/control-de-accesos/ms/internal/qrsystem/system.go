package qrsystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync/atomic"
	"time"
)

// Client is the integration with whatever issues/reads QR codes at the gate.
// Implementations must honour ctx: a call that doesn't answer is cut by the
// breaker's timeout.
type Client interface {
	// Issue asks the QR system for a new code.
	Issue(ctx context.Context) (string, error)
	// Scan checks that the QR reader at the gate is reachable.
	Scan(ctx context.Context) error
}

// Local issues codes in-process (no external dependency). It is what runs
// outside development, and the "healthy" behaviour of the Simulator.
type Local struct{}

func (Local) Issue(context.Context) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (Local) Scan(context.Context) error { return nil }

// Mode of the Simulator.
const (
	ModeUp   = "UP"
	ModeDown = "DOWN" // the dependency hangs: it never answers
)

// Simulator stands in for a physical reader / external QR service during
// development. In DOWN mode it behaves like an unresponsive device: it blocks
// until the caller's context ends, which is exactly what the timeout and the
// circuit breaker exist to contain.
type Simulator struct {
	inner Local
	down  atomic.Bool
}

func (s *Simulator) SetMode(mode string) { s.down.Store(mode == ModeDown) }

func (s *Simulator) Mode() string {
	if s.down.Load() {
		return ModeDown
	}
	return ModeUp
}

func (s *Simulator) wait(ctx context.Context) error {
	if !s.down.Load() {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *Simulator) Issue(ctx context.Context) (string, error) {
	if err := s.wait(ctx); err != nil {
		return "", err
	}
	return s.inner.Issue(ctx)
}

func (s *Simulator) Scan(ctx context.Context) error {
	if err := s.wait(ctx); err != nil {
		return err
	}
	return s.inner.Scan(ctx)
}

// System is the QR integration behind its circuit breaker.
type System struct {
	client  Client
	breaker *Breaker
}

func New(client Client, cfg Config, logger *slog.Logger) *System {
	onChange := func(from, to State) {
		if logger != nil {
			logger.Warn("qr system circuit breaker", "from", from.String(), "to", to.String())
		}
	}
	return &System{client: client, breaker: NewBreaker(cfg, onChange)}
}

// Issue returns a new QR code, or an *UnavailableError without waiting when the
// circuit is open.
func (s *System) Issue(ctx context.Context) (string, error) {
	var code string
	err := s.breaker.Do(ctx, func(c context.Context) error {
		var err error
		code, err = s.client.Issue(c)
		return err
	})
	return code, err
}

// Scan verifies the QR reader is reachable before a QR is validated.
func (s *System) Scan(ctx context.Context) error {
	return s.breaker.Do(ctx, s.client.Scan)
}

// Public is what any authenticated user may learn about the system.
type Public struct {
	Available         bool   `json:"available"`
	State             string `json:"state"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
	Fallback          string `json:"fallback,omitempty"`
}

// Availability reports whether QR operations are expected to work. When they
// are not, Fallback names the method to use instead.
func (s *System) Availability() Public {
	st := s.breaker.Status()
	p := Public{Available: st.State != Open, State: st.State.String()}
	if st.State == Open {
		p.RetryAfterSeconds = int((st.RetryAfter + time.Second - 1) / time.Second)
		p.Fallback = "PRE_AUTH"
	}
	return p
}

// Status is the detailed view for the development tools.
type Detail struct {
	Mode              string `json:"mode,omitempty"`
	State             string `json:"state"`
	Failures          int    `json:"failures"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	Simulated         bool   `json:"simulated"`
}

func (s *System) Detail() Detail {
	st := s.breaker.Status()
	d := Detail{State: st.State.String(), Failures: st.Failures, RetryAfterSeconds: int((st.RetryAfter + time.Second - 1) / time.Second)}
	if sim, ok := s.client.(*Simulator); ok {
		d.Mode, d.Simulated = sim.Mode(), true
	}
	return d
}

// Simulator returns the simulated client, if this system uses one.
func (s *System) Simulator() (*Simulator, bool) {
	sim, ok := s.client.(*Simulator)
	return sim, ok
}

// ResetBreaker closes the circuit right away.
func (s *System) ResetBreaker() { s.breaker.Reset() }
