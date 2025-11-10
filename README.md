# Outbound Voice Campaign Platform

Outbound Voice Campaign is a Go-based microservice platform for orchestrating large-scale outbound call campaigns with per-campaign scheduling, concurrency control, retries, and deep observability. The stack now runs **natively** (no Docker/Kubernetes required) and can be bootstrapped end-to-end with a single command.

For detailed architecture documentation, see [ARCHITECTURE.md](./ARCHITECTURE.md).

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

## System Design Details

- **API & Persistence** – REST endpoints (Fiber) validate payloads, normalise retry/business-hour options, and persist campaign state + targets to PostgreSQL (optionally sharded via Citus). All calls are associated with campaigns to leverage business hour scheduling, concurrency control, and retry policies.
- **Direct Call Creation** – Individual calls can be triggered via `POST /api/v1/calls` but must specify a campaign_id and use phone numbers from the campaign's registered target list.
- **Target Validation** – Campaigns must be registered with their complete target phone number list. The `/campaigns/{id}/targets` endpoint only accepts phone numbers that were part of the original campaign registration, ensuring strict campaign boundaries.
- **Scheduler Loop** – Periodically scans in-progress campaigns, evaluates timezone-aware business-hour windows, and only dispatches work inside permitted windows. Targets are fetched in batches and scheduled for execution.
- **Dispatch Pipeline** – Kafka decouples scheduling from execution. Call workers acquire per-campaign capacity through Redis-backed Lua scripts before invoking the telephony provider, guaranteeing configurable concurrency limits per campaign.
- **Status & Retry Flow** – Worker callbacks write detailed attempt histories to ScyllaDB and adjust aggregates in PostgreSQL. Retryable failures are re-queued with exponential backoff and decorrelated jitter governed by each campaign's `RetryPolicy`.
- **Fault Tolerance & Observability** – Multiple replicas of every worker share Kafka partitions for horizontal scale. Redis operations are atomic, and OpenTelemetry spans connect API handlers, repositories, and background workers for rapid diagnosis.
- **Business Hour Encoding** – Windows are expressed as `{ "day_of_week": 1, "start": "09:00", "end": "18:00" }` (Monday). Provide multiple entries per day if needed; omitting `business_hours` defaults to 24×7 dialling.

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

- API server on `http://localhost:8081`
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
make load-test   # run default load test (3 campaigns, 50 calls)
make load-test CAMPAIGNS=10 CALLS=500 CONCURRENT=50  # custom load test
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
curl -X POST http://localhost:8081/api/v1/campaigns \
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

2. **Update campaign configuration**

```bash
CAMPAIGN_ID=<campaign-id-from-create>
curl -X PUT http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID} \
  -H 'Content-Type: application/json' \
  -d '{
    "description": "Evening calling window",
    "max_concurrent_calls": 25,
    "business_hours": [
      {"day_of_week": 1, "start": "13:00", "end": "21:00"},
      {"day_of_week": 2, "start": "13:00", "end": "21:00"}
    ],
    "retry_policy": {
      "max_attempts": 4,
      "base_delay": "3s",
      "max_delay": "45s",
      "jitter": 0.3
    }
  }'
```

3. **Start (or resume) the campaign**

```bash
CAMPAIGN_ID=<campaign-id-from-create>
curl -X POST http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/start
```

4. **Pause the campaign**

```bash
curl -X POST http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/pause
```

5. **Inspect a campaign**

```bash
curl http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID} | jq
```

6. **Trigger an individual call (campaign-based)**

```bash
# Note: phone_number must be part of the campaign's registered target list
curl -X POST http://localhost:8081/api/v1/calls \
  -H 'Content-Type: application/json' \
  -d '{
    "campaign_id": "'${CAMPAIGN_ID}'",
    "phone_number": "+15555550001",
    "metadata": {"priority": "high"}
  }'
```

7. **Fetch campaign statistics**

```bash
curl http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/stats | jq
```

8. **List calls for a campaign**

```bash
curl http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/calls | jq
```

9. **Get a specific call**

```bash
# First get a call ID from the campaign calls list
CALL_ID=$(curl -s http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/calls | jq -r '.calls[0].id')
curl http://localhost:8081/api/v1/calls/${CALL_ID} | jq
```

10. **Complete a campaign**

```bash
CAMPAIGN_ID=<campaign-id-from-create>
curl -X POST http://localhost:8081/api/v1/campaigns/${CAMPAIGN_ID}/complete
```

