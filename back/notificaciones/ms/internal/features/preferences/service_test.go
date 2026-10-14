package preferences

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"superapp/notificaciones/internal/platform/apperr"
)

type mockRepo struct {
	rows    map[uuid.UUID]map[string]bool
	err     error
	queries int
}

func newMockRepo() *mockRepo { return &mockRepo{rows: map[uuid.UUID]map[string]bool{}} }

func (m *mockRepo) set(userID uuid.UUID, group string, enabled bool) {
	if m.rows[userID] == nil {
		m.rows[userID] = map[string]bool{}
	}
	m.rows[userID][group] = enabled
}

func (m *mockRepo) GetByUser(_ context.Context, userID uuid.UUID) (map[string]bool, error) {
	out := map[string]bool{}
	for g, e := range m.rows[userID] {
		out[g] = e
	}
	return out, m.err
}

func (m *mockRepo) MutedUsers(_ context.Context, userIDs []uuid.UUID, group string) (map[uuid.UUID]bool, error) {
	m.queries++
	if m.err != nil {
		return nil, m.err
	}
	out := map[uuid.UUID]bool{}
	for _, id := range userIDs {
		if enabled, ok := m.rows[id][group]; ok && !enabled {
			out[id] = true
		}
	}
	return out, nil
}

func (m *mockRepo) Save(_ context.Context, userID uuid.UUID, enabled map[string]bool) error {
	for g, e := range enabled {
		m.set(userID, g, e)
	}
	return m.err
}

func ptr[T any](v T) *T { return &v }

func TestCategories_EveryCategoryHasAGroup(t *testing.T) {
	for _, c := range []string{CategorySOS, CategoryCall, CategoryVisitor, CategoryReservation, CategoryVote, CategoryAccount} {
		assert.True(t, IsKnown(c), c)
	}
	assert.Equal(t, GroupIntercom, GroupOf(CategoryCall))
	assert.Equal(t, GroupVotes, GroupOf(CategoryVote))
	assert.False(t, IsKnown("AMENITY"))
	assert.Equal(t, []string{CategoryAssembly, CategoryVote}, CategoriesOf([]string{GroupAssemblies, GroupVotes}))
}

func TestFilter_MutedGroupSuppressed(t *testing.T) {
	repo := newMockRepo()
	a, muted, b := uuid.New(), uuid.New(), uuid.New()
	repo.set(muted, GroupCommunity, false)
	repo.set(b, GroupCommunity, true)

	out := NewChecker(repo).FilterRecipients(context.Background(), []uuid.UUID{a, muted, b}, CategoryAnnouncement)

	assert.Equal(t, []uuid.UUID{a, b}, out)
	assert.Equal(t, 1, repo.queries, "one query per event, not per recipient")
}

func TestFilter_LockedAndSystemGroupsAlwaysDeliver(t *testing.T) {
	repo := newMockRepo()
	u := uuid.New()
	repo.set(u, GroupAccessControl, false)
	repo.set(u, GroupIntercom, false)

	for _, c := range []string{CategoryVisitor, CategorySOS, CategoryCall, CategoryAccount} {
		out := NewChecker(repo).FilterRecipients(context.Background(), []uuid.UUID{u}, c)
		assert.Equal(t, []uuid.UUID{u}, out, c)
	}
	assert.Zero(t, repo.queries, "locked groups never hit the database")
}

func TestFilter_FailsOpenOnRepoError(t *testing.T) {
	repo := newMockRepo()
	repo.err = errors.New("boom")
	u := uuid.New()

	out := NewChecker(repo).FilterRecipients(context.Background(), []uuid.UUID{u}, CategoryReservation)

	assert.Equal(t, []uuid.UUID{u}, out)
}

func TestGetPreferences_DesignOrderDefaultsAndLocks(t *testing.T) {
	repo := newMockRepo()
	u := uuid.New()
	repo.set(u, GroupVotes, false)
	repo.set(u, GroupAccessControl, false)

	resp, err := NewService(repo).GetPreferences(context.Background(), u)

	require.NoError(t, err)
	assert.Equal(t, []PreferenceItem{
		{Group: GroupAccessControl, Enabled: true, Locked: true},
		{Group: GroupIntercom, Enabled: true, Locked: true},
		{Group: GroupReservations, Enabled: true},
		{Group: GroupCommunity, Enabled: true},
		{Group: GroupAssemblies, Enabled: true},
		{Group: GroupVotes, Enabled: false},
	}, resp.Preferences)
}

func TestUpdatePreferences_SavesMutableGroups(t *testing.T) {
	repo := newMockRepo()
	u := uuid.New()

	err := NewService(repo).UpdatePreferences(context.Background(), u, UpdatePreferencesRequest{Preferences: []PreferenceUpdate{
		{Group: GroupVotes, Enabled: ptr(false)},
		{Group: GroupReservations, Enabled: ptr(true)},
	}})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{GroupVotes: false, GroupReservations: true}, repo.rows[u])
}

func TestUpdatePreferences_Rejections(t *testing.T) {
	cases := map[string]UpdatePreferencesRequest{
		"empty":           {},
		"locked group":    {Preferences: []PreferenceUpdate{{Group: GroupAccessControl, Enabled: ptr(false)}}},
		"intercom locked": {Preferences: []PreferenceUpdate{{Group: GroupIntercom, Enabled: ptr(false)}}},
		"system":          {Preferences: []PreferenceUpdate{{Group: GroupSystem, Enabled: ptr(false)}}},
		"unknown group":   {Preferences: []PreferenceUpdate{{Group: "NOPE", Enabled: ptr(false)}}},
		"missing enabled": {Preferences: []PreferenceUpdate{{Group: GroupVotes}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newMockRepo()
			err := NewService(repo).UpdatePreferences(context.Background(), uuid.New(), req)
			assert.True(t, apperr.IsValidation(err))
			assert.Empty(t, repo.rows)
		})
	}
}
