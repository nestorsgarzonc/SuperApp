package preferences

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	rows      map[string]*Preference
	timezones map[uuid.UUID]string
	getErr    error
	upserts   []*Preference
	queries   int
}

func newMockRepo() *mockRepo {
	return &mockRepo{rows: map[string]*Preference{}, timezones: map[uuid.UUID]string{}}
}

func key(userID uuid.UUID, category string) string { return userID.String() + "|" + category }

func (m *mockRepo) GetByUser(_ context.Context, userID uuid.UUID) ([]*Preference, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	var out []*Preference
	for _, p := range m.rows {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockRepo) GetByUsersCategory(_ context.Context, userIDs []uuid.UUID, category string) (map[uuid.UUID]*Preference, error) {
	m.queries++
	if m.getErr != nil {
		return nil, m.getErr
	}
	out := map[uuid.UUID]*Preference{}
	for _, id := range userIDs {
		if p, ok := m.rows[key(id, category)]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (m *mockRepo) Upsert(_ context.Context, p *Preference) error {
	cp := *p
	m.rows[key(p.UserID, p.Category)] = &cp
	m.upserts = append(m.upserts, &cp)
	return nil
}

func (m *mockRepo) GetTimezones(_ context.Context, userIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	m.queries++
	out := map[uuid.UUID]string{}
	for _, id := range userIDs {
		if tz, ok := m.timezones[id]; ok {
			out[id] = tz
		}
	}
	return out, nil
}

func (m *mockRepo) UpsertTimezone(_ context.Context, userID uuid.UUID, tz string) error {
	m.timezones[userID] = tz
	return nil
}

func ptr[T any](v T) *T { return &v }

func bogota(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("America/Bogota")
	require.NoError(t, err)
	return loc
}

func delivers(chk *Checker, userID uuid.UUID, category string, now time.Time) bool {
	return len(chk.FilterRecipients(context.Background(), []uuid.UUID{userID}, category, now)) == 1
}

func TestFilter_CriticalAlwaysDelivers(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	chk := NewChecker(repo, time.UTC)
	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)

	for _, cat := range []string{CategorySOS, CategorySecurity, CategoryEmergency, CategoryCall} {

		repo.rows[key(userID, cat)] = &Preference{UserID: userID, Category: cat, Enabled: false, QuietStart: ptr(0), QuietEnd: ptr(1439)}
		assert.True(t, delivers(chk, userID, cat, now), "critical category %s must always deliver", cat)
	}
	assert.Zero(t, repo.queries, "critical sends must not even query preferences")
}

func TestFilter_MutedNonCriticalSuppressed(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	repo.rows[key(userID, CategoryAnnouncement)] = &Preference{UserID: userID, Category: CategoryAnnouncement, Enabled: false}

	assert.False(t, delivers(NewChecker(repo, time.UTC), userID, CategoryAnnouncement, time.Now()))
}

func TestFilter_NoPreferenceDefaultsToDeliver(t *testing.T) {
	assert.True(t, delivers(NewChecker(newMockRepo(), time.UTC), uuid.New(), CategoryAnnouncement, time.Now()))
}

func TestFilter_QuietHoursWrappingWindow(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()

	repo.rows[key(userID, CategoryGeneral)] = &Preference{UserID: userID, Category: CategoryGeneral, Enabled: true, QuietStart: ptr(1320), QuietEnd: ptr(360)}
	chk := NewChecker(repo, time.UTC)

	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)), "03:00 inside")
	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 23, 0, 0, 0, time.UTC)), "23:00 inside")
	assert.True(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)), "12:00 outside")
	assert.True(t, delivers(chk, userID, CategorySOS, time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)), "critical ignores quiet hours")
}

func TestFilter_SameDayQuietWindow(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	repo.rows[key(userID, CategoryGeneral)] = &Preference{UserID: userID, Category: CategoryGeneral, Enabled: true, QuietStart: ptr(60), QuietEnd: ptr(360)}
	chk := NewChecker(repo, time.UTC)

	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)))
	assert.True(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 7, 0, 0, 0, time.UTC)))
}

func TestFilter_FailsOpenOnRepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getErr = errors.New("boom")

	assert.True(t, delivers(NewChecker(repo, time.UTC), uuid.New(), CategoryAnnouncement, time.Now()))
}

func TestFilter_BatchesQueriesAndKeepsOrder(t *testing.T) {
	repo := newMockRepo()
	a, muted, b := uuid.New(), uuid.New(), uuid.New()
	repo.rows[key(muted, CategoryGeneral)] = &Preference{UserID: muted, Category: CategoryGeneral, Enabled: false}

	repo.rows[key(b, CategoryGeneral)] = &Preference{UserID: b, Category: CategoryGeneral, Enabled: true, QuietStart: ptr(60), QuietEnd: ptr(120)}
	noon := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	out := NewChecker(repo, time.UTC).FilterRecipients(context.Background(), []uuid.UUID{a, muted, b}, CategoryGeneral, noon)

	assert.Equal(t, []uuid.UUID{a, b}, out)
	assert.Equal(t, 2, repo.queries, "one preferences query + one timezone query, not one per recipient")
}