**Note:** All calls are automatically associated with campaigns. Calls can be created in two ways: 1) Automatically by the scheduler processing campaign targets, or 2) Directly via the API using `POST /api/v1/calls` with a specified `campaign_id` and phone number from the campaign's registered targets. Calls are accessed through their parent campaigns using `GET /api/v1/campaigns/{campaign-id}/calls` or individually via `GET /api/v1/calls/{call-id}`.

## Complete API Reference

### Health Check
- `GET /healthz` - Service health check with database connectivity status

### Campaigns API
- `POST /api/v1/campaigns` - Create a new campaign
- `GET /api/v1/campaigns` - List all campaigns
- `GET /api/v1/campaigns/{id}` - Get campaign details
- `PUT /api/v1/campaigns/{id}` - Update campaign configuration
- `POST /api/v1/campaigns/{id}/start` - Start/resume a campaign
- `POST /api/v1/campaigns/{id}/pause` - Pause a campaign
- `POST /api/v1/campaigns/{id}/complete` - Mark campaign as completed
- `GET /api/v1/campaigns/{id}/stats` - Get campaign statistics
- `POST /api/v1/campaigns/{id}/targets` - Add targets to a campaign
- `GET /api/v1/campaigns/{id}/calls` - List calls for a campaign

### Calls API
- `POST /api/v1/calls` - Trigger an individual call (campaign-based)
- `GET /api/v1/calls/{id}` - Get call details

## Configuration Defaults & Telephony Integration

### Default Values
- **`max_concurrent_calls`**: 500 (when not specified in campaign creation)
- **`retry_policy.max_attempts`**: 5 (when not specified)
- **`retry_policy.base_delay`**: 2 seconds (when not specified)
- **`retry_policy.max_delay`**: 2 minutes (when not specified)

### Telephony Provider (Mock Implementation)
The platform currently uses a **mock telephony provider** for development and testing. The mock provider simulates realistic call behavior:
- 80% success rate for calls
- Random call duration between 1-5 seconds
- 70% of failures are retryable

To integrate with a real telephony service (Twilio, Nexmo, etc.):

1. **Create a new provider implementation** in `internal/telephony/` following the `Provider` interface:
   ```go
   type Provider interface {
       PlaceCall(ctx context.Context, msg queue.DispatchMessage) (Result, error)
   }
   ```

2. **Update the container** (`internal/app/container.go`) to select providers based on configuration:
   ```go
   // Replace hardcoded mock provider selection
   var telephonyProvider telephony.Provider
   switch c.Config.CallBridge.ProviderName {
   case "twilio":
       telephonyProvider = twilio.NewProvider(c.Config.CallBridge)
   case "mock":
       telephonyProvider = mock.NewProvider(c.Config.CallBridge)
   default:
       return fmt.Errorf("unknown provider: %s", c.Config.CallBridge.ProviderName)
   }
   ```

3. **Update configuration** in `configs/config.yaml`:
   ```yaml
   call_bridge:
     provider_name: twilio  # instead of 'mock'
     # Add provider-specific config like API keys, endpoints, etc.
   ```

All commands assume the default configuration in `configs/config.yaml`. Adjust host/port or payload values as needed for your environment.

## Load Testing

The service includes built-in load testing capabilities for multiple campaigns with multiple calls. The load testing script creates realistic campaign scenarios with business hours, retry policies, and concurrent call limits.

### Quick Load Test

Create 3 campaigns with 50 calls each:

```bash
./scripts/load-test.sh 3 50 10
```

Parameters:
- `3` = number of campaigns to create
- `50` = calls/targets per campaign
- `10` = concurrent API requests (to avoid overwhelming)
- `debug` = optional 4th parameter to enable debug output

**Debug mode for troubleshooting:**
```bash
./scripts/load-test.sh 1 5 3 debug
```

### Campaign-Based Load Testing
Use the built-in script to create realistic campaign scenarios with registered target lists, business hours, retry policies, and concurrency limits:

```bash
# Create campaigns with complete registered target lists that respect business hours and concurrency limits
./scripts/load-test.sh [campaigns] [calls_per_campaign] [concurrent_requests]

# Examples:
./scripts/load-test.sh 1 10 5     # Small test: 1 campaign with 10 registered targets
./scripts/load-test.sh 5 100 20   # Medium test: 5 campaigns, 100 registered targets each
./scripts/load-test.sh 10 500 50  # Large test: 10 campaigns, 500 registered targets each

# Via Make:
make load-test                           # Default: 3 campaigns, 50 registered targets each
make load-test CAMPAIGNS=5 CALLS=200     # Custom: 5 campaigns, 200 registered targets each
```

This approach tests the complete system including:
- Campaign lifecycle management with strict target validation
- Registered target list enforcement (only approved phone numbers)
- Business hour scheduling
- Concurrency control via Redis
- Retry logic with backoff
- Statistics aggregation


