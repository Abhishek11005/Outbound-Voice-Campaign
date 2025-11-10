package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/acme/outbound-call-campaign/internal/config"
)

// Kafka aggregates helpers for interacting with Kafka.
type Kafka struct {
	cfg config.KafkaConfig
}

// NewKafka initializes the Kafka helper.
func NewKafka(cfg config.KafkaConfig) (*Kafka, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: no brokers configured")
	}
	return &Kafka{cfg: cfg}, nil
}

// NewWriter creates a kafka writer for a specific topic.
func (k *Kafka) NewWriter(topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(k.cfg.Brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
}

// NewReader creates a kafka reader for a topic.
func (k *Kafka) NewReader(topic, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        k.cfg.Brokers,
		Topic:          topic,
		GroupID:        groupID,
		StartOffset:    kafka.FirstOffset,
		CommitInterval: k.cfg.CommitInterval,
		MinBytes:       1e3,
		MaxBytes:       10e6,
	})
}

// NewReaderWithConfig creates a kafka reader with a custom configuration.
func (k *Kafka) NewReaderWithConfig(config kafka.ReaderConfig) *kafka.Reader {
	return kafka.NewReader(config)
}

// Close is a no-op kept for interface symmetry.
func (k *Kafka) Close() error {
	return nil
}

// EnsureTopics creates topics if they do not exist.
func (k *Kafka) EnsureTopics(ctx context.Context, topics []string, partitions int, replicationFactor int) error {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, ClientID: k.cfg.ClientID}
	conn, err := dialer.DialContext(ctx, "tcp", k.cfg.Brokers[0])
	if err != nil {
		return fmt.Errorf("kafka: dial: %w", err)
	}
	defer conn.Close()

	existing, err := conn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("kafka: read partitions: %w", err)
	}
	exists := make(map[string]bool)
	for _, p := range existing {
		exists[p.Topic] = true
	}

	for _, topic := range topics {
		if exists[topic] {
			continue
		}
		if err := conn.CreateTopics(kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     partitions,
			ReplicationFactor: replicationFactor,
		}); err != nil {
			return fmt.Errorf("kafka: create topic %s: %w", topic, err)
		}
	}

	return nil
}
