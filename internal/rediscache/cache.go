package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"devops-practice/internal/request"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

var markProcessedScript = redis.NewScript(`
if redis.call('SETNX', KEYS[1], '1') == 1 then
  redis.call('INCR', KEYS[2])
  return 1
end
return 0
`)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func New(addr, password string, db int, ttl time.Duration) *Cache {
	return &Cache{
		client: redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db}),
		ttl:    ttl,
	}
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Get(ctx context.Context, id uuid.UUID) (request.Request, error) {
	raw, err := c.client.Get(ctx, requestKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return request.Request{}, ErrCacheMiss
	}
	if err != nil {
		return request.Request{}, fmt.Errorf("get cached request: %w", err)
	}

	var value request.Request
	if err := json.Unmarshal(raw, &value); err != nil {
		_ = c.client.Del(ctx, requestKey(id)).Err()
		return request.Request{}, ErrCacheMiss
	}
	return value, nil
}

func (c *Cache) Set(ctx context.Context, value request.Request) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cached request: %w", err)
	}
	if err := c.client.Set(ctx, requestKey(value.ID), raw, c.ttl).Err(); err != nil {
		return fmt.Errorf("cache request: %w", err)
	}
	return nil
}

func (c *Cache) Delete(ctx context.Context, id uuid.UUID) error {
	if err := c.client.Del(ctx, requestKey(id)).Err(); err != nil {
		return fmt.Errorf("delete cached request: %w", err)
	}
	return nil
}

func (c *Cache) MarkProcessedAndCount(ctx context.Context, id uuid.UUID) (bool, error) {
	result, err := markProcessedScript.Run(ctx, c.client, []string{processedKey(id), "stats:processed"}).Int64()
	if err != nil {
		return false, fmt.Errorf("count processed request: %w", err)
	}
	return result == 1, nil
}

func (c *Cache) ProcessedCount(ctx context.Context) (int64, error) {
	value, err := c.client.Get(ctx, "stats:processed").Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get processed count: %w", err)
	}
	return value, nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}

func requestKey(id uuid.UUID) string {
	return "request:" + id.String()
}

func processedKey(id uuid.UUID) string {
	return "processed:" + id.String()
}
