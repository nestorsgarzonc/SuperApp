//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"superapp/notificaciones/internal/events"
	"superapp/notificaciones/internal/features/devices"
	"superapp/notificaciones/internal/features/notifications"
	"superapp/notificaciones/internal/features/preferences"
)

var logger = slog.New(slog.DiscardHandler)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`TRUNCATE notifications, processed_events, notification_preferences, notification_settings, user_devices`)
	require.NoError(t, err, "is the database migrated? run `make migrate`")
	return db
}

func openRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func ptr[T any](v T) *T { return &v }

func TestNotificationsRepo_BulkInsertAndIdempotency(t *testing.T) {
	db := openDB(t)
	repo := notifications.NewPostgresRepository(db)
	ctx := context.Background()
	complexID := uuid.New()
	users := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	in := notifications.NewNotifications{
		ComplexID: &complexID, UserIDs: users, Title: "t", Body: "b",
		Data: notifications.JSONB{"deep_link": "/x"}, Type: "VISITOR",
		EventID: "evt-1", Source: "test",
	}
	created, err := repo.CreateNotifications(ctx, in)
	require.NoError(t, err)
	require.Len(t, created, 3)
	got := []uuid.UUID{}
	for _, n := range created {
		got = append(got, n.UserID)
		assert.Equal(t, notifications.DeliveryStatusInboxOnly, n.DeliveryStatus)
	}
	assert.ElementsMatch(t, users, got)

	_, err = repo.CreateNotifications(ctx, in)
	assert.ErrorIs(t, err, notifications.ErrDuplicateEvent)

	var rows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE complex_id = $1`, complexID).Scan(&rows))
	assert.Equal(t, 3, rows, "the duplicate event must not insert rows")
}

func TestNotificationsRepo_InboxIncludesAccountLevelRows(t *testing.T) {
	repo := notifications.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	user := uuid.New()
	complexA, complexB := uuid.New(), uuid.New()

	for _, in := range []notifications.NewNotifications{
		{EventID: uuid.NewString(), ComplexID: &complexA, UserIDs: []uuid.UUID{user}, Title: "A", Body: "b", Type: "GENERAL"},
		{EventID: uuid.NewString(), ComplexID: &complexB, UserIDs: []uuid.UUID{user}, Title: "B", Body: "b", Type: "GENERAL"},
		{EventID: uuid.NewString(), ComplexID: nil, UserIDs: []uuid.UUID{user}, Title: "Account", Body: "b", Type: "ACCOUNT"},
	} {
		_, err := repo.CreateNotifications(ctx, in)
		require.NoError(t, err)
	}

	inbox, err := repo.GetUserNotifications(ctx, complexA, user, false, 50)
	require.NoError(t, err)
	titles := []string{}
	for _, n := range inbox {
		titles = append(titles, n.Title)
	}
	assert.ElementsMatch(t, []string{"A", "Account"}, titles, "other complexes excluded, account rows included")

	count, err := repo.CountUnread(ctx, complexA, user)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	marked, err := repo.MarkAllAsRead(ctx, complexA, user)
	require.NoError(t, err)
	assert.EqualValues(t, 2, marked)

	count, err = repo.CountUnread(ctx, complexB, user)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "complex B still has its own unread row; the account row was read")
}

func TestNotificationsRepo_MarkAsReadIsScopedToOwner(t *testing.T) {
	repo := notifications.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	complexID, owner := uuid.New(), uuid.New()
	created, err := repo.CreateNotifications(ctx, notifications.NewNotifications{
		EventID: uuid.NewString(), ComplexID: &complexID, UserIDs: []uuid.UUID{owner}, Title: "t", Body: "b", Type: "GENERAL",
	})
	require.NoError(t, err)
	id := created[0].ID

	assert.ErrorIs(t, repo.MarkAsRead(ctx, id, complexID, uuid.New()), notifications.ErrNotFound, "another user")
	assert.ErrorIs(t, repo.MarkAsRead(ctx, id, uuid.New(), owner), notifications.ErrNotFound, "another complex")
	require.NoError(t, repo.MarkAsRead(ctx, id, complexID, owner))
}

func TestNotificationsRepo_UpdateDelivery(t *testing.T) {
	repo := notifications.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	complexID, user := uuid.New(), uuid.New()

	created, err := repo.CreateNotifications(ctx, notifications.NewNotifications{
		EventID: uuid.NewString(), ComplexID: &complexID, UserIDs: []uuid.UUID{user}, Title: "t", Body: "b", Type: "VISITOR",
	})
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, repo.UpdateDelivery(ctx, created[0].ID, notifications.DeliveryStatusPushSent, &now))
	assert.ErrorIs(t, repo.UpdateDelivery(ctx, uuid.New(), notifications.DeliveryStatusPushSent, &now), notifications.ErrNotFound)

	inbox, err := repo.GetUserNotifications(ctx, complexID, user, false, 50)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	assert.Equal(t, notifications.DeliveryStatusPushSent, inbox[0].DeliveryStatus)
	assert.NotNil(t, inbox[0].DeliveredAt)
}

func TestPreferencesRepo_BatchLookupAndTimezones(t *testing.T) {
	repo := preferences.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	muted, quiet, untouched := uuid.New(), uuid.New(), uuid.New()

	require.NoError(t, repo.Upsert(ctx, &preferences.Preference{UserID: muted, Category: "GENERAL", Enabled: false}))
	require.NoError(t, repo.Upsert(ctx, &preferences.Preference{UserID: quiet, Category: "GENERAL", Enabled: true, QuietStart: ptr(1320), QuietEnd: ptr(360)}))
	require.NoError(t, repo.Upsert(ctx, &preferences.Preference{UserID: muted, Category: "GENERAL", Enabled: true}), "upsert updates")
	require.NoError(t, repo.UpsertTimezone(ctx, quiet, "Europe/Madrid"))

	prefs, err := repo.GetByUsersCategory(ctx, []uuid.UUID{muted, quiet, untouched}, "GENERAL")
	require.NoError(t, err)
	require.Len(t, prefs, 2)
	assert.True(t, prefs[muted].Enabled)
	assert.Equal(t, 1320, *prefs[quiet].QuietStart)

	tzs, err := repo.GetTimezones(ctx, []uuid.UUID{muted, quiet})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]string{quiet: "Europe/Madrid"}, tzs)
}

func TestDevicesRepo_TokenLifecycle(t *testing.T) {
	repo := devices.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	alice, bob := uuid.New(), uuid.New()

	_, err := repo.Upsert(ctx, &devices.Device{UserID: alice, Platform: "IOS", PushToken: "shared-phone"})
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, &devices.Device{UserID: alice, Platform: "ANDROID", PushToken: "alice-tablet"})
	require.NoError(t, err)

	_, err = repo.Upsert(ctx, &devices.Device{UserID: bob, Platform: "IOS", PushToken: "shared-phone"})
	require.NoError(t, err)

	tokens, err := repo.TokensByUserIDs(ctx, []uuid.UUID{alice, bob})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice-tablet"}, tokens[alice])
	assert.Equal(t, []string{"shared-phone"}, tokens[bob])

	assert.ErrorIs(t, repo.DeleteByToken(ctx, "shared-phone", alice), devices.ErrNotFound, "only the owner can unregister")
	require.NoError(t, repo.DeleteByToken(ctx, "shared-phone", bob))

	require.NoError(t, repo.DeleteTokens(ctx, []string{"alice-tablet"}))
	tokens, err = repo.TokensByUserIDs(ctx, []uuid.UUID{alice, bob})
	require.NoError(t, err)
	assert.Empty(t, tokens)
}

type consumerEnv struct {
	rdb    *redis.Client
	stream string
	cancel context.CancelFunc
	done   chan struct{}
}

func startConsumer(t *testing.T, rdb *redis.Client, svc events.Dispatcher) *consumerEnv {
	t.Helper()
	stream := "test.notifications." + uuid.NewString()
	t.Cleanup(func() { rdb.Del(context.Background(), stream, stream+".dlq") })

	c := events.NewConsumer(rdb, stream, "notificaciones", "test-consumer", svc, logger)
	c.ClaimIdle = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	env := &consumerEnv{rdb: rdb, stream: stream, cancel: cancel, done: make(chan struct{})}
	go func() { defer close(env.done); _ = c.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-env.done })

	require.Eventually(t, func() bool {
		groups, err := rdb.XInfoGroups(context.Background(), stream).Result()
		return err == nil && len(groups) == 1
	}, 5*time.Second, 20*time.Millisecond)
	return env
}

func (e *consumerEnv) publish(t *testing.T, payload any) {
	t.Helper()
	raw, ok := payload.(string)
	if !ok {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		raw = string(b)
	}
	require.NoError(t, e.rdb.XAdd(context.Background(), &redis.XAddArgs{
		Stream: e.stream, Values: map[string]any{"payload": raw},
	}).Err())
}

func (e *consumerEnv) dlqLen() int64 {
	n, _ := e.rdb.XLen(context.Background(), e.stream+".dlq").Result()
	return n
}

func (e *consumerEnv) pending() int64 {
	p, err := e.rdb.XPending(context.Background(), e.stream, "notificaciones").Result()
	if err != nil {
		return -1
	}
	return p.Count
}

func TestConsumer_ProcessesEventsOnceAndDeadLettersInvalid(t *testing.T) {
	db := openDB(t)
	svc := notifications.NewService(notifications.NewPostgresRepository(db), nil, nil,
		preferences.NewChecker(preferences.NewPostgresRepository(db), time.UTC), logger)
	env := startConsumer(t, openRedis(t), svc)
	complexID, user := uuid.New(), uuid.New()

	event := events.NotificationRequested{
		EventID: uuid.NewString(), Source: "control-de-accesos", ComplexID: &complexID,
		UserIDs: []uuid.UUID{user}, Category: "VISITOR", Title: "Visitante", Body: "Juan en portería",
		Data: map[string]any{"deep_link": "/visitors"},
	}
	env.publish(t, event)
	env.publish(t, event)
	env.publish(t, `{"event_id":"bad","category":"NOPE","user_ids":[]}`)
	env.publish(t, `not json`)

	require.Eventually(t, func() bool { return env.dlqLen() == 2 && env.pending() == 0 }, 10*time.Second, 50*time.Millisecond)

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1`, user).Scan(&count))
	assert.Equal(t, 1, count, "duplicate event persisted once")
}

type failingDispatcher struct{ calls chan struct{} }

func (f failingDispatcher) Dispatch(context.Context, notifications.DispatchInput) error {
	f.calls <- struct{}{}
	return errors.New("connection refused")
}

func TestConsumer_RetriesTransientFailuresThenDeadLetters(t *testing.T) {
	calls := make(chan struct{}, 100)
	env := startConsumer(t, openRedis(t), failingDispatcher{calls: calls})

	env.publish(t, events.NotificationRequested{EventID: "evt-transient", UserIDs: []uuid.UUID{uuid.New()}, Category: "GENERAL", Title: "t", Body: "b"})

	require.Eventually(t, func() bool { return env.dlqLen() == 1 && env.pending() == 0 }, 15*time.Second, 50*time.Millisecond)
	assert.Equal(t, 5, len(calls), "first delivery + retries up to maxDeliveries, then DLQ")

	msgs, err := env.rdb.XRange(context.Background(), env.stream+".dlq", "-", "+").Result()
	require.NoError(t, err)
	assert.Contains(t, msgs[0].Values["error"], "gave up after")
}
