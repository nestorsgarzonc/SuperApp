package repository

import "context"

type HealthRepository interface {
	Ping(context.Context) error
}
