package queue

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("queue: not found")

type Job struct {
	ID          string
	EventID     string
	Attempt     int
	AvailableAt time.Time
	LastError   string
	Payload     map[string]any
}

type Queue interface {
	Enqueue(ctx context.Context, job *Job) error
	Dequeue(ctx context.Context) (*Job, error)
	Ack(ctx context.Context, jobID string) error
	Nack(ctx context.Context, jobID, reason string) error
}
