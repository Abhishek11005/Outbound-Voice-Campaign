package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// CallDispatcher publishes call dispatch events to Kafka.
type CallDispatcher struct {
	writer *kafka.Writer
}

// NewCallDispatcher constructs a dispatcher for the given topic.
func NewCallDispatcher(k *Kafka, topic string) *CallDispatcher {
	return &CallDispatcher{
		writer: k.NewWriter(topic),
	}
}

// DispatchCall writes the dispatch message to Kafka.
func (d *CallDispatcher) DispatchCall(ctx context.Context, msg DispatchMessage) error {
	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("call dispatcher: marshal message: %w", err)
	}

	record := kafka.Message{
		Key:   msg.CallID[:],
		Value: value,
		Time:  time.Now().UTC(),
	}

	if err := d.writer.WriteMessages(ctx, record); err != nil {
		return fmt.Errorf("call dispatcher: write message: %w", err)
	}
	return nil
}

// Close closes the underlying writer.
func (d *CallDispatcher) Close() error {
	return d.writer.Close()
}