func TestFilter_SkipsTimezoneLookupWithoutQuietHours(t *testing.T) {
	repo := newMockRepo()
	muted := uuid.New()
	repo.rows[key(muted, CategoryGeneral)] = &Preference{UserID: muted, Category: CategoryGeneral, Enabled: false}

	NewChecker(repo, time.UTC).FilterRecipients(context.Background(), []uuid.UUID{uuid.New(), muted}, CategoryGeneral, time.Now())

	assert.Equal(t, 1, repo.queries, "only the preferences query")
}

func TestFilter_QuietHoursUseDefaultTimezone(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()

	repo.rows[key(userID, CategoryGeneral)] = &Preference{UserID: userID, Category: CategoryGeneral, Enabled: true, QuietStart: ptr(1320), QuietEnd: ptr(360)}
	chk := NewChecker(repo, bogota(t))

	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)))

	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)))

	assert.True(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 23, 0, 0, 0, time.UTC)))
}

func TestFilter_QuietHoursUseUserTimezone(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	repo.rows[key(userID, CategoryGeneral)] = &Preference{UserID: userID, Category: CategoryGeneral, Enabled: true, QuietStart: ptr(1320), QuietEnd: ptr(360)}
	repo.timezones[userID] = "Europe/Madrid"
	chk := NewChecker(repo, bogota(t))

	assert.False(t, delivers(chk, userID, CategoryGeneral, time.Date(2026, 6, 10, 21, 0, 0, 0, time.UTC)))
}

func TestGetPreferences_DefaultsAllEnabledWithDefaultTimezone(t *testing.T) {
	svc := NewService(newMockRepo(), bogota(t))

	resp, err := svc.GetPreferences(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "America/Bogota", resp.Timezone)
	require.Len(t, resp.Preferences, len(ConfigurableCategories()))
	for _, item := range resp.Preferences {
		assert.True(t, item.Enabled, "category %s should default enabled", item.Category)
		assert.Nil(t, item.QuietStart)
	}
}

func TestGetPreferences_ReflectsStoredRowsAndTimezone(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	repo.rows[key(userID, CategoryAnnouncement)] = &Preference{UserID: userID, Category: CategoryAnnouncement, Enabled: false}
	repo.timezones[userID] = "America/Mexico_City"

	resp, err := NewService(repo, bogota(t)).GetPreferences(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, "America/Mexico_City", resp.Timezone)
	for _, item := range resp.Preferences {
		if item.Category == CategoryAnnouncement {
			assert.False(t, item.Enabled)
		}
	}
}

func TestUpdatePreferences_UpsertsNonCritical(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()

	err := NewService(repo, time.UTC).UpdatePreferences(context.Background(), userID, nil, UpdatePreferencesRequest{
		Preferences: []PreferenceItem{
			{Category: CategoryAnnouncement, Enabled: false},
			{Category: CategoryGeneral, Enabled: true, QuietStart: ptr(1320), QuietEnd: ptr(360)},
		},
	})
	require.NoError(t, err)
	require.Len(t, repo.upserts, 2)
	assert.False(t, repo.rows[key(userID, CategoryAnnouncement)].Enabled)
	assert.Equal(t, 1320, *repo.rows[key(userID, CategoryGeneral)].QuietStart)
}

func TestUpdatePreferences_Rejections(t *testing.T) {
	cases := map[string]UpdatePreferencesRequest{
		"empty":             {},
		"critical":          {Preferences: []PreferenceItem{{Category: CategorySOS}}},
		"unknown category":  {Preferences: []PreferenceItem{{Category: "NOT_A_CATEGORY"}}},
		"half quiet window": {Preferences: []PreferenceItem{{Category: CategoryGeneral, QuietStart: ptr(60)}}},
		"out of range":      {Preferences: []PreferenceItem{{Category: CategoryGeneral, QuietStart: ptr(0), QuietEnd: ptr(1440)}}},
		"bad timezone":      {Timezone: ptr("Mars/Olympus")},
		"local timezone":    {Timezone: ptr("Local")},

		"partial batch": {Preferences: []PreferenceItem{{Category: CategoryGeneral}, {Category: CategorySOS}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newMockRepo()
			err := NewService(repo, time.UTC).UpdatePreferences(context.Background(), uuid.New(), nil, req)
			require.Error(t, err)
			assert.Empty(t, repo.upserts)
			assert.Empty(t, repo.timezones)
		})
	}
}

func TestUpdatePreferences_TimezoneOnly(t *testing.T) {
	repo := newMockRepo()
	userID := uuid.New()
	tz := "America/Lima"

	require.NoError(t, NewService(repo, time.UTC).UpdatePreferences(context.Background(), userID, nil, UpdatePreferencesRequest{Timezone: &tz}))
	assert.Equal(t, "America/Lima", repo.timezones[userID])
}
