package rediscache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

func TestMarkProcessedAndCountCountsRequestOnce(t *testing.T) {
	server := miniredis.RunT(t)
	cache := New(server.Addr(), "", 0, time.Minute)
	t.Cleanup(func() { _ = cache.Close() })

	id := uuid.MustParse("2ff51af3-6e98-4d33-ad04-7cc30ea2de3f")
	first, err := cache.MarkProcessedAndCount(context.Background(), id)
	if err != nil {
		t.Fatalf("first MarkProcessedAndCount() error = %v", err)
	}
	second, err := cache.MarkProcessedAndCount(context.Background(), id)
	if err != nil {
		t.Fatalf("second MarkProcessedAndCount() error = %v", err)
	}
	count, err := cache.ProcessedCount(context.Background())
	if err != nil {
		t.Fatalf("ProcessedCount() error = %v", err)
	}

	if !first || second || count != 1 {
		t.Fatalf("first=%v second=%v count=%d, want true false 1", first, second, count)
	}
}
