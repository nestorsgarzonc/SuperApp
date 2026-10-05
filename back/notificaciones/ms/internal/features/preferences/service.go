package preferences

import (
	"context"

	"github.com/google/uuid"

	"superapp/notificaciones/internal/platform/apperr"
)

type Service interface {
	GetPreferences(ctx context.Context, userID uuid.UUID) (*GetPreferencesResponse, error)
	UpdatePreferences(ctx context.Context, userID uuid.UUID, req UpdatePreferencesRequest) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetPreferences(ctx context.Context, userID uuid.UUID) (*GetPreferencesResponse, error) {
	stored, err := s.repo.GetByUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	items := make([]PreferenceItem, 0, len(preferenceGroups))
	for _, g := range preferenceGroups {
		enabled, ok := stored[g.Group]
		if !ok || g.Locked {
			enabled = true
		}
		items = append(items, PreferenceItem{Group: g.Group, Enabled: enabled, Locked: g.Locked})
	}
	return &GetPreferencesResponse{Preferences: items}, nil
}

func (s *service) UpdatePreferences(ctx context.Context, userID uuid.UUID, req UpdatePreferencesRequest) error {
	if len(req.Preferences) == 0 {
		return apperr.Field("preferences", "At least one preference is required")
	}
	enabled := make(map[string]bool, len(req.Preferences))
	for _, p := range req.Preferences {
		if !isMutable(p.Group) {
			if IsGroup(p.Group) {
				return apperr.Field("group", "Group "+p.Group+" is mandatory and cannot be changed")
			}
			return apperr.Field("group", "Unknown group "+p.Group)
		}
		if p.Enabled == nil {
			return apperr.Field("enabled", "Enabled is required")
		}
		enabled[p.Group] = *p.Enabled
	}
	if err := s.repo.Save(ctx, userID, enabled); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

type Checker struct {
	repo Repository
}

func NewChecker(repo Repository) *Checker {
	return &Checker{repo: repo}
}

func (c *Checker) FilterRecipients(ctx context.Context, userIDs []uuid.UUID, category string) []uuid.UUID {
	group := GroupOf(category)
	if !isMutable(group) || len(userIDs) == 0 {
		return userIDs
	}
	muted, err := c.repo.MutedUsers(ctx, userIDs, group)
	if err != nil || len(muted) == 0 {
		return userIDs
	}
	out := make([]uuid.UUID, 0, len(userIDs))
	for _, id := range userIDs {
		if !muted[id] {
			out = append(out, id)
		}
	}
	return out
}
