# Outbound Voice Campaign Platform

Outbound Voice Campaign is a Go-based microservice platform for orchestrating large-scale outbound call campaigns with per-campaign scheduling, concurrency control, retries, and deep observability. The stack now runs **natively** (no Docker/Kubernetes required) and can be bootstrapped end-to-end with a single command.

## Architecture Overview

```
+----------------+        +----------------+        +------------------+
|    REST API    | <----> |   PostgreSQL   | <----> |   Scheduler Job  |
| (Fiber + OTel) |        | (Citus shard)  |        | (Business hours) |
+--------+-------+        +--+---------+---+        +---------+---------+
         |                   |         ^                       |
         v                   |         |                       v
  +------+---------+         |    +----+----+           +------+---------+
  | Kafka Dispatcher|  ----> |    |  Redis  | <---------| Retry Scheduler |
  +------+---------+         |    |Limiter  |           +------+---------+
         |                   |    +----+----+                  |
         v                   |         |                       v
+--------+------+            |         |                +------+-------+
| Call Worker   | -----------+         |                | Retry Worker |
| (Telephony)   | publishes status     |                +--------------+
+--------+------+            +---------v---------+
         |                   |   ScyllaDB        |
         v                   | (Call timeline)   |
+--------+------+            +------------------+
| Status Worker |
+---------------+
```

**Core services**

- **API Service** – CRUD for campaigns, target ingestion, ad-hoc call triggering, campaign statistics.
- **Scheduler** – Enforces business-hour windows and feeds targets into Kafka respecting campaign limits.
- **Call Worker** – Consumes dispatch events, executes the mock telephony provider, emits status events, honours Redis-based concurrency limits.
- **Status Worker** – Persists call outcomes to ScyllaDB, updates aggregates, and schedules retries when required.
- **Retry Worker** – Drains per-attempt retry topics and re-queues calls after backoff.
- **PostgreSQL (Citus)** – Campaign metadata, targets, statistics, events.
- **ScyllaDB / Cassandra** – High-volume call history and attempt timelines.
- **Kafka + Zookeeper** – Back-pressure tolerant pipeline for dispatching, statuses, and retries.
- **Redis** – Distributed semaphore for per-campaign concurrency throttling.
- **OpenTelemetry Collector + Jaeger** – Distributed tracing pipeline and UI.

## Quick Start (Single Command)

The native toolchain relies on Homebrew (macOS) or APT/YUM (Linux) to install system services. Corporate proxy issues are avoided by running everything directly on the host.

```bash
# Install dependencies (PostgreSQL+Citus, Scylla/Cassandra, Kafka, Redis, Jaeger, OTel Collector, Go tools)
make setup

# Provision databases, keyspaces, and Kafka topics after services are running
make init

# Launch the entire stack (Kafka, Redis, Jaeger, OTEL, API, workers, scheduler)
make start
```

- `make setup` runs `scripts/install-all.sh` and may prompt for `sudo` when installing packages.
- `make init` waits for services to be reachable and applies all database/schema migrations.
- `make start` invokes `scripts/run-all.sh`, which ensures stateful services are up (via Homebrew/systemd) and then starts Kafka, Redis, Jaeger, OTEL, and every Go service via `goreman`.

To stop the stack:

```bash
make stop
```

For hot-reload development of Go services, run:

```bash
make start-dev   # uses Procfile.dev + air
```

## What Gets Installed

| Component | Purpose | Access |
|-----------|---------|--------|
| PostgreSQL 14 (+ Citus*) | Primary relational store | `localhost:5432` |
| ScyllaDB / Cassandra | Call attempt history | `localhost:9042` |
| Confluent Kafka + Zookeeper | Message backbone | `localhost:9092` / `localhost:2181` |
| Redis 7 | Concurrency limiter | `localhost:6379` |
| OpenTelemetry Collector | Trace/metric ingestion | `http://localhost:4318` |
| Jaeger All-in-One | Distributed tracing UI | `http://localhost:16686` |
| Goreman | Process orchestration | N/A |
| Air | Go hot-reload | N/A |

Services started by `make start`:

- API server on `http://localhost:8080`
- Scheduler, Call Worker, Status Worker, Retry Worker
- Kafka broker, Zookeeper, Redis, Jaeger, OpenTelemetry Collector

**Note:** PostgreSQL, ScyllaDB/Cassandra, and Redis run as native services (Homebrew or systemd). The helper scripts attempt to start them automatically; feel free to manage them manually if you prefer.

*Citus extension is optional for local development. If you need distributed PostgreSQL features, install Citus manually after setup.

