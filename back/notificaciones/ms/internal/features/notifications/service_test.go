package notifications

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"superapp/notificaciones/internal/platform/apperr"
	"superapp/notificaciones/internal/platform/push"
)

type fakeRepo struct {
	Repository

	mu         sync.Mutex
	created    []NewNotifications
	processed  map[string]bool
	deliveries map[uuid.UUID]string
	createErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{processed: map[string]bool{}, deliveries: map[uuid.UUID]string{}}
}

func (f *fakeRepo) CreateNotifications(_ context.Context, in NewNotifications) ([]*Notification, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.processed[in.EventID] {
		return nil, ErrDuplicateEvent
	}
	f.processed[in.EventID] = true
	f.created = append(f.created, in)
	out := make([]*Notification, 0, len(in.UserIDs))
	for _, id := range in.UserIDs {
		out = append(out, &Notification{ID: uuid.New(), UserID: id, Type: in.Type, DeliveryStatus: DeliveryStatusInboxOnly})
	}
	return out, nil
}

func (f *fakeRepo) UpdateDelivery(_ context.Context, id uuid.UUID, status string, _ *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries[id] = status
	return nil
}

type fakeTokens struct {
	mu      sync.Mutex
	byUser  map[uuid.UUID][]string
	deleted []string
}

func (f *fakeTokens) TokensByUserIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]string, error) {
	return f.byUser, nil
}

func (f *fakeTokens) DeleteTokens(_ context.Context, tokens []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, tokens...)
	return nil
}

type fakePusher struct {
	mu     sync.Mutex
	sent   []push.Message
	result func(push.Message) (push.Result, error)
}

func (f *fakePusher) Send(_ context.Context, msg push.Message) (push.Result, error) {
	f.mu.Lock()
	f.sent = append(f.sent, msg)
	f.mu.Unlock()
	if f.result != nil {
		return f.result(msg)
	}
	return push.Result{}, nil
}

type mutedChecker map[uuid.UUID]bool

func (m mutedChecker) FilterRecipients(_ context.Context, userIDs []uuid.UUID, _ string, _ time.Time) []uuid.UUID {
	out := []uuid.UUID{}
	for _, id := range userIDs {
		if !m[id] {
			out = append(out, id)
		}
	}
	return out
}

func newTestService(repo *fakeRepo, tokens TokenStore, pusher push.Sender, checker DeliveryChecker) Service {
	return NewService(repo, tokens, pusher, checker, slog.New(slog.DiscardHandler))
}

func validEvent() DispatchInput {
	complexID := uuid.New()
	return DispatchInput{
		EventID: uuid.NewString(), Source: "control-de-accesos", ComplexID: &complexID,
		UserIDs: []uuid.UUID{uuid.New()}, Category: "ACCESS",
		Title: "Visitante en portería", Body: "Juan Pérez está en la entrada",
	}
}

func eventFor(userIDs ...uuid.UUID) DispatchInput {
	ev := validEvent()
	ev.UserIDs = userIDs
	return ev
}

func TestDispatch_FiltersMutedRecipientsAndDedupes(t *testing.T) {
	a, b, muted := uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepo()
	svc := newTestService(repo, nil, nil, mutedChecker{muted: true})

	require.NoError(t, svc.Dispatch(context.Background(), eventFor(a, b, a, muted)))

	require.Len(t, repo.created, 1)
	assert.ElementsMatch(t, []uuid.UUID{a, b}, repo.created[0].UserIDs)
}

func TestDispatch_EveryoneMutedRecordsEventWithoutRows(t *testing.T) {
	u := uuid.New()
	repo := newFakeRepo()
	svc := newTestService(repo, nil, nil, mutedChecker{u: true})

	require.NoError(t, svc.Dispatch(context.Background(), eventFor(u)))

	require.Len(t, repo.created, 1)
	assert.Empty(t, repo.created[0].UserIDs)
}

func TestDispatch_SanitizesAndRequiresTitle(t *testing.T) {
	svc := newTestService(newFakeRepo(), nil, nil, nil)
	ev := validEvent()
	ev.Title = " \x00\x07 "

	assert.True(t, apperr.IsValidation(svc.Dispatch(context.Background(), ev)))
}

func TestDispatch_RecordsDeliveryStatusPerRecipient(t *testing.T) {
	ok, partial, failed, noDevice := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newFakeRepo()
	tokens := &fakeTokens{byUser: map[uuid.UUID][]string{
		ok:      {"ok-1"},
		partial: {"partial-good", "partial-stale"},
		failed:  {"failed-1"},
	}}
	pusher := &fakePusher{result: func(m push.Message) (push.Result, error) {
		switch m.Tokens[0] {
		case "partial-good":
			return push.Result{Failed: []string{"partial-stale"}, Unregistered: []string{"partial-stale"}}, nil
		case "failed-1":
			return push.Result{}, errors.New("fcm down")
		}
		return push.Result{}, nil
	}}
	svc := newTestService(repo, tokens, pusher, nil)

	require.NoError(t, svc.Dispatch(context.Background(), eventFor(ok, partial, failed, noDevice)))
	svc.Wait()

	assert.Len(t, pusher.sent, 3, "users without devices are not pushed")
	assert.Len(t, repo.deliveries, 3, "users without devices stay INBOX_ONLY")
	assert.ElementsMatch(t,
		[]string{DeliveryStatusPushSent, DeliveryStatusPushPartial, DeliveryStatusPushFailed},
		values(repo.deliveries))
	assert.Equal(t, []string{"partial-stale"}, tokens.deleted, "unregistered tokens are pruned")
}

