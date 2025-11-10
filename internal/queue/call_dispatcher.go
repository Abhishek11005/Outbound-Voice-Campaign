package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	log.Printf("DEBUG: DispatchCall called for call %s to %s", msg.CallID, msg.PhoneNumber)
	value, err := json.Marshal(msg)
	if err != nil {
		log.Printf("DEBUG: Failed to marshal message: %v", err)
		return fmt.Errorf("call dispatcher: marshal message: %w", err)
	}

	record := kafka.Message{
		Key:   msg.CallID[:],
		Value: value,
		Time:  time.Now().UTC(),
	}

	log.Printf("DEBUG: Writing message to Kafka topic %s", d.writer.Stats().Topic)
	if err := d.writer.WriteMessages(ctx, record); err != nil {
		log.Printf("DEBUG: Failed to write message to Kafka: %v", err)
		return fmt.Errorf("call dispatcher: write message: %w", err)
	}
	log.Printf("DEBUG: Successfully dispatched call %s", msg.CallID)
	return nil
}

// Close closes the underlying writer.
func (d *CallDispatcher) Close() error {
	return d.writer.Close()
}
