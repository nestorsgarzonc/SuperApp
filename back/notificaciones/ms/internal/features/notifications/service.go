package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"superapp/notificaciones/internal/features/preferences"
	"superapp/notificaciones/internal/platform/apperr"
	"superapp/notificaciones/internal/platform/push"
)

type Service interface {
	GetUserNotifications(ctx context.Context, complexID, userID uuid.UUID, q InboxQuery) ([]*Notification, error)
	CountUnread(ctx context.Context, complexID, userID uuid.UUID) (int, error)
	MarkAsRead(ctx context.Context, id, complexID, userID uuid.UUID) error
	MarkAllAsRead(ctx context.Context, complexID, userID uuid.UUID) (int64, error)

	Dispatch(ctx context.Context, in DispatchInput) error

	Wait()
}

type TokenStore interface {
	TokensByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	DeleteTokens(ctx context.Context, tokens []string) error
}

type DeliveryChecker interface {
	FilterRecipients(ctx context.Context, userIDs []uuid.UUID, category string) []uuid.UUID
}

const (
	pushConcurrency     = 20
	criticalConcurrency = 200

	pushTimeout = 2 * time.Minute
)

type service struct {
	repo    Repository
	tokens  TokenStore
	pusher  push.Sender
	checker DeliveryChecker
	logger  *slog.Logger
	now     func() time.Time

	pushSlots     chan struct{}
	criticalSlots chan struct{}
	pushes        sync.WaitGroup
}

func NewService(repo Repository, tokens TokenStore, pusher push.Sender, checker DeliveryChecker, logger *slog.Logger) Service {
	return &service{
		repo: repo, tokens: tokens, pusher: pusher,
		checker: checker, logger: logger, now: time.Now,
		pushSlots:     make(chan struct{}, pushConcurrency),
		criticalSlots: make(chan struct{}, criticalConcurrency),
	}
}

func (s *service) Wait() { s.pushes.Wait() }

func (s *service) GetUserNotifications(ctx context.Context, complexID, userID uuid.UUID, q InboxQuery) ([]*Notification, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	var types []string
	for _, g := range q.Groups {
		if !preferences.IsGroup(g) {
			return nil, apperr.Field("group", "Unknown group "+g)
		}
	}
	if len(q.Groups) > 0 {
		types = preferences.CategoriesOf(q.Groups)
	}
	out, err := s.repo.GetUserNotifications(ctx, complexID, userID, q.UnreadOnly, types, q.Limit)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	for _, n := range out {
		n.Group = preferences.GroupOf(n.Type)
	}
	return out, nil
}

func (s *service) CountUnread(ctx context.Context, complexID, userID uuid.UUID) (int, error) {
	n, err := s.repo.CountUnread(ctx, complexID, userID)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	return n, nil
}

func (s *service) MarkAsRead(ctx context.Context, id, complexID, userID uuid.UUID) error {
	if err := s.repo.MarkAsRead(ctx, id, complexID, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.NotFound("Notification", id.String())
		}
		return apperr.Internal(err)
	}
	return nil
}

func (s *service) MarkAllAsRead(ctx context.Context, complexID, userID uuid.UUID) (int64, error) {
	n, err := s.repo.MarkAllAsRead(ctx, complexID, userID)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	return n, nil
}

func (s *service) Dispatch(ctx context.Context, in DispatchInput) error {
	nn, err := validate(in)
	if err != nil {
		return err
	}
	if s.checker != nil {
		nn.UserIDs = s.checker.FilterRecipients(ctx, nn.UserIDs, nn.Type)
	}

	created, err := s.repo.CreateNotifications(ctx, nn)
	if errors.Is(err, ErrDuplicateEvent) {
		s.logger.Info("event already processed, skipping", "event_id", in.EventID)
		return nil
	}
	if err != nil {
		return apperr.Internal(err)
	}
	if len(created) > 0 && s.pusher != nil && s.tokens != nil {

		pushCtx := context.WithoutCancel(ctx)
		s.pushes.Add(1)
		go func() {
			defer s.pushes.Done()
			pushCtx, cancel := context.WithTimeout(pushCtx, pushTimeout)
			defer cancel()
			s.push(pushCtx, nn, created)
		}()
	}
	return nil
}

