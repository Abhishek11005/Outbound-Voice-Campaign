# Infrastructure Services
redis: redis-server config/redis.conf
# zookeeper and kafka are now managed via Docker (see scripts/init-db.sh)
# jaeger: $HOME/jaeger/jaeger-all-in-one --collector.zipkin.http-port=9411
# otel: $HOME/otel-collector/otelcol-contrib --config=configs/otel-collector-config.yaml

# Application Services
api: go run ./cmd/api --config ${CONFIG_FILE:-configs/config.yaml}
callworker: go run ./cmd/callworker --config ${CONFIG_FILE:-configs/config.yaml}
statusworker: go run ./cmd/statusworker --config ${CONFIG_FILE:-configs/config.yaml}
retryworker: go run ./cmd/retryworker --config ${CONFIG_FILE:-configs/config.yaml}
scheduler: go run ./cmd/scheduler --config ${CONFIG_FILE:-configs/config.yaml}
