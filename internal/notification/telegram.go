package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"
)

const telegramResponseLimit = 64 << 10

type TelegramSender struct {
	client *http.Client
	token  string
	chatID string
}

func NewTelegramSender(client *http.Client, token, chatID string) *TelegramSender {
	return &TelegramSender{client: client, token: token, chatID: chatID}
}

func (t *TelegramSender) Send(ctx context.Context, message string) error {
	if utf8.RuneCountInString(message) > 4096 {
		return errors.New("telegram message exceeds 4096 characters")
	}
	payload, err := json.Marshal(struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{ChatID: t.chatID, Text: message})
	if err != nil {
		return errors.New("encode telegram message")
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.telegram.org/bot"+t.token+"/sendMessage",
		bytes.NewReader(payload),
	)
	if err != nil {
		return errors.New("create telegram request")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := t.client.Do(request)
	if err != nil {
		return errors.New("telegram send failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, telegramResponseLimit+1))
	if err != nil || len(body) > telegramResponseLimit {
		return errors.New("telegram send failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed: status %d", response.StatusCode)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return errors.New("telegram send failed")
	}
	return nil
}
