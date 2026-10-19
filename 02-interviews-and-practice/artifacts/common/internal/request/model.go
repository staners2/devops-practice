package request

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusCreated           = "created"
	StatusProcessed         = "processed"
	EventTypeRequestCreated = "request.created"
	EventVersion            = 1
)

type Request struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Title string `json:"title"`
}

type RequestCreatedEvent struct {
	Version    int       `json:"version"`
	Type       string    `json:"type"`
	RequestID  uuid.UUID `json:"request_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
