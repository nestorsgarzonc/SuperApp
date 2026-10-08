package users

import "time"

type UpdateRequester struct {
	ID       string
	Email    *string
	FullName *string
	Status   *string
}

type UpdatePresenter struct{}

type GetRequester struct {
	UserID string
}

type Presenter struct {
	ID        string
	Email     string
	FullName  string
	Status    string
	CreatedAt time.Time
}
