package observability

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLoggerDoesNotExposeDriverErrorSecrets(t *testing.T) {
	var output bytes.Buffer
	Logger(&output).Error("connect PostgreSQL", "error", errors.New("postgres://user:secret@host/db"), "request_id", "business-id")
	if strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "business-id") || !strings.Contains(output.String(), "operation_failed") {
		t.Fatalf("unsafe error log: %s", output.String())
	}
}