## Environment Configuration

Environment variables are loaded from `.env.local` if present. Copy the sample file and tweak credentials as needed:

```
cp env/local.env.example .env.local
```

For production pipelines, seed a secrets manager or CI/CD environment with the values from `env/production.env.example`.

`env/local.env.example` contains sensible defaults:

```
POSTGRES_HOST=localhost
POSTGRES_SUPERUSER=postgres
POSTGRES_SUPERUSER_PASSWORD=
POSTGRES_APP_USER=campaign
POSTGRES_APP_PASSWORD=campaign
POSTGRES_APP_DB=campaign
SCYLLA_HOST=localhost
SCYLLA_PORT=9042
SCYLLA_KEYSPACE=campaign
KAFKA_HOST=localhost
KAFKA_PORT=9092
REDIS_HOST=localhost
REDIS_PORT=6379
CONFIG_FILE=./configs/config.yaml
```

Provide `POSTGRES_SUPERUSER` / `POSTGRES_SUPERUSER_PASSWORD` if your local PostgreSQL uses password authentication or runs under a different OS user (e.g., Homebrew defaults to your shell user).

## Scripts & Tooling

| Script | Purpose |
|--------|---------|
| `scripts/install-all.sh` | Installs system dependencies, downloads binaries, and prepares data directories. |
| `scripts/init-db.sh` | Creates PostgreSQL role/database, enables Citus, creates Scylla keyspace, and provisions all Kafka topics. |
| `scripts/run-all.sh` | Ensures stateful services are online, then launches the full stack with Goreman. |
| `scripts/stop-all.sh` | Gracefully stops Goreman-managed processes. |
| `scripts/tail-logs.sh` | Tails Redis/Kafka/Jaeger logs for quick debugging. |

All scripts are safe to rerun and guard against clobbering existing configuration.

## Development Workflow

1. **Install & initialise:** `make setup && make init`
2. **Run the stack:** `make start`
3. **Tail logs:** `make logs`
4. **Iterate:** edit code, rebuild (`make build-all`) or rely on `make start-dev` for hot reload.
5. **Run tests:** `make test`
6. **Stop services:** `make stop`
7. **Clean artifacts:** `make clean`

## Manual Service Checks

- PostgreSQL readiness: `pg_isready -h localhost -p 5432`
- Scylla/Cassandra meter: `cqlsh localhost 9042 -e 'DESCRIBE KEYSPACE campaign'`
- Kafka topics: `$HOME/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092`
- Redis ping: `redis-cli ping`
- Jaeger UI: `http://localhost:16686`

## Updating Configuration

Service-specific configuration lives under `config/`:

- `config/postgres.conf` – optional overrides for PostgreSQL
- `config/scylla.yaml` – ScyllaDB tuning for local development
- `config/kafka-server.properties` / `config/zookeeper.properties`
- `config/redis.conf`

The Go application configuration is at `configs/config.yaml`, now targeting `localhost` hosts and ports.

## Make Targets

```
make setup       # install native dependencies
make init        # run database/schema initialisation
make start       # start full stack via Goreman
make start-dev   # start stack with hot reload
make stop        # stop Goreman-managed processes
make build-all   # build all service binaries
make run SERVICE=api   # run a single service
make logs        # tail infrastructure logs
make clean       # remove build artifacts and local data
make tidy        # go mod tidy
make test        # go test ./...
```

## Testing

```bash
go test ./...
```

Integration tests can leverage the native stack started by `make start`.

## Observability

- OpenTelemetry Collector listens on `http://localhost:4318`
- Jaeger UI available at `http://localhost:16686`
- Trace IDs propagate across API, workers, scheduler, and repository layers via `internal/telemetry` helpers.

## Troubleshooting

- **Ports already in use:** Stop existing services (PostgreSQL, Redis, Kafka) or adjust ports in the config files.
- **PostgreSQL authentication failures:** Set `POSTGRES_SUPERUSER`/`POSTGRES_SUPERUSER_PASSWORD` in `.env.local` to match your local setup.
- **Kafka fails to start:** Ensure no previous Kafka processes are running; delete `data/kafka-logs` and retry `make start`. Confluent Kafka is used instead of Apache Kafka for better stability.
- **Corporate proxy restrictions:** The script disables proxy settings; if you still have issues, check for proxy config in `~/.gitconfig`, `~/.curlrc`, or environment variables.
- **Git authentication errors:** Homebrew may try to clone GitHub repos. If you see Git auth prompts, check your Git config: `git config --global --unset http.proxy` and `git config --global --unset https.proxy`.
- **Citus extension missing:** On macOS, Citus is optional for local development. The database will work without it.

