package sessions

import "time"

type RevokeParams struct {
	UserID    string
	SessionID string
}

type ListRequester struct {
	UserID string
}

type ListPresenter struct {
	Sessions []Presenter
}

type Presenter struct {
	ID        string
	Device    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type LogoutAllRequester struct {
	UserID string
}

type LogoutAllPresenter struct{}

type RevokePresenter struct{}
