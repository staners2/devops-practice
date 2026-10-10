package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"devops-practice/internal/request"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Store interface {
	MarkProcessed(context.Context, uuid.UUID, time.Time) (bool, error)
}

type Cache interface {
	Delete(context.Context, uuid.UUID) error
	MarkProcessedAndCount(context.Context, uuid.UUID) (bool, error)
}

type Consumer interface {
	Poll(context.Context) (*kgo.Record, error)
	Commit(context.Context, *kgo.Record) error
}

type Worker struct {
	store      Store
	cache      Cache
	consumer   Consumer
	logger     *slog.Logger
	retryDelay time.Duration
}

func New(store Store, cache Cache, consumer Consumer, logger *slog.Logger) *Worker {
	return &Worker{
		store: store, cache: cache, consumer: consumer, logger: logger,
		retryDelay: 2 * time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		record, err := w.consumer.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			w.logger.Error("poll Kafka", "error", err)
			if err := wait(ctx, 2*time.Second); err != nil {
				return nil
			}
			continue
		}
		for {
			if err := w.processRecord(ctx, record); err != nil {
				w.logger.Error("process Kafka record", "topic", record.Topic, "partition", record.Partition, "offset", record.Offset, "error", err)
				if err := wait(ctx, w.retryDelay); err != nil {
					return nil
				}
				continue
			}
			break
		}
	}
}

func (w *Worker) processRecord(ctx context.Context, record *kgo.Record) error {
	var event request.RequestCreatedEvent
	if err := json.Unmarshal(record.Value, &event); err != nil || !validEvent(event) {
		w.logger.Warn("discard invalid Kafka record", "topic", record.Topic, "partition", record.Partition, "offset", record.Offset)
		return w.consumer.Commit(ctx, record)
	}

	if _, err := w.store.MarkProcessed(ctx, event.RequestID, time.Now().UTC()); err != nil {
		return fmt.Errorf("update PostgreSQL: %w", err)
	}
	if err := w.cache.Delete(ctx, event.RequestID); err != nil {
		return fmt.Errorf("invalidate Redis cache: %w", err)
	}
	counted, err := w.cache.MarkProcessedAndCount(ctx, event.RequestID)
	if err != nil {
		return fmt.Errorf("update Redis counter: %w", err)
	}
	if err := w.consumer.Commit(ctx, record); err != nil {
		return fmt.Errorf("commit Kafka offset: %w", err)
	}
	w.logger.Info("request processed", "request_id", event.RequestID, "counted", counted, "topic", record.Topic, "partition", record.Partition, "offset", record.Offset)
	return nil
}

func validEvent(event request.RequestCreatedEvent) bool {
	return event.Version == request.EventVersion &&
		event.Type == request.EventTypeRequestCreated &&
		event.RequestID != uuid.Nil &&
		!event.OccurredAt.IsZero()
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
