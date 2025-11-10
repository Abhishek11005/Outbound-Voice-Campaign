# Infrastructure Services
zookeeper: $HOME/confluent/bin/zookeeper-server-start config/zookeeper.properties
kafka: sleep 10 && $HOME/confluent/bin/kafka-server-start config/server.properties
redis: redis-server config/redis.conf
jaeger: $HOME/jaeger/jaeger-all-in-one --collector.zipkin.http-port=9411
otel: $HOME/otel-collector/otelcol-contrib --config=configs/otel-collector-config.yaml

# Application Services
api: go run ./cmd/api --config ${CONFIG_FILE:-configs/config.yaml}
callworker: go run ./cmd/callworker --config ${CONFIG_FILE:-configs/config.yaml}
statusworker: go run ./cmd/statusworker --config ${CONFIG_FILE:-configs/config.yaml}
retryworker: go run ./cmd/retryworker --config ${CONFIG_FILE:-configs/config.yaml}
scheduler: go run ./cmd/scheduler --config ${CONFIG_FILE:-configs/config.yaml}