## Repository Layout

```
cmd/                Service entrypoints (api, workers, scheduler)
internal/api        HTTP handlers and server wiring
internal/app        Dependency wiring & lifecycle management
internal/domain     Core domain models
internal/repository PostgreSQL & Scylla data access layers
internal/service    Domain services (campaign, call, concurrency)
internal/worker     Background workers (call, status, retry)
internal/scheduler  Business hour scheduler
internal/telemetry  OpenTelemetry bootstrap helpers
pkg/                Shared logger and error helpers
configs/            Application + OTEL collector configuration
config/             Native infrastructure configuration files
scripts/            Automation scripts for install/run/stop/logs
db/migrations       SQL/CQL schema definitions
```

Happy dialing! 🎧

## Production Readiness & Scaling Guidelines

The platform ships with a production-tuned configuration (`configs/config.production.yaml`) aimed at sustaining **50k QPS / billions of calls per day**. Key expectations:

- **Kafka throughput**: Dispatch/status/retry topics are created with 48 partitions by default. Adjust `DISPATCH_PARTITIONS`, `STATUS_PARTITIONS`, `RETRY_PARTITIONS`, or `DEADLETTER_PARTITIONS` environment variables before running `make init` if you need different counts. For redundancy, use a replication factor ≥3 in production Kafka clusters.
- **Database capacity**: The production config sets the PostgreSQL pool to 800 connections (200 warm) and enables Citus for horizontal sharding. size worker pools across nodes to ensure each shard stays <65% utilisation. Scylla/Cassandra is configured for `LOCAL_QUORUM` consistency across three nodes.
- **Redis concurrency control**: Redis pool sizing (512 connections, 128 idle) supports high-volume limiter operations. Increase `global_concurrency` and campaign-level defaults in the throttle section if traffic profiles demand it.
- **Scheduler fan-out**: `worker_count: 128` and `max_batch_size: 5000` keep the scheduler ahead of demand. Scale worker instances horizontally—Kafka partitions guarantee work distribution across pods/processes.
- **Service fleets**: Run multiple replicas of API, call/status/retry workers, and scheduler. All services are stateless once configured and support interface-driven dependency injection for clean scaling in containers or orchestrated environments.
- **Observability and resiliency**: OpenTelemetry is enabled end-to-end. Feed traces/metrics into your existing collector stack, enforce circuit breakers at the telephony provider boundary, and prefer mTLS between internal services.

For production builds:

```bash
make build-all
ENV_FILE=env/production.env.example CONFIG_FILE=./configs/config.production.yaml make start
```

Ensure infrastructure automation (Terraform/Ansible/Kubernetes) provisions managed equivalents for PostgreSQL/Citus, Scylla, Kafka, and Redis with the capacities listed above.

## API Smoke Tests

Once `make start` reports all services healthy, you can validate the stack end-to-end with these ready-to-run `curl` commands. Replace placeholder IDs as indicated.

1. **Create a campaign**

```bash
curl -X POST http://localhost:8080/api/v1/campaigns \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "test-campaign",
    "description": "Sample outbound campaign",
    "time_zone": "America/New_York",
    "max_concurrent_calls": 10,
    "retry_policy": {
      "max_attempts": 3,
      "base_delay": "2s",
      "max_delay": "30s",
      "jitter": 0.2
    },
    "business_hours": [{"day_of_week": 1, "start": "09:00", "end": "18:00"}],
    "targets": [
      {"phone_number": "+15555550001"},
      {"phone_number": "+15555550002"}
    ]
  }'
```

Capture the returned `id` for subsequent requests.

2. **Start the campaign**

```bash
CAMPAIGN_ID=<campaign-id-from-create>
curl -X POST http://localhost:8080/api/v1/campaigns/${CAMPAIGN_ID}/start
```

3. **Inspect a campaign**

```bash
curl http://localhost:8080/api/v1/campaigns/${CAMPAIGN_ID} | jq
```

4. **Trigger a manual call**

```bash
curl -X POST http://localhost:8080/api/v1/calls \
  -H 'Content-Type: application/json' \
  -d '{"phone_number": "+15555550055"}'
```

5. **Fetch campaign statistics**

```bash
curl http://localhost:8080/api/v1/campaigns/${CAMPAIGN_ID}/stats | jq
```

6. **List recent calls**

```bash
curl http://localhost:8080/api/v1/calls | jq
```

All commands assume the default configuration in `configs/config.yaml`. Adjust host/port or payload values as needed for your environment.