func validate(in DispatchInput) (NewNotifications, error) {
	if strings.TrimSpace(in.EventID) == "" {
		return NewNotifications{}, apperr.Field("event_id", "Event ID is required")
	}
	if !preferences.IsKnown(in.Category) {
		return NewNotifications{}, apperr.Field("category", "Unknown category "+in.Category)
	}
	if in.ComplexID != nil && *in.ComplexID == uuid.Nil {
		return NewNotifications{}, apperr.Field("complex_id", "Complex ID must be a valid UUID")
	}
	if len(in.UserIDs) == 0 {
		return NewNotifications{}, apperr.Field("user_ids", "At least one user ID is required")
	}
	seen := make(map[uuid.UUID]struct{}, len(in.UserIDs))
	unique := make([]uuid.UUID, 0, len(in.UserIDs))
	for _, id := range in.UserIDs {
		if id == uuid.Nil {
			return NewNotifications{}, apperr.Field("user_ids", "User IDs must be valid UUIDs")
		}
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	if len(unique) > maxRecipients {
		return NewNotifications{}, apperr.Field("user_ids", fmt.Sprintf("At most %d recipients per event", maxRecipients))
	}

	title := truncate(sanitize(in.Title, false), 200)
	if title == "" {
		return NewNotifications{}, apperr.Field("title", "Title is required")
	}
	body := truncate(sanitize(in.Body, true), 2000)
	if body == "" {
		return NewNotifications{}, apperr.Field("body", "Body is required")
	}
	return NewNotifications{
		EventID: in.EventID, Source: in.Source, ComplexID: in.ComplexID, UserIDs: unique,
		Title: title, Body: body, Data: JSONB(in.Data), Type: in.Category,
	}, nil
}

func (s *service) push(ctx context.Context, in NewNotifications, created []*Notification) {
	tokensByUser, err := s.tokens.TokensByUserIDs(ctx, in.UserIDs)
	if err != nil {
		s.logger.Error("failed to resolve push tokens", "error", err)
		return
	}

	critical := preferences.IsCritical(in.Type)
	slots := s.pushSlots
	if critical {
		slots = s.criticalSlots
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var stale []string

	for _, n := range created {
		tokens := tokensByUser[n.UserID]
		if len(tokens) == 0 {
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			s.logger.Warn("push timed out waiting for a slot", "notification_id", n.ID)
			continue
		}
		wg.Add(1)
		go func(n *Notification, tokens []string) {
			defer wg.Done()
			defer func() { <-slots }()
			unregistered := s.pushOne(ctx, in, n, tokens, critical)
			if len(unregistered) > 0 {
				mu.Lock()
				stale = append(stale, unregistered...)
				mu.Unlock()
			}
		}(n, tokens)
	}
	wg.Wait()

	if err := s.tokens.DeleteTokens(ctx, stale); err != nil {
		s.logger.Error("failed to prune unregistered tokens", "error", err)
	}
}

func (s *service) pushOne(ctx context.Context, in NewNotifications, n *Notification, tokens []string, critical bool) []string {
	res, err := s.pusher.Send(ctx, push.Message{
		Tokens: tokens, Title: in.Title, Body: in.Body,
		Type: in.Type, Data: in.Data, Critical: critical,
	})
	status := DeliveryStatusPushSent
	var deliveredAt *time.Time
	if err != nil || len(res.Failed) == len(tokens) {
		status = DeliveryStatusPushFailed
		if err != nil {
			s.logger.Warn("push failed", "error", err, "notification_id", n.ID)
		}
	} else {
		now := s.now().UTC()
		deliveredAt = &now
		if len(res.Failed) > 0 {
			status = DeliveryStatusPushPartial
		}
	}
	if err := s.repo.UpdateDelivery(ctx, n.ID, status, deliveredAt); err != nil {
		s.logger.Error("failed to record delivery", "error", err, "notification_id", n.ID)
	}
	return res.Unregistered
}

func sanitize(s string, multiline bool) string {
	s = strings.Map(func(r rune) rune {
		if multiline && (r == '\n' || r == '\t') {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
