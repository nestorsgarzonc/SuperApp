package qrsystem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestBreaker(failures int, openFor time.Duration) (*Breaker, *clock, *[]string) {
	clk := &clock{t: time.Unix(1_700_000_000, 0)}
	var transitions []string
	b := NewBreaker(Config{Failures: failures, OpenFor: openFor, CallTimeout: 50 * time.Millisecond},
		func(from, to State) { transitions = append(transitions, from.String()+">"+to.String()) })
	b.now = clk.Now
	return b, clk, &transitions
}

var errBoom = errors.New("boom")

func fail(context.Context) error { return errBoom }
func ok(context.Context) error   { return nil }

func TestOpensAfterConsecutiveFailures(t *testing.T) {
	b, _, tr := newTestBreaker(3, 10*time.Second)
	for i := 0; i < 2; i++ {
		_ = b.Do(context.Background(), fail)
	}
	if b.Status().State != Closed {
		t.Fatal("should still be closed after 2 of 3 failures")
	}
	_ = b.Do(context.Background(), fail)
	if b.Status().State != Open {
		t.Fatal("should open on the 3rd consecutive failure")
	}
	if got := *tr; len(got) != 1 || got[0] != "CLOSED>OPEN" {
		t.Fatalf("unexpected transitions %v", got)
	}
}

func TestSuccessResetsTheFailureCount(t *testing.T) {
	b, _, _ := newTestBreaker(3, time.Second)
	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), ok)
	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)
	if b.Status().State != Closed {
		t.Fatal("failures must be consecutive to open the circuit")
	}
}

func TestOpenRejectsWithoutCallingTheDependency(t *testing.T) {
	b, _, _ := newTestBreaker(1, 10*time.Second)
	_ = b.Do(context.Background(), fail)

	var calls atomic.Int32
	err := b.Do(context.Background(), func(context.Context) error { calls.Add(1); return nil })

	var ue *UnavailableError
	if !errors.As(err, &ue) || ue.Cause != CauseCircuitOpen || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want CIRCUIT_OPEN, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("the dependency must not be called while the circuit is open")
	}
	if ue.RetryAfter <= 0 || ue.RetryAfter > 10*time.Second {
		t.Fatalf("RetryAfter should be within the cool-down, got %v", ue.RetryAfter)
	}
}

func TestHungCallIsCutByTheTimeoutAndCounts(t *testing.T) {
	b, _, _ := newTestBreaker(2, time.Second)
	hang := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

	start := time.Now()
	err := b.Do(context.Background(), hang)
	var ue *UnavailableError
	if !errors.As(err, &ue) || ue.Cause != CauseTimeout {
		t.Fatalf("want TIMEOUT, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("a hung dependency must not block past the call timeout")
	}
	_ = b.Do(context.Background(), hang)
	if b.Status().State != Open {
		t.Fatal("timeouts must open the circuit")
	}
}

func TestHalfOpenProbeSuccessCloses(t *testing.T) {
	b, clk, tr := newTestBreaker(1, 10*time.Second)
	_ = b.Do(context.Background(), fail)
	clk.Advance(11 * time.Second)

	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("probe after cool-down should go through, got %v", err)
	}
	if b.Status().State != Closed {
		t.Fatal("successful probe must close the circuit")
	}
	want := []string{"CLOSED>OPEN", "OPEN>HALF_OPEN", "HALF_OPEN>CLOSED"}
	if got := *tr; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("transitions %v, want %v", got, want)
	}
}

func TestHalfOpenProbeFailureReopens(t *testing.T) {
	b, clk, _ := newTestBreaker(1, 10*time.Second)
	_ = b.Do(context.Background(), fail)
	clk.Advance(11 * time.Second)

	_ = b.Do(context.Background(), fail) // the probe fails
	st := b.Status()
	if st.State != Open {
		t.Fatalf("failed probe must re-open, got %v", st.State)
	}
	if st.RetryAfter < 9*time.Second {
		t.Fatalf("a new cool-down must start, got %v", st.RetryAfter)
	}
}

func TestOnlyOneProbeAtATime(t *testing.T) {
	b, clk, _ := newTestBreaker(1, time.Second)
	_ = b.Do(context.Background(), fail)
	clk.Advance(2 * time.Second)

	release := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Do(context.Background(), func(context.Context) error { close(started); <-release; return nil })
	}()
	<-started // the probe is in flight

	var calls atomic.Int32
	err := b.Do(context.Background(), func(context.Context) error { calls.Add(1); return nil })
	if !errors.Is(err, ErrUnavailable) || calls.Load() != 0 {
		t.Fatalf("a second call during the probe must be rejected, got err=%v calls=%d", err, calls.Load())
	}
	close(release)
	wg.Wait()
	if b.Status().State != Closed {
		t.Fatal("probe success should close the circuit")
	}
}

