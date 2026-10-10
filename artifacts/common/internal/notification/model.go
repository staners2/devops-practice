package notification

import (
	"context"
	"time"
)

type Event struct {
	Key        string
	OccurredAt time.Time
	Message    string
}

type Source interface {
	Events(context.Context) ([]Event, error)
}

type Sender interface {
	Send(context.Context, string) error
}
