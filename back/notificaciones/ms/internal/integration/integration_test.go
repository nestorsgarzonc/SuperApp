//go:build integration

package integration

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"superapp/notificaciones/internal/features/devices"
	"superapp/notificaciones/internal/features/notifications"
	"superapp/notificaciones/internal/features/preferences"
	"superapp/notificaciones/internal/platform/apperr"
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
	_, err = db.Exec(`TRUNCATE notifications, processed_events, notification_preferences, user_devices`)
	require.NoError(t, err, "is the database migrated? run `make migrate`")
	return db
}

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

	inbox, err := repo.GetUserNotifications(ctx, complexA, user, false, nil, 50)
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

	inbox, err := repo.GetUserNotifications(ctx, complexID, user, false, nil, 50)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	assert.Equal(t, notifications.DeliveryStatusPushSent, inbox[0].DeliveryStatus)
	assert.NotNil(t, inbox[0].DeliveredAt)
}

func TestPreferencesRepo_SaveAndMutedUsers(t *testing.T) {
	repo := preferences.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	muted, enabled, untouched := uuid.New(), uuid.New(), uuid.New()

	require.NoError(t, repo.Save(ctx, muted, map[string]bool{"VOTES": false, "COMMUNITY": false}))
	require.NoError(t, repo.Save(ctx, enabled, map[string]bool{"VOTES": false}))
	require.NoError(t, repo.Save(ctx, enabled, map[string]bool{"VOTES": true}), "save updates")

	got, err := repo.MutedUsers(ctx, []uuid.UUID{muted, enabled, untouched}, "VOTES")
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]bool{muted: true}, got)

	prefs, err := repo.GetByUser(ctx, muted)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"VOTES": false, "COMMUNITY": false}, prefs)
}

func TestNotificationsRepo_InboxFiltersByType(t *testing.T) {
	repo := notifications.NewPostgresRepository(openDB(t))
	ctx := context.Background()
	complexID, user := uuid.New(), uuid.New()
	for _, typ := range []string{"VOTE", "ASSEMBLY", "VISITOR"} {
		_, err := repo.CreateNotifications(ctx, notifications.NewNotifications{
			EventID: uuid.NewString(), ComplexID: &complexID, UserIDs: []uuid.UUID{user}, Title: typ, Body: "b", Type: typ,
		})
		require.NoError(t, err)
	}

	inbox, err := repo.GetUserNotifications(ctx, complexID, user, false, []string{"ASSEMBLY", "VOTE"}, 50)
	require.NoError(t, err)
	titles := []string{}
	for _, n := range inbox {
		titles = append(titles, n.Title)
	}
	assert.ElementsMatch(t, []string{"VOTE", "ASSEMBLY"}, titles)
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

// Mismo flujo que POST /internal/notifications: un evento repetido se guarda una
// sola vez y uno inválido se rechaza con error de validación.
func TestDispatch_IdempotentAndValidated(t *testing.T) {
	db := openDB(t)
	svc := notifications.NewService(notifications.NewPostgresRepository(db), nil, nil,
		preferences.NewChecker(preferences.NewPostgresRepository(db)), logger)
	ctx := context.Background()
	complexID, user := uuid.New(), uuid.New()

	event := notifications.DispatchInput{
		EventID: uuid.NewString(), Source: "control-de-accesos", ComplexID: &complexID,
		UserIDs: []uuid.UUID{user}, Category: "VISITOR", Title: "Visitante", Body: "Juan en portería",
		Data: map[string]any{"deep_link": "/visitors"},
	}
	require.NoError(t, svc.Dispatch(ctx, event))
	require.NoError(t, svc.Dispatch(ctx, event))

	err := svc.Dispatch(ctx, notifications.DispatchInput{EventID: "bad", Category: "NOPE"})
	assert.True(t, apperr.IsValidation(err))

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1`, user).Scan(&count))
	assert.Equal(t, 1, count, "duplicate event persisted once")
}
