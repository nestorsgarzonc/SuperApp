package devices

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"superapp/notificaciones/internal/platform/apperr"
)

type Service interface {
	Register(ctx context.Context, userID uuid.UUID, req RegisterDeviceRequest) (*Device, error)
	List(ctx context.Context, userID uuid.UUID) ([]*Device, error)
	Unregister(ctx context.Context, id, userID uuid.UUID) error
	UnregisterToken(ctx context.Context, token string, userID uuid.UUID) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Register(ctx context.Context, userID uuid.UUID, req RegisterDeviceRequest) (*Device, error) {
	req.Platform = strings.ToUpper(strings.TrimSpace(req.Platform))
	switch req.Platform {
	case PlatformIOS, PlatformAndroid, PlatformWeb:
	default:
		return nil, apperr.Field("platform", "Platform must be one of IOS, ANDROID, WEB")
	}
	req.PushToken = strings.TrimSpace(req.PushToken)
	if req.PushToken == "" || len(req.PushToken) > 4096 {
		return nil, apperr.Field("push_token", "A valid push token is required")
	}

	d, err := s.repo.Upsert(ctx, &Device{
		UserID:     userID,
		Platform:   req.Platform,
		PushToken:  req.PushToken,
		DeviceName: req.DeviceName,
		AppVersion: req.AppVersion,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return d, nil
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]*Device, error) {
	out, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

func (s *service) Unregister(ctx context.Context, id, userID uuid.UUID) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.NotFound("Device", id.String())
		}
		return apperr.Internal(err)
	}
	return nil
}

func (s *service) UnregisterToken(ctx context.Context, token string, userID uuid.UUID) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return apperr.Field("push_token", "Push token is required")
	}
	if err := s.repo.DeleteByToken(ctx, token, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.NotFound("Device", "with this token")
		}
		return apperr.Internal(err)
	}
	return nil
}
