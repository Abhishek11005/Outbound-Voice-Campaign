package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/acme/outbound-call-campaign/internal/api/handlers"
	"github.com/acme/outbound-call-campaign/internal/config"
	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/infra/db"
	"github.com/acme/outbound-call-campaign/internal/infra/redis"
	"github.com/acme/outbound-call-campaign/internal/queue"
	"github.com/acme/outbound-call-campaign/internal/repository"
	pgrepo "github.com/acme/outbound-call-campaign/internal/repository/postgres"
	scyllarepo "github.com/acme/outbound-call-campaign/internal/repository/scylla"
	campaignsvc "github.com/acme/outbound-call-campaign/internal/service/campaign"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
	"github.com/acme/outbound-call-campaign/internal/service/concurrency"
	telephonySvc "github.com/acme/outbound-call-campaign/internal/telephony"
	telephonyMock "github.com/acme/outbound-call-campaign/internal/telephony/mock"
	"github.com/acme/outbound-call-campaign/pkg/logger"
)

// Container wires together shared infrastructure dependencies.
type Container struct {
	Config *config.Config
	Logger *logger.Logger

	Postgres *db.Postgres
	Scylla   *db.Scylla
	Redis    *redis.Client
	Kafka    *queue.Kafka

	// lazily initialised components
	components struct {
		once         sync.Once
		repositories *repositories
		services    *services
		dispatchers *dispatchers
		providers   *providers
		limiters    *limiters
	}
}

type repositories struct {
	Campaign      repository.CampaignRepository
	BusinessHours repository.BusinessHourRepository
	Targets       repository.CampaignTargetRepository
	Stats         repository.CampaignStatisticsRepository
	CallStore     repository.CallStore
}

type services struct {
	Campaign *campaignsvc.Service
	Call     *callsvc.Service
}

type dispatchers struct {
	CallDispatcher   *queue.CallDispatcher
	StatusPublisher  *queue.StatusPublisher
	RetryScheduler   *queue.RetryScheduler
}

type providers struct {
	Telephony telephonySvc.Provider
}

type limiters struct {
	Concurrency *concurrency.Limiter
}

// Build constructs a container for the given configuration path.
func Build(ctx context.Context, configPath string) (*Container, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	lg, err := logger.New(cfg.App.Env)
	if err != nil {
		return nil, err
	}

	pg, err := db.NewPostgres(ctx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("bootstrap postgres: %w", err)
	}

	scylla, err := db.NewScylla(cfg.Scylla)
	if err != nil {
		return nil, fmt.Errorf("bootstrap scylla: %w", err)
	}

	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("bootstrap redis: %w", err)
	}

	kafka, err := queue.NewKafka(cfg.Kafka)
	if err != nil {
		return nil, fmt.Errorf("bootstrap kafka: %w", err)
	}

	container := &Container{
		Config:   cfg,
		Logger:   lg,
		Postgres: pg,
		Scylla:   scylla,
		Redis:    redisClient,
		Kafka:    kafka,
	}

	return container, nil
}

func (c *Container) initComponents() {
	c.components.once.Do(func() {
		repos := &repositories{
			Campaign:      pgrepo.NewCampaignRepository(c.Postgres.DB()),
			BusinessHours: pgrepo.NewBusinessHourRepository(c.Postgres.DB()),
			Targets:       pgrepo.NewCampaignTargetRepository(c.Postgres.DB()),
			Stats:         pgrepo.NewCampaignStatisticsRepository(c.Postgres.DB()),
			CallStore:     scyllarepo.NewCallStore(c.Scylla.Session()),
		}

		disp := &dispatchers{
			CallDispatcher:  queue.NewCallDispatcher(c.Kafka, c.Config.Kafka.CallTopic),
			StatusPublisher: queue.NewStatusPublisher(c.Kafka, c.Config.Kafka.StatusTopic),
			RetryScheduler:  queue.NewRetryScheduler(c.Kafka, c.Config.Kafka.RetryTopics),
		}

		services := &services{
			Campaign: campaignsvc.NewService(
				repos.Campaign,
				repos.BusinessHours,
				repos.Targets,
				repos.Stats,
				c.Config.Throttle.DefaultPerCampaign,
			),
		}

		defaultRetry := domain.RetryPolicy{
			MaxAttempts: c.Config.Retry.MaxAttempts,
			BaseDelay:   c.Config.Retry.BaseDelay,
			MaxDelay:    c.Config.Retry.MaxDelay,
			Jitter:      c.Config.Retry.Jitter,
		}

		services.Call = callsvc.NewService(
			repos.CallStore,
			repos.Campaign,
			repos.Stats,
			disp.CallDispatcher,
			defaultRetry,
			c.Config.Throttle.DefaultPerCampaign,
		)

		providers := &providers{
			Telephony: telephonyMock.NewProvider(c.Config.CallBridge),
		}

		limiters := &limiters{
			Concurrency: concurrency.NewLimiter(c.Redis.Inner(), c.Config.Throttle.DefaultPerCampaign, c.Config.Scheduler.LockTTL),
		}

		c.components.repositories = repos
		c.components.dispatchers = disp
		c.components.services = services
		c.components.providers = providers
		c.components.limiters = limiters
	})
}

