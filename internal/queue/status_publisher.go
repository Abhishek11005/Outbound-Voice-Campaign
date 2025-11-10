package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// StatusPublisher publishes call status events.
type StatusPublisher struct {
	writer *kafka.Writer
}

// NewStatusPublisher constructs a status publisher for the given topic.
func NewStatusPublisher(k *Kafka, topic string) *StatusPublisher {
	return &StatusPublisher{writer: k.NewWriter(topic)}
}

// PublishStatus emits a status message to Kafka.
func (p *StatusPublisher) PublishStatus(ctx context.Context, msg StatusMessage) error {
	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("status publisher: marshal message: %w", err)
	}
	record := kafka.Message{
		Key:   msg.CallID[:],
		Value: value,
		Time:  time.Now().UTC(),
	}
	if err := p.writer.WriteMessages(ctx, record); err != nil {
		return fmt.Errorf("status publisher: write message: %w", err)
	}
	return nil
}

// Close closes the publisher.
func (p *StatusPublisher) Close() error {
	return p.writer.Close()
}
