#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "========================================"
echo "Initializing Databases and Topics"
echo "========================================"

# Utility helpers
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

if ! command_exists nc; then
    echo "error: netcat (nc) is required" >&2
    exit 1
fi

# Configurable connection settings
POSTGRES_HOST=${POSTGRES_HOST:-localhost}
POSTGRES_PORT=${POSTGRES_PORT:-5432}
# On macOS with Homebrew, PostgreSQL runs as current user
if [[ "$(uname -s)" == "Darwin" ]]; then
    POSTGRES_SUPERUSER=${POSTGRES_SUPERUSER:-$(whoami)}
else
    POSTGRES_SUPERUSER=${POSTGRES_SUPERUSER:-postgres}
fi
POSTGRES_SUPERUSER_PASSWORD=${POSTGRES_SUPERUSER_PASSWORD:-}
POSTGRES_APP_USER=${POSTGRES_APP_USER:-campaign}
POSTGRES_APP_PASSWORD=${POSTGRES_APP_PASSWORD:-campaign}
POSTGRES_APP_DB=${POSTGRES_APP_DB:-campaign}

SCYLLA_HOST=${SCYLLA_HOST:-localhost}
SCYLLA_PORT=${SCYLLA_PORT:-9042}
SCYLLA_KEYSPACE=${SCYLLA_KEYSPACE:-campaign}

KAFKA_HOST=${KAFKA_HOST:-localhost}
KAFKA_PORT=${KAFKA_PORT:-9092}

REDIS_HOST=${REDIS_HOST:-localhost}
REDIS_PORT=${REDIS_PORT:-6379}

# Wait for services to be ready
wait_for_service() {
    local service=$1
    local host=$2
    local port=$3
    local max_attempts=30
    local attempt=0

    echo -n "Waiting for $service at $host:$port..."
    while ! nc -z "$host" "$port" 2>/dev/null; do
        attempt=$((attempt + 1))
        if [[ $attempt -eq $max_attempts ]]; then
            echo " FAILED"
            echo "ERROR: $service is not responding at $host:$port"
            exit 1
        fi
        echo -n "."
        sleep 2
    done
    echo " OK"
}

# Initialize PostgreSQL
echo ""
echo "1. Initializing PostgreSQL..."

# On macOS, ensure PostgreSQL service is running
if [[ "$(uname -s)" == "Darwin" ]]; then
    if ! brew services list | grep postgresql | grep started >/dev/null 2>&1; then
        echo "Starting PostgreSQL service..."
        brew services start postgresql@14
        sleep 5
    fi
fi

wait_for_service "PostgreSQL" "$POSTGRES_HOST" "$POSTGRES_PORT"

if ! command_exists psql; then
    echo "error: psql command not found" >&2
    exit 1
fi

run_psql_superuser() {
    local sql="$1"
    local opts=(-h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -d postgres -v ON_ERROR_STOP=1)

    # On macOS with Homebrew, PostgreSQL runs as current user
    if [[ "$(uname -s)" == "Darwin" ]]; then
        if [[ -n "$POSTGRES_SUPERUSER_PASSWORD" ]]; then
            PGPASSWORD="$POSTGRES_SUPERUSER_PASSWORD" psql "${opts[@]}" -U "$POSTGRES_SUPERUSER" -c "$sql"
        else
            # Try connecting as current user (Homebrew default)
            psql "${opts[@]}" -c "$sql" 2>/dev/null
        fi
        return $?
    fi

    # On Linux, try various connection methods
    if [[ -n "$POSTGRES_SUPERUSER_PASSWORD" ]]; then
        PGPASSWORD="$POSTGRES_SUPERUSER_PASSWORD" psql "${opts[@]}" -U "$POSTGRES_SUPERUSER" -c "$sql"
        return $?
    fi

    if psql "${opts[@]}" -U "$POSTGRES_SUPERUSER" -c "$sql" 2>/dev/null; then
        return 0
    fi

    if command_exists sudo && [[ "$POSTGRES_SUPERUSER" == "postgres" ]]; then
        sudo -u postgres psql "${opts[@]}" -c "$sql" 2>/dev/null
        return $?
    fi

    echo "warning: failed to connect as $POSTGRES_SUPERUSER; set POSTGRES_SUPERUSER/POSTGRES_SUPERUSER_PASSWORD" >&2
    return 1
}

echo "Creating role and database..."
run_psql_superuser "CREATE ROLE \"$POSTGRES_APP_USER\" WITH LOGIN PASSWORD '$POSTGRES_APP_PASSWORD';" || true
run_psql_superuser "ALTER ROLE \"$POSTGRES_APP_USER\" WITH LOGIN PASSWORD '$POSTGRES_APP_PASSWORD';"
run_psql_superuser "CREATE DATABASE \"$POSTGRES_APP_DB\" OWNER \"$POSTGRES_APP_USER\";" || true
run_psql_superuser "GRANT ALL PRIVILEGES ON DATABASE \"$POSTGRES_APP_DB\" TO \"$POSTGRES_APP_USER\";"
run_psql_superuser "ALTER DATABASE \"$POSTGRES_APP_DB\" OWNER TO \"$POSTGRES_APP_USER\";"
run_psql_superuser "CREATE EXTENSION IF NOT EXISTS citus;" 2>/dev/null || echo "Note: Citus extension not available (this is normal on macOS local setup)"

echo "Running PostgreSQL migrations..."
PGPASSWORD="$POSTGRES_APP_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_APP_USER" -d "$POSTGRES_APP_DB" \
    -f "$PROJECT_ROOT/db/migrations/postgres/0001_init.sql" -v ON_ERROR_STOP=1

# Initialize ScyllaDB/Cassandra
echo ""
echo "2. Initializing ScyllaDB/Cassandra..."

