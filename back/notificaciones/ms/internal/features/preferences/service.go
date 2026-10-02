package preferences

import (
	"context"
	"time"

	"github.com/google/uuid"

	"superapp/notificaciones/internal/platform/apperr"
)

type Service interface {
	GetPreferences(ctx context.Context, userID uuid.UUID) (*GetPreferencesResponse, error)
	UpdatePreferences(ctx context.Context, userID uuid.UUID, complexID *uuid.UUID, req UpdatePreferencesRequest) error
}

type service struct {
	repo       Repository
	defaultLoc *time.Location
}

func NewService(repo Repository, defaultLoc *time.Location) Service {
	return &service{repo: repo, defaultLoc: defaultLoc}
}

func (s *service) GetPreferences(ctx context.Context, userID uuid.UUID) (*GetPreferencesResponse, error) {
	stored, err := s.repo.GetByUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	timezones, err := s.repo.GetTimezones(ctx, []uuid.UUID{userID})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	tz, ok := timezones[userID]
	if !ok {
		tz = s.defaultLoc.String()
	}

	byCategory := make(map[string]*Preference, len(stored))
	for _, p := range stored {
		byCategory[p.Category] = p
	}

	cats := ConfigurableCategories()
	items := make([]PreferenceItem, 0, len(cats))
	for _, cat := range cats {

		item := PreferenceItem{Category: cat, Enabled: true}
		if p, ok := byCategory[cat]; ok {
			item.Enabled = p.Enabled
			item.QuietStart = p.QuietStart
			item.QuietEnd = p.QuietEnd
		}
		items = append(items, item)
	}
	return &GetPreferencesResponse{Timezone: tz, Preferences: items}, nil
}

func (s *service) UpdatePreferences(ctx context.Context, userID uuid.UUID, complexID *uuid.UUID, req UpdatePreferencesRequest) error {
	if len(req.Preferences) == 0 && req.Timezone == nil {
		return apperr.Field("preferences", "At least one preference or a timezone is required")
	}
	if req.Timezone != nil {
		if _, err := time.LoadLocation(*req.Timezone); err != nil || *req.Timezone == "" || *req.Timezone == "Local" {
			return apperr.Field("timezone", "Timezone must be a valid IANA name, e.g. America/Bogota")
		}
	}

	for _, in := range req.Preferences {
		if in.Category == "" {
			return apperr.Field("category", "Category is required")
		}

		if IsCritical(in.Category) {
			return apperr.Field("category", "Category "+in.Category+" is critical and cannot be muted or scheduled")
		}
		if !IsKnown(in.Category) {
			return apperr.Field("category", "Unknown category "+in.Category)
		}
		if err := validateQuietHours(in.QuietStart, in.QuietEnd); err != nil {
			return err
		}
	}

	if req.Timezone != nil {
		if err := s.repo.UpsertTimezone(ctx, userID, *req.Timezone); err != nil {
			return apperr.Internal(err)
		}
	}
	for _, in := range req.Preferences {
		if err := s.repo.Upsert(ctx, &Preference{
			UserID:     userID,
			ComplexID:  complexID,
			Category:   in.Category,
			Enabled:    in.Enabled,
			QuietStart: in.QuietStart,
			QuietEnd:   in.QuietEnd,
		}); err != nil {
			return apperr.Internal(err)
		}
	}
	return nil
}

func validateQuietHours(start, end *int) error {
	if (start == nil) != (end == nil) {
		return apperr.Field("quiet_hours", "quiet_start and quiet_end must both be set or both be null")
	}
	for _, v := range []*int{start, end} {
		if v != nil && (*v < 0 || *v >= 1440) {
			return apperr.Field("quiet_hours", "quiet hours must be minutes-of-day in [0, 1440)")
		}
	}
	return nil
}

type Checker struct {
	repo       Repository
	defaultLoc *time.Location
}

func NewChecker(repo Repository, defaultLoc *time.Location) *Checker {
	return &Checker{repo: repo, defaultLoc: defaultLoc}
}

func (c *Checker) FilterRecipients(ctx context.Context, userIDs []uuid.UUID, category string, now time.Time) []uuid.UUID {
	if IsCritical(category) || len(userIDs) == 0 {
		return userIDs
	}
	prefs, err := c.repo.GetByUsersCategory(ctx, userIDs, category)
	if err != nil {
		return userIDs
	}

	var quiet []uuid.UUID
	for id, p := range prefs {
		if p.Enabled && p.QuietStart != nil {
			quiet = append(quiet, id)
		}
	}
	var timezones map[uuid.UUID]string
	if len(quiet) > 0 {
		if timezones, err = c.repo.GetTimezones(ctx, quiet); err != nil {
			timezones = nil
		}
	}
	locs := map[string]*time.Location{}

	out := make([]uuid.UUID, 0, len(userIDs))
	for _, id := range userIDs {
		pref, ok := prefs[id]
		if !ok {
			out = append(out, id)
			continue
		}
		if !pref.Enabled {
			continue
		}
		tz := timezones[id]
		loc, ok := locs[tz]
		if !ok {
			loc = c.location(tz)
			locs[tz] = loc
		}
		if withinQuietHours(pref.QuietStart, pref.QuietEnd, now.In(loc)) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (c *Checker) location(tz string) *time.Location {
	if tz == "" {
		return c.defaultLoc
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return c.defaultLoc
	}
	return loc
}

func withinQuietHours(start, end *int, now time.Time) bool {
	if start == nil || end == nil {
		return false
	}
	s, e := *start, *end
	if s == e {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if s < e {
		return m >= s && m < e
	}
	return m >= s || m < e
}