// Repositories exposes initialized repositories.
func (c *Container) Repositories() *repositories {
	c.initComponents()
	return c.components.repositories
}

// Services exposes initialized services.
func (c *Container) Services() *services {
	c.initComponents()
	return c.components.services
}

// Dispatchers exposes Kafka dispatchers.
func (c *Container) Dispatchers() *dispatchers {
	c.initComponents()
	return c.components.dispatchers
}

// Providers exposes external providers.
func (c *Container) Providers() *providers {
	c.initComponents()
	return c.components.providers
}

// Limiters exposes limiter utilities.
func (c *Container) Limiters() *limiters {
	c.initComponents()
	return c.components.limiters
}

// HandlerSet builds HTTP handlers with dependencies.
func (c *Container) HandlerSet() *handlers.HandlerSet {
	return handlers.NewHandlerSet(c)
}

// Close releases all held resources.
func (c *Container) Close(ctx context.Context) error {
	var errs []error
	if c.components.dispatchers != nil {
		if d := c.components.dispatchers;
			d != nil {
			if d.CallDispatcher != nil {
				if err := d.CallDispatcher.Close(); err != nil {
					errs = append(errs, fmt.Errorf("dispatcher close: %w", err))
				}
			}
			if d.StatusPublisher != nil {
				if err := d.StatusPublisher.Close(); err != nil {
					errs = append(errs, fmt.Errorf("status publisher close: %w", err))
				}
			}
			if d.RetryScheduler != nil {
				if err := d.RetryScheduler.Close(); err != nil {
					errs = append(errs, fmt.Errorf("retry scheduler close: %w", err))
				}
			}
		}
	}
	if c.Kafka != nil {
		if err := c.Kafka.Close(); err != nil {
			errs = append(errs, fmt.Errorf("kafka close: %w", err))
		}
	}
	if c.Redis != nil {
		if err := c.Redis.Close(); err != nil {
			errs = append(errs, fmt.Errorf("redis close: %w", err))
		}
	}
	if c.Scylla != nil {
		if err := c.Scylla.Close(); err != nil {
			errs = append(errs, fmt.Errorf("scylla close: %w", err))
		}
	}
	if c.Postgres != nil {
		if err := c.Postgres.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("postgres close: %w", err))
		}
	}
	if c.Logger != nil {
		c.Logger.Sync()
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// EnsureTopics ensures required Kafka topics exist.
func (c *Container) EnsureTopics(ctx context.Context) error {
	c.initComponents()

	topics := []string{c.Config.Kafka.CallTopic, c.Config.Kafka.StatusTopic}
	if err := c.Kafka.EnsureTopics(ctx, topics, 48, 1); err != nil {
		return err
	}

	if len(c.Config.Kafka.RetryTopics) > 0 {
		if err := c.Kafka.EnsureTopics(ctx, c.Config.Kafka.RetryTopics, 48, 1); err != nil {
			return err
		}
	}

	if c.Config.Kafka.DeadLetterTopic != "" {
		if err := c.Kafka.EnsureTopics(ctx, []string{c.Config.Kafka.DeadLetterTopic}, 12, 1); err != nil {
			return err
		}
	}

	return nil
}
