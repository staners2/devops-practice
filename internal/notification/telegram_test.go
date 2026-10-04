package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func telegramResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestTelegramSendPostsJSONMessage(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s", request.Method)
		}
		if request.URL.Path != "/bot12345678:test-token/sendMessage" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["chat_id"] != "-1001234567890" || payload["text"] != "MR готов" {
			t.Fatalf("payload = %#v", payload)
		}
		if _, exists := payload["parse_mode"]; exists {
			t.Fatal("parse_mode must not be sent")
		}
		return telegramResponse(http.StatusOK, `{"ok":true}`), nil
	})}

	if err := NewTelegramSender(client, "12345678:test-token", "-1001234567890").Send(context.Background(), "MR готов"); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramSendRejectsNon2xxWithoutLeakingToken(t *testing.T) {
	const token = "12345678:secret-token"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return telegramResponse(http.StatusUnauthorized, `{"ok":false,"description":"token 12345678:secret-token invalid"}`), nil
	})}

	err := NewTelegramSender(client, token, "chat").Send(context.Background(), "test")
	if err == nil {
		t.Fatal("Send() error = nil")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestTelegramSendRejectsOKFalse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return telegramResponse(http.StatusOK, `{"ok":false}`), nil
	})}

	if err := NewTelegramSender(client, "token", "chat").Send(context.Background(), "test"); err == nil {
		t.Fatal("Send() error = nil")
	}
}

func TestTelegramSendRejectsMessageLongerThan4096Runes(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return telegramResponse(http.StatusOK, `{"ok":true}`), nil
	})}

	err := NewTelegramSender(client, "token", "chat").Send(context.Background(), strings.Repeat("я", 4097))
	if err == nil {
		t.Fatal("Send() error = nil")
	}
	if called {
		t.Fatal("oversized message was sent")
	}
}
