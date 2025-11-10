package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config captures the full configuration surface for the application.
type Config struct {
	App        AppConfig        `mapstructure:"app"`
	HTTP       HTTPConfig       `mapstructure:"http"`
	Postgres   PostgresConfig   `mapstructure:"postgres"`
	Scylla     ScyllaConfig     `mapstructure:"scylla"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Telemetry  TelemetryConfig  `mapstructure:"telemetry"`
	Scheduler  SchedulerConfig  `mapstructure:"scheduler"`
	Retry      RetryConfig      `mapstructure:"retry"`
	Throttle   ThrottleConfig   `mapstructure:"throttle"`
	CallBridge CallBridgeConfig `mapstructure:"call_bridge"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Env     string `mapstructure:"env"`
	Version string `mapstructure:"version"`
}

type HTTPConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type PostgresConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
	HealthQuery     string        `mapstructure:"health_query"`
}

type ScyllaConfig struct {
	Hosts             []string      `mapstructure:"hosts"`
	Port              int           `mapstructure:"port"`
	Keyspace          string        `mapstructure:"keyspace"`
	Consistency       string        `mapstructure:"consistency"`
	Timeout           time.Duration `mapstructure:"timeout"`
	DisableInitSchema bool          `mapstructure:"disable_init_schema"`
}

type KafkaConfig struct {
	Brokers              []string      `mapstructure:"brokers"`
	ClientID             string        `mapstructure:"client_id"`
	CallTopic            string        `mapstructure:"call_topic"`
	StatusTopic          string        `mapstructure:"status_topic"`
	RetryTopics          []string      `mapstructure:"retry_topics"`
	DeadLetterTopic      string        `mapstructure:"dead_letter_topic"`
	ConsumerGroupID      string        `mapstructure:"consumer_group_id"`
	RetryConsumerGroupID string        `mapstructure:"retry_consumer_group_id"`
	CommitInterval       time.Duration `mapstructure:"commit_interval"`
}

type RedisConfig struct {
	Address      string        `mapstructure:"address"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns"`
	MaxRetries   int           `mapstructure:"max_retries"`
}

type TelemetryConfig struct {
	Endpoint          string        `mapstructure:"endpoint"`
	ServiceName       string        `mapstructure:"service_name"`
	SampleRatio       float64       `mapstructure:"sample_ratio"`
	MetricsInterval   time.Duration `mapstructure:"metrics_interval"`
	MetricsEnabled    bool          `mapstructure:"metrics_enabled"`
	TracingEnabled    bool          `mapstructure:"tracing_enabled"`
	Propagators       []string      `mapstructure:"propagators"`
	ShutdownTimeout   time.Duration `mapstructure:"shutdown_timeout"`
	CollectorProtocol string        `mapstructure:"collector_protocol"`
}

type SchedulerConfig struct {
	TickInterval  time.Duration `mapstructure:"tick_interval"`
	LookAhead     time.Duration `mapstructure:"look_ahead"`
	MaxBatchSize  int           `mapstructure:"max_batch_size"`
	WorkerCount   int           `mapstructure:"worker_count"`
	LockTTL       time.Duration `mapstructure:"lock_ttl"`
	LockKeyPrefix string        `mapstructure:"lock_key_prefix"`
}

type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts"`
	BaseDelay   time.Duration `mapstructure:"base_delay"`
	MaxDelay    time.Duration `mapstructure:"max_delay"`
	Jitter      float64       `mapstructure:"jitter"`
}

type ThrottleConfig struct {
	GlobalConcurrency int `mapstructure:"global_concurrency"`
	DefaultPerCampaign int `mapstructure:"default_per_campaign"`
}

type CallBridgeConfig struct {
	ProviderName string        `mapstructure:"provider_name"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
}

// Load reads configuration from file and environment variables.
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvPrefix("OUTBOUND")
	v.SetEnvKeyReplacer(NewEnvReplacer())

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: failed to read config file: %w", err)
	}

	cfg := new(Config)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("config: failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// NewEnvReplacer standardizes environment variable names.
func NewEnvReplacer() *strings.Replacer {
	return strings.NewReplacer(".", "_", "-", "_")
}
