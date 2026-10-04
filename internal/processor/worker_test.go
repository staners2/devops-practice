package processor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"devops-practice/internal/request"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

type workerStoreStub struct {
	marked   int
	failures int
}

func (s *workerStoreStub) MarkProcessed(context.Context, uuid.UUID, time.Time) (bool, error) {
	s.marked++
	if s.failures > 0 {
		s.failures--
		return false, errors.New("temporary PostgreSQL failure")
	}
	return s.marked == 1, nil
}

type workerCacheStub struct {
	deleted int
	counted int
}

func (c *workerCacheStub) Delete(context.Context, uuid.UUID) error {
	c.deleted++
	return nil
}
func (c *workerCacheStub) MarkProcessedAndCount(context.Context, uuid.UUID) (bool, error) {
	c.counted++
	return c.counted == 1, nil
}

type workerConsumerStub struct {
	committed int
	polled    int
	record    *kgo.Record
}

func (c *workerConsumerStub) Poll(context.Context) (*kgo.Record, error) {
	c.polled++
	if c.polled == 1 && c.record != nil {
		return c.record, nil
	}
	return nil, context.Canceled
}
func (c *workerConsumerStub) Commit(context.Context, *kgo.Record) error {
	c.committed++
	return nil
}

func TestProcessRecordUpdatesStoresBeforeCommit(t *testing.T) {
	store := &workerStoreStub{}
	cache := &workerCacheStub{}
	consumer := &workerConsumerStub{}
	worker := New(store, cache, consumer, slog.New(slog.NewTextHandler(io.Discard, nil)))

	event := request.RequestCreatedEvent{
		Version: request.EventVersion, Type: request.EventTypeRequestCreated,
		RequestID:  uuid.MustParse("64ed00ff-c2f2-49ef-acdd-2042c2146b39"),
		OccurredAt: time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	record := &kgo.Record{Topic: "requests.created.v1", Partition: 0, Offset: 1, Value: raw}

	if err := worker.processRecord(context.Background(), record); err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if store.marked != 1 || cache.deleted != 1 || cache.counted != 1 || consumer.committed != 1 {
		t.Fatalf("marked=%d deleted=%d counted=%d committed=%d", store.marked, cache.deleted, cache.counted, consumer.committed)
	}
}

func TestProcessRecordCommitsPoisonMessageWithoutCallingStores(t *testing.T) {
	store := &workerStoreStub{}
	cache := &workerCacheStub{}
	consumer := &workerConsumerStub{}
	worker := New(store, cache, consumer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	record := &kgo.Record{Topic: "requests.created.v1", Partition: 0, Offset: 2, Value: []byte(`{"version":2}`)}

	if err := worker.processRecord(context.Background(), record); err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if store.marked != 0 || cache.deleted != 0 || cache.counted != 0 || consumer.committed != 1 {
		t.Fatalf("marked=%d deleted=%d counted=%d committed=%d", store.marked, cache.deleted, cache.counted, consumer.committed)
	}
}

func TestRunRetriesCurrentRecordBeforePollingAgain(t *testing.T) {
	event := request.RequestCreatedEvent{
		Version: request.EventVersion, Type: request.EventTypeRequestCreated,
		RequestID:  uuid.MustParse("384b7e7d-8031-483b-9159-5fd7fcf61c91"),
		OccurredAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	store := &workerStoreStub{failures: 1}
	cache := &workerCacheStub{}
	consumer := &workerConsumerStub{record: &kgo.Record{Topic: "requests.created.v1", Value: raw}}
	worker := New(store, cache, consumer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker.retryDelay = time.Millisecond

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if store.marked != 2 || consumer.committed != 1 || consumer.polled != 2 {
		t.Fatalf("marked=%d committed=%d polled=%d", store.marked, consumer.committed, consumer.polled)
	}
}
