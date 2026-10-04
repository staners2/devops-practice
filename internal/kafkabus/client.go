package kafkabus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"devops-practice/internal/request"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(brokers []string) (*Producer, error) {
	if len(brokers) == 0 {
		return nil, errors.New("at least one Kafka broker is required")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowIdempotentProduceCancellation(),
		kgo.RecordDeliveryTimeout(5*time.Second),
		kgo.RecordRetries(3),
		kgo.RequestTimeoutOverhead(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &Producer{client: client}, nil
}

func (p *Producer) Ping(ctx context.Context) error {
	return p.client.Ping(ctx)
}

func (p *Producer) Publish(ctx context.Context, topic string, event request.RequestCreatedEvent) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode Kafka event: %w", err)
	}
	record := &kgo.Record{Topic: topic, Key: []byte(event.RequestID.String()), Value: raw}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka event: %w", err)
	}
	return nil
}

func (p *Producer) Close() {
	p.client.Close()
}

type Consumer struct {
	client  *kgo.Client
	pending []*kgo.Record
}

func NewConsumer(brokers []string, topic, group string) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, errors.New("at least one Kafka broker is required")
	}
	if topic == "" || group == "" {
		return nil, errors.New("Kafka topic and group are required")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup(group),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &Consumer{client: client}, nil
}

func (c *Consumer) Poll(ctx context.Context) (*kgo.Record, error) {
	if record := c.popPending(); record != nil {
		return record, nil
	}

	fetches := c.client.PollFetches(ctx)
	if err := fetches.Err(); err != nil {
		return nil, err
	}
	fetches.EachRecord(func(current *kgo.Record) {
		c.pending = append(c.pending, current)
	})
	record := c.popPending()
	if record == nil {
		return nil, errors.New("Kafka poll returned no record")
	}
	return record, nil
}

func (c *Consumer) popPending() *kgo.Record {
	if len(c.pending) == 0 {
		return nil
	}
	record := c.pending[0]
	c.pending[0] = nil
	c.pending = c.pending[1:]
	return record
}

func (c *Consumer) Commit(ctx context.Context, record *kgo.Record) error {
	if err := c.client.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("commit Kafka record: %w", err)
	}
	return nil
}

func (c *Consumer) Close() {
	c.client.Close()
}
