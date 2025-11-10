package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// RetryScheduler publishes retry instructions to dedicated topics.
type RetryScheduler struct {
	writers []*kafka.Writer
}

// NewRetryScheduler constructs a scheduler from configured retry topics.
func NewRetryScheduler(k *Kafka, topics []string) *RetryScheduler {
	writers := make([]*kafka.Writer, 0, len(topics))
	for _, topic := range topics {
		writers = append(writers, k.NewWriter(topic))
	}
	return &RetryScheduler{writers: writers}
}

// ScheduleRetry publishes the message to the retry topic associated with the attempt index (1-based).
func (r *RetryScheduler) ScheduleRetry(ctx context.Context, attempt int, msg RetryMessage) error {
	if attempt <= 0 || attempt > len(r.writers) {
		return fmt.Errorf("retry scheduler: attempt %d out of range", attempt)
	}

	value, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("retry scheduler: marshal message: %w", err)
	}

	record := kafka.Message{
		Key:   msg.CallID[:],
		Value: value,
		Time:  time.Now().UTC(),
	}

	if err := r.writers[attempt-1].WriteMessages(ctx, record); err != nil {
		return fmt.Errorf("retry scheduler: write: %w", err)
	}
	return nil
}

// Close closes all writers.
func (r *RetryScheduler) Close() error {
	var err error
	for _, w := range r.writers {
		if w == nil {
			continue
		}
		if cerr := w.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