func TestCallerCancellationIsNotADependencyFailure(t *testing.T) {
	b, _, _ := newTestBreaker(1, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := b.Do(ctx, func(c context.Context) error { <-c.Done(); return c.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if b.Status().State != Closed || b.Status().Failures != 0 {
		t.Fatal("a client hang-up must not count against the dependency")
	}
}

func TestOpenCircuitFailsFastUnderLoad(t *testing.T) {
	b, _, _ := newTestBreaker(1, time.Minute)
	_ = b.Do(context.Background(), fail)

	var calls atomic.Int32
	hang := func(ctx context.Context) error { calls.Add(1); <-ctx.Done(); return ctx.Err() }
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = b.Do(context.Background(), hang) }()
	}
	wg.Wait()
	if calls.Load() != 0 {
		t.Fatalf("%d calls reached a dependency behind an open circuit", calls.Load())
	}
	if time.Since(start) > 40*time.Millisecond {
		t.Fatalf("200 rejected calls took %v: they must not wait on the dependency", time.Since(start))
	}
}

func TestSimulatorDownHangsUntilTimeoutThenBreakerOpens(t *testing.T) {
	sim := &Simulator{}
	sys := New(sim, Config{Failures: 2, OpenFor: time.Minute, CallTimeout: 30 * time.Millisecond}, nil)

	if _, err := sys.Issue(context.Background()); err != nil {
		t.Fatalf("UP should issue a code, got %v", err)
	}
	sim.SetMode(ModeDown)
	for i := 0; i < 2; i++ {
		start := time.Now()
		if _, err := sys.Issue(context.Background()); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("DOWN should be unavailable, got %v", err)
		}
		if time.Since(start) > 500*time.Millisecond {
			t.Fatal("DOWN must be cut by the call timeout")
		}
	}
	if sys.Availability().Available || sys.Availability().Fallback != "PRE_AUTH" {
		t.Fatalf("circuit should be open with PRE_AUTH fallback: %+v", sys.Availability())
	}
	start := time.Now()
	_, err := sys.Issue(context.Background())
	if !errors.Is(err, ErrUnavailable) || time.Since(start) > 10*time.Millisecond {
		t.Fatalf("an open circuit must fail immediately, got %v after %v", err, time.Since(start))
	}
	if err := sys.Scan(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("Scan shares the same breaker")
	}
}

func TestStatusReportsHalfOpenOnceCoolDownIsOver(t *testing.T) {
	b, clk, _ := newTestBreaker(1, 10*time.Second)
	_ = b.Do(context.Background(), fail)
	if st := b.Status(); st.State != Open || st.RetryAfter <= 0 {
		t.Fatalf("want OPEN with time left, got %+v", st)
	}
	clk.Advance(11 * time.Second)
	if st := b.Status(); st.State != HalfOpen || st.RetryAfter != 0 {
		t.Fatalf("after the cool-down the status must invite a probe (HALF_OPEN), got %+v", st)
	}
	// ...and that probe is what closes it.
	if err := b.Do(context.Background(), ok); err != nil || b.Status().State != Closed {
		t.Fatalf("probe should close the circuit, err=%v state=%v", err, b.Status().State)
	}
}

func TestPublicAvailabilityRecoversAfterCoolDown(t *testing.T) {
	sim := &Simulator{}
	sys := New(sim, Config{Failures: 1, OpenFor: 40 * time.Millisecond, CallTimeout: 20 * time.Millisecond}, nil)
	sim.SetMode(ModeDown)
	_, _ = sys.Issue(context.Background())
	if sys.Availability().Available {
		t.Fatal("should be unavailable while open")
	}
	sim.SetMode(ModeUp)
	time.Sleep(60 * time.Millisecond)
	if a := sys.Availability(); !a.Available || a.State != "HALF_OPEN" {
		t.Fatalf("clients must be told to retry once the cool-down is over, got %+v", a)
	}
	if _, err := sys.Issue(context.Background()); err != nil {
		t.Fatalf("the probe should succeed, got %v", err)
	}
	if a := sys.Availability(); !a.Available || a.State != "CLOSED" {
		t.Fatalf("should be CLOSED after a successful probe, got %+v", a)
	}
}

// ── bulkhead y callbacks ─────────────────────────────────────────────────────

func TestBulkheadRejectsBeyondMaxConcurrentWithoutCountingAFailure(t *testing.T) {
	b := NewBreaker(Config{Failures: 1, OpenFor: time.Minute, CallTimeout: time.Second, MaxConcurrent: 2}, nil)
	release := make(chan struct{})
	var wg sync.WaitGroup
	started := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Do(context.Background(), func(context.Context) error { started <- struct{}{}; <-release; return nil })
		}()
	}
	<-started
	<-started // los 2 cupos están ocupados

	var calls atomic.Int32
	start := time.Now()
	err := b.Do(context.Background(), func(context.Context) error { calls.Add(1); return nil })
	var ue *UnavailableError
	if !errors.As(err, &ue) || ue.Cause != CauseOverloaded {
		t.Fatalf("want OVERLOADED, got %v", err)
	}
	if calls.Load() != 0 || time.Since(start) > 50*time.Millisecond {
		t.Fatal("an overloaded call must be rejected immediately without reaching the dependency")
	}
	if st := b.Status(); st.State != Closed || st.Failures != 0 {
		t.Fatalf("being overloaded is not the dependency's failure: %+v", st)
	}
	close(release)
	wg.Wait()
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("slots must be freed once the calls finish, got %v", err)
	}
}

func TestOnChangeRunsOutsideTheLock(t *testing.T) {
	var b *Breaker
	done := make(chan struct{})
	b = NewBreaker(Config{Failures: 1, OpenFor: time.Minute, CallTimeout: 50 * time.Millisecond}, func(from, to State) {
		_ = b.Status() // re-entrante: con el lock tomado esto se quedaría colgado para siempre
		close(done)
	})
	go func() { _ = b.Do(context.Background(), fail) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onChange deadlocked: callbacks must run after the lock is released")
	}
}

func TestInflightCounterNeverLeaks(t *testing.T) {
	b := NewBreaker(Config{Failures: 1000, OpenFor: time.Minute, CallTimeout: 20 * time.Millisecond, MaxConcurrent: 3}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				_ = b.Do(context.Background(), ok)
			case 1:
				_ = b.Do(context.Background(), fail)
			default:
				_ = b.Do(context.Background(), func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
			}
		}(i)
	}
	wg.Wait()
	b.mu.Lock()
	left := b.inflight
	b.mu.Unlock()
	if left != 0 {
		t.Fatalf("in-flight counter leaked: %d", left)
	}
}
