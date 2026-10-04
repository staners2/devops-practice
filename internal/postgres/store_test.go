package postgres

import (
	"context"
	"testing"
)

func TestNewRejectsInvalidDatabaseURL(t *testing.T) {
	_, err := New(context.Background(), "://invalid")
	if err == nil {
		t.Fatal("New() error = nil, want invalid database URL error")
	}
}