# On macOS, ensure Cassandra service is running
if [[ "$(uname -s)" == "Darwin" ]]; then
    if ! brew services list | grep cassandra | grep started >/dev/null 2>&1; then
        echo "Starting Cassandra service..."
        brew services start cassandra
        sleep 10  # Cassandra takes longer to start
    fi
fi

wait_for_service "ScyllaDB/Cassandra" "$SCYLLA_HOST" "$SCYLLA_PORT"

# Wait additional time for Cassandra to fully initialize
sleep 5

# Create keyspace and run migrations
echo "Creating keyspace and tables..."
if ! command_exists cqlsh; then
    echo "error: cqlsh command not found (install ScyllaDB or Cassandra)" >&2
    exit 1
fi

# Create keyspace
echo "Creating keyspace..."
cqlsh "$SCYLLA_HOST" "$SCYLLA_PORT" -e "CREATE KEYSPACE IF NOT EXISTS $SCYLLA_KEYSPACE WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1} AND durable_writes = true;"

# Run ScyllaDB migrations
echo "Running ScyllaDB migrations..."
cqlsh "$SCYLLA_HOST" "$SCYLLA_PORT" -f "$PROJECT_ROOT/db/migrations/scylla/0001_init.cql"

# Initialize Kafka topics
echo ""
echo "3. Creating Kafka topics..."

# Ensure Kafka and Zookeeper are running
CONFLUENT_HOME="$HOME/confluent"
if [[ "$(uname -s)" == "Darwin" ]]; then
    # Check if Confluent Kafka binaries exist
    if [[ ! -x "$CONFLUENT_HOME/bin/kafka-server-start" ]]; then
        echo "error: Confluent Kafka binaries not found at $CONFLUENT_HOME. Run scripts/install-all.sh first." >&2
        exit 1
    fi

    # Check if Zookeeper is running (on port 2181)
    if ! nc -z localhost 2181 2>/dev/null; then
        echo "Starting Zookeeper..."
        "$CONFLUENT_HOME/bin/zookeeper-server-start" "$PROJECT_ROOT/config/zookeeper.properties" >/dev/null 2>&1 &
        ZOOKEEPER_PID=$!
        sleep 5

        # Wait for Zookeeper to be ready
        for i in {1..10}; do
            if nc -z localhost 2181 2>/dev/null; then
                break
            fi
            sleep 1
        done
    fi

    # Check if Kafka is running (on port 9092)
    if ! nc -z localhost 9092 2>/dev/null; then
        echo "Starting Kafka..."
        "$CONFLUENT_HOME/bin/kafka-server-start" "$PROJECT_ROOT/config/server.properties" >/dev/null 2>&1 &
        KAFKA_PID=$!
        sleep 10  # Kafka takes longer to start
    fi
fi

wait_for_service "Kafka" "$KAFKA_HOST" "$KAFKA_PORT"

KAFKA_TOPICS="$CONFLUENT_HOME/bin/kafka-topics"

if [[ ! -x "$KAFKA_TOPICS" ]]; then
    echo "error: kafka-topics.sh not found at $KAFKA_TOPICS" >&2
    exit 1
fi

# Function to create topic
create_topic() {
    local topic=$1
    local partitions=${2:-3}
    local replication=${3:-1}
    
    echo "Creating topic: $topic"
    "$KAFKA_TOPICS" --create \
        --bootstrap-server "$KAFKA_HOST:$KAFKA_PORT" \
        --topic "$topic" \
        --partitions $partitions \
        --replication-factor $replication \
        2>/dev/null || echo "Topic $topic already exists"
}

# Create all required topics (high partition counts for throughput)
DISPATCH_PARTITIONS=${DISPATCH_PARTITIONS:-48}
STATUS_PARTITIONS=${STATUS_PARTITIONS:-48}
RETRY_PARTITIONS=${RETRY_PARTITIONS:-48}
DEADLETTER_PARTITIONS=${DEADLETTER_PARTITIONS:-12}

create_topic "campaign.calls.dispatch" "$DISPATCH_PARTITIONS" 1
create_topic "campaign.calls.status" "$STATUS_PARTITIONS" 1
create_topic "campaign.calls.retry.1" "$RETRY_PARTITIONS" 1
create_topic "campaign.calls.retry.2" "$RETRY_PARTITIONS" 1
create_topic "campaign.calls.retry.3" "$RETRY_PARTITIONS" 1
create_topic "campaign.calls.retry.4" "$RETRY_PARTITIONS" 1
create_topic "campaign.calls.retry.5" "$RETRY_PARTITIONS" 1
create_topic "campaign.calls.deadletter" "$DEADLETTER_PARTITIONS" 1

# List created topics
echo ""
echo "Created Kafka topics:"
"$KAFKA_TOPICS" --list --bootstrap-server "$KAFKA_HOST:$KAFKA_PORT"

# Initialize Redis
echo ""
echo "4. Testing Redis connection..."

# On macOS, ensure Redis service is running
if [[ "$(uname -s)" == "Darwin" ]]; then
    if ! brew services list | grep redis | grep started >/dev/null 2>&1; then
        echo "Starting Redis service..."
        brew services start redis
        sleep 3
    fi
fi

wait_for_service "Redis" "$REDIS_HOST" "$REDIS_PORT"
if ! command_exists redis-cli; then
    echo "error: redis-cli not found" >&2
    exit 1
fi
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping

echo ""
echo "========================================"
echo "Database Initialization Complete!"
echo "========================================"
echo ""
echo "Initialized:"
echo "  ✓ PostgreSQL database: campaign"
echo "  ✓ ScyllaDB keyspace: campaign"
echo "  ✓ Kafka topics (8 topics created)"
echo "  ✓ Redis connection verified"
echo ""
echo "Ready to start application!"