**Note:** All calls must be part of a campaign. The system uses a campaign-centric architecture where calls are created either by the scheduler processing campaign targets or by direct API calls that specify a campaign_id and use registered target phone numbers.

**Unique Campaign Names:** The load test script generates unique campaign names with timestamps and random suffixes, eliminating the need for database clearing between test runs.

### Load Testing Best Practices

1. **Start Small**: Begin with 1 campaign, 10 calls to verify everything works
2. **Gradual Ramp-up**: Increase load incrementally to identify bottlenecks
   - Small: 1 campaign, 10 calls
   - Medium: 5 campaigns, 100 calls each
   - Large: 10+ campaigns, 500+ calls each
3. **Monitor Resources**: Watch CPU, memory, and I/O on all components during testing
4. **Test Business Hours**: Create campaigns with different business hour windows to test scheduler logic
5. **Test Retry Logic**: Monitor failed calls and verify they retry with proper backoff
6. **Database Performance**: Track PostgreSQL and ScyllaDB write performance under load
7. **Concurrency Testing**: Verify Redis-based concurrency limiting works correctly
8. **Worker Scaling**: Test with different worker pool sizes in configuration
9. **Fault Tolerance**: Kill/restart services during load testing to verify recovery
10. **No Cleanup Required**: Load test campaigns use unique names - no database cleanup needed between runs

### Monitoring Load Tests

#### Real-time Statistics
```bash
# Monitor specific campaign (macOS/Linux)
watch -n 2 'curl -s http://localhost:8081/api/v1/campaigns/{campaign-id}/stats | jq'

# Alternative for macOS (if watch not available):
while true; do curl -s http://localhost:8081/api/v1/campaigns/{campaign-id}/stats | jq; sleep 2; done

# Monitor all campaigns
watch -n 5 'curl -s http://localhost:8081/api/v1/campaigns | jq ".campaigns[] | {id, status}"'

# Alternative for macOS:
while true; do curl -s http://localhost:8081/api/v1/campaigns | jq ".campaigns[] | {id, status}"; sleep 5; done
```

#### System Resources
```bash
# Monitor Kafka topics (requires Kafka tools in PATH)
$HOME/kafka/bin/kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic campaign.calls.dispatch --from-beginning --max-messages 10

# Monitor Redis concurrency counters
redis-cli KEYS "outbound:campaign:*"
redis-cli GET "outbound:campaign:{campaign-id}:active"  # Current active calls

# Monitor database connections
psql -h localhost -U campaign -d campaign -c "SELECT count(*) FROM pg_stat_activity WHERE datname = 'campaign';"

# Monitor campaign targets state
psql -h localhost -U campaign -d campaign -c "SELECT state, count(*) FROM campaign_targets GROUP BY state;"

# View service logs
make logs
```

#### Performance Metrics
- **Throughput**: calls/second processed
- **Latency**: API response times, call completion times
- **Error Rate**: failed vs successful calls
- **Resource Usage**: CPU, memory, disk I/O per component

### Scaling Configuration for Load Testing

For high-volume load testing, adjust these configuration values:

```yaml
# configs/config.yaml
scheduler:
  worker_count: 128        # Increase scheduler parallelism
  max_batch_size: 5000     # Larger batches for efficiency

throttle:
  global_concurrency: 50000  # Allow more concurrent calls
  default_per_campaign: 1000 # Higher per-campaign limits

call_bridge:
  request_timeout: 30s     # Longer timeouts for busy periods

# Database connection pools
postgres:
  max_conns: 200          # More DB connections

redis:
  pool_size: 200          # More Redis connections
```

### What Gets Tested

The campaign-based load testing validates the complete system:
- ✅ End-to-end campaign workflow (create → start → schedule → execute → complete)
- ✅ Business hour enforcement by the scheduler
- ✅ Per-campaign concurrency limiting via Redis
- ✅ Retry logic with exponential backoff and jitter
- ✅ Real-time statistics aggregation
- ✅ Worker processing and telephony integration
- ✅ Multi-campaign orchestration
- ✅ Target validation (campaign boundaries enforced)

### Expected Performance

With default configuration and mock provider:
- **API Throughput**: ~500-1000 requests/second
- **Call Processing**: ~100-200 calls/second per worker
- **Database Writes**: ~1000+ operations/second combined
- **Kafka Messages**: ~1000+ messages/second throughput

Performance scales linearly with:
- Number of API instances (horizontal scaling)
- Worker pool sizes (vertical scaling)
- Database cluster size (horizontal scaling)
- Redis cluster size (horizontal scaling)
