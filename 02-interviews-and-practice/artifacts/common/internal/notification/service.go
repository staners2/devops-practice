package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

type Service struct {
	source Source
	sender Sender
	state  *StateStore
	logger *slog.Logger
}

func NewService(source Source, sender Sender, state *StateStore, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{source: source, sender: sender, state: state, logger: logger}
}

func (s *Service) Poll(ctx context.Context) error {
	events, err := s.source.Events(ctx)
	if err != nil {
		return fmt.Errorf("read notification events: %w", err)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].Key < events[j].Key
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})

	if !s.state.Initialized() {
		keys := make([]string, 0, len(events))
		for _, event := range events {
			keys = append(keys, event.Key)
		}
		if err := s.state.Bootstrap(keys...); err != nil {
			return fmt.Errorf("bootstrap notification state: %w", err)
		}
		return nil
	}

	for _, event := range events {
		if s.state.Contains(event.Key) {
			continue
		}
		if err := s.sender.Send(ctx, event.Message); err != nil {
			return fmt.Errorf("send notification event %q: %w", event.Key, err)
		}
		if err := s.state.Mark(event.Key); err != nil {
			return fmt.Errorf("persist notification event %q: %w", event.Key, err)
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	if ctx.Err() != nil {
		return nil
	}
	if err := s.Poll(ctx); err != nil {
		s.logger.Error("notification poll failed", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Poll(ctx); err != nil {
				s.logger.Error("notification poll failed", "error", err)
			}
		}
	}
}
