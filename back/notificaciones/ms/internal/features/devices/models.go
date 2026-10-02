package devices

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlatformIOS     = "IOS"
	PlatformAndroid = "ANDROID"
	PlatformWeb     = "WEB"
)

type Device struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Platform   string    `json:"platform"`
	PushToken  string    `json:"-"`
	DeviceName *string   `json:"device_name,omitempty"`
	AppVersion *string   `json:"app_version,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type RegisterDeviceRequest struct {
	Platform   string  `json:"platform"`
	PushToken  string  `json:"push_token"`
	DeviceName *string `json:"device_name,omitempty"`
	AppVersion *string `json:"app_version,omitempty"`
}

type UnregisterDeviceRequest struct {
	PushToken string `json:"push_token"`
}
