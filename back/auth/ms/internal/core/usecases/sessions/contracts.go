package sessions

import "time"

type (
	RevokeParams struct {
		UserID    string
		SessionID string
	}

	ListRequester struct {
		UserID string
	}

	ListPresenter struct {
		Sessions []Presenter
	}

	Presenter struct {
		ID        string
		Device    string
		CreatedAt time.Time
		ExpiresAt time.Time
	}

	LogoutAllRequester struct {
		UserID string
	}

	LogoutAllPresenter struct{}

	RevokePresenter struct{}
)