func values(m map[uuid.UUID]string) []string { return slices.Collect(maps.Values(m)) }

func TestDispatch_IsIdempotentOnEventID(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, nil, nil, nil)
	ev := validEvent()

	require.NoError(t, svc.Dispatch(context.Background(), ev))
	require.NoError(t, svc.Dispatch(context.Background(), ev), "redelivery is not an error")

	assert.Len(t, repo.created, 1, "the same event must be persisted once")
}

func TestDispatch_RejectsUnknownCategoryAsPermanent(t *testing.T) {
	svc := newTestService(newFakeRepo(), nil, nil, nil)
	ev := validEvent()
	ev.Category = "NOPE"

	assert.True(t, apperr.IsValidation(svc.Dispatch(context.Background(), ev)))
}

func TestDispatch_RequiresEventID(t *testing.T) {
	svc := newTestService(newFakeRepo(), nil, nil, nil)
	ev := validEvent()
	ev.EventID = ""

	assert.True(t, apperr.IsValidation(svc.Dispatch(context.Background(), ev)))
}

func TestDispatch_AllowsAccountLevelWithoutComplex(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, nil, nil, nil)
	ev := validEvent()
	ev.ComplexID = nil
	ev.Category = "ACCOUNT"

	require.NoError(t, svc.Dispatch(context.Background(), ev))
	require.Len(t, repo.created, 1)
	assert.Nil(t, repo.created[0].ComplexID)
}

func TestDispatch_CriticalCategoryPushesHighPriority(t *testing.T) {
	u := uuid.New()
	pusher := &fakePusher{}
	svc := newTestService(newFakeRepo(), &fakeTokens{byUser: map[uuid.UUID][]string{u: {"t"}}}, pusher, nil)
	ev := validEvent()
	ev.UserIDs = []uuid.UUID{u}
	ev.Category = "SOS"

	require.NoError(t, svc.Dispatch(context.Background(), ev))
	svc.Wait()
	require.Len(t, pusher.sent, 1)
	assert.True(t, pusher.sent[0].Critical)
}

func TestDispatch_RepoFailureIsTransient(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = errors.New("connection refused")
	svc := newTestService(repo, nil, nil, nil)

	err := svc.Dispatch(context.Background(), validEvent())

	require.Error(t, err)
	assert.False(t, apperr.IsValidation(err), "DB errors must be retried, not dead-lettered")
}

func TestDispatch_PushSurvivesCallerCancellation(t *testing.T) {
	u := uuid.New()
	repo := newFakeRepo()
	release := make(chan struct{})
	pusher := &fakePusher{result: func(push.Message) (push.Result, error) {
		<-release
		return push.Result{}, nil
	}}
	svc := newTestService(repo, &fakeTokens{byUser: map[uuid.UUID][]string{u: {"t"}}}, pusher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, svc.Dispatch(ctx, eventFor(u)), "Dispatch returns once the inbox is written, before push completes")

	cancel()
	close(release)
	svc.Wait()

	assert.Equal(t, []string{DeliveryStatusPushSent}, values(repo.deliveries))
}

func TestDispatch_CriticalPushSkipsTheQueue(t *testing.T) {
	block := make(chan struct{})
	criticalSent := make(chan struct{})
	pusher := &fakePusher{result: func(m push.Message) (push.Result, error) {
		if m.Critical {
			close(criticalSent)
			return push.Result{}, nil
		}
		<-block
		return push.Result{}, nil
	}}

	broadcast := make([]uuid.UUID, pushConcurrency+5)
	tokens := map[uuid.UUID][]string{}
	for i := range broadcast {
		broadcast[i] = uuid.New()
		tokens[broadcast[i]] = []string{"t"}
	}
	sosUser := uuid.New()
	tokens[sosUser] = []string{"sos"}

	svc := newTestService(newFakeRepo(), &fakeTokens{byUser: tokens}, pusher, nil)
	defer func() { close(block); svc.Wait() }()

	require.NoError(t, svc.Dispatch(context.Background(), eventFor(broadcast...)))

	ev := validEvent()
	ev.UserIDs = []uuid.UUID{sosUser}
	ev.Category = "SOS"
	require.NoError(t, svc.Dispatch(context.Background(), ev))

	select {
	case <-criticalSent:
	case <-time.After(2 * time.Second):
		t.Fatal("critical push was blocked behind the non-critical broadcast")
	}
}
