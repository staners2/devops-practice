package kafkabus

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestNewProducerRejectsEmptyBrokers(t *testing.T) {
	_, err := NewProducer(nil)
	if err == nil {
		t.Fatal("NewProducer() error = nil, want empty brokers error")
	}
}

func TestConsumerReturnsBufferedRecordsInOrder(t *testing.T) {
	first := &kgo.Record{Offset: 41}
	second := &kgo.Record{Offset: 42}
	consumer := &Consumer{pending: []*kgo.Record{first, second}}

	if got := consumer.popPending(); got != first {
		t.Fatalf("first popPending() = %v, want offset %d", got, first.Offset)
	}
	if got := consumer.popPending(); got != second {
		t.Fatalf("second popPending() = %v, want offset %d", got, second.Offset)
	}
	if got := consumer.popPending(); got != nil {
		t.Fatalf("third popPending() = %v, want nil", got)
	}
}
