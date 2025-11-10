package retry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/queue"
)

// Worker handles retry scheduling for failed calls.
type Worker struct {
	container *app.Container
}

// New creates a retry worker instance.
func New(container *app.Container) *Worker {
	return &Worker{container: container}
}

// Run waits for cancellation. Logic will be implemented later.
func (w *Worker) Run(ctx context.Context) error {
	cfg := w.container.Config
	if len(cfg.Kafka.RetryTopics) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(cfg.Kafka.RetryTopics))
	var wg sync.WaitGroup

	for idx, topic := range cfg.Kafka.RetryTopics {
		wg.Add(1)
		go func(topic string, attemptIndex int) {
			defer wg.Done()
			if err := w.consumeTopic(ctx, topic, attemptIndex); err != nil && ctx.Err() == nil {
				errCh <- err
			}
		}(topic, idx+1)
	}

	select {
	case <-ctx.Done():
		wg.Wait()
		return ctx.Err()
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
}

func (w *Worker) consumeTopic(ctx context.Context, topic string, attemptIndex int) error {
	cfg := w.container.Config
	groupID := cfg.Kafka.RetryConsumerGroupID
	if groupID == "" {
		groupID = fmt.Sprintf("%s-retry-%d", cfg.Kafka.ConsumerGroupID, attemptIndex)
	} else {
		groupID = fmt.Sprintf("%s-%d", groupID, attemptIndex)
	}

	reader := w.container.Kafka.NewReader(topic, groupID)
	defer reader.Close()

	dispatcher := w.container.Dispatchers().CallDispatcher
	logger := w.container.Logger

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("retry worker: fetch", zap.Error(err))
			continue
		}

		var retryMsg queue.RetryMessage
		if err := json.Unmarshal(msg.Value, &retryMsg); err != nil {
			logger.Error("retry worker: unmarshal", zap.Error(err))
			_ = reader.CommitMessages(ctx, msg)
			continue
		}

		tracer := otel.Tracer("outbound.retryworker")
		sctx, span := tracer.Start(ctx, "retry.dispatch", trace.WithAttributes(
			attribute.String("call.id", retryMsg.CallID.String()),
			attribute.String("campaign.id", retryMsg.CampaignID.String()),
			attribute.Int("attempt", retryMsg.DispatchMessage.Attempt),
		))
		defer span.End()

		if sleepErr := w.sleepUntil(sctx, retryMsg.NextAttempt); sleepErr != nil {
			span.RecordError(sleepErr)
			logger.Error("retry worker: wait", zap.Error(sleepErr))
			_ = reader.CommitMessages(sctx, msg)
			continue
		}

		dispatch := retryMsg.DispatchMessage
		dispatch.EnqueuedAt = time.Now().UTC()

		if err := dispatcher.DispatchCall(sctx, dispatch); err != nil {
			span.RecordError(err)
			logger.Error("retry worker: dispatch", zap.Error(err))
			continue
		}

		if err := reader.CommitMessages(sctx, msg); err != nil {
			span.RecordError(err)
			logger.Error("retry worker: commit", zap.Error(err))
		}
	}
}

func (w *Worker) sleepUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
