package notification

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type sequenceSource struct {
	events [][]Event
	calls  int
}

func (s *sequenceSource) Events(context.Context) ([]Event, error) {
	s.calls++
	index := s.calls - 1
	if index >= len(s.events) {
		index = len(s.events) - 1
	}
	return append([]Event(nil), s.events[index]...), nil
}

type recordingSender struct {
	messages []string
	err      error
}

func (s *recordingSender) Send(_ context.Context, message string) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestPollBootstrapsExistingEventsWithoutSending(t *testing.T) {
	store, err := OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	source := &sequenceSource{events: [][]Event{{
		{Key: "pipeline:1:mr", OccurredAt: time.Unix(1, 0), Message: "MR готов"},
	}}}
	sender := &recordingSender{}

	if err := NewService(source, sender, store, testLogger()).Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("sent bootstrap messages: %#v", sender.messages)
	}
	if !store.Initialized() || !store.Contains("pipeline:1:mr") {
		t.Fatal("bootstrap state was not persisted")
	}
}

func TestPollSendsNewEventsInChronologicalOrder(t *testing.T) {
	store, err := OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	source := &sequenceSource{events: [][]Event{
		{},
		{
			{Key: "newer", OccurredAt: time.Unix(20, 0), Message: "второе"},
			{Key: "older", OccurredAt: time.Unix(10, 0), Message: "первое"},
		},
	}}
	sender := &recordingSender{}
	service := NewService(source, sender, store, testLogger())

	if err := service.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"первое", "второе"}; !reflect.DeepEqual(sender.messages, want) {
		t.Fatalf("messages = %#v, want %#v", sender.messages, want)
	}
}

func TestPollDoesNotMarkFailedSend(t *testing.T) {
	store, err := OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	source := &sequenceSource{events: [][]Event{{
		{Key: "pipeline:2:ready", OccurredAt: time.Unix(2, 0), Message: "версия готова"},
	}}}
	sender := &recordingSender{err: errors.New("telegram unavailable")}

	err = NewService(source, sender, store, testLogger()).Poll(context.Background())
	if err == nil {
		t.Fatal("Poll() error = nil")
	}
	if store.Contains("pipeline:2:ready") {
		t.Fatal("failed event was marked as sent")
	}
}

func TestPollDoesNotResendConfirmedEventAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	event := Event{Key: "deployment:3", OccurredAt: time.Unix(3, 0), Message: "развёрнуто"}
	firstSender := &recordingSender{}
	if err := NewService(&sequenceSource{events: [][]Event{{event}}}, firstSender, store, testLogger()).Poll(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	secondSender := &recordingSender{}
	if err := NewService(&sequenceSource{events: [][]Event{{event}}}, secondSender, reopened, testLogger()).Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(secondSender.messages) != 0 {
		t.Fatalf("resent messages: %#v", secondSender.messages)
	}
}

func TestRunStopsImmediatelyWhenContextIsCanceled(t *testing.T) {
	store, err := OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	source := &sequenceSource{events: [][]Event{{}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewService(source, &recordingSender{}, store, testLogger()).Run(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
	if source.calls != 0 {
		t.Fatalf("source calls = %d, want 0", source.calls)
	}
}
