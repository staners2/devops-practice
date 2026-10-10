package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
)

// Logger keeps the failed operation in the message without exposing driver payloads.
func Logger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if err, ok := attr.Value.Any().(error); ok {
			kind := "operation_failed"
			var networkError net.Error
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				kind = "deadline_exceeded"
			case errors.Is(err, context.Canceled):
				kind = "canceled"
			case errors.As(err, &networkError):
				kind = "network_error"
			}
			return slog.String(attr.Key, kind)
		}
		return attr
	}}))
}
