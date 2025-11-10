#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PROCFILE=${PROCFILE:-$PROJECT_ROOT/Procfile}
ENV_FILE=${ENV_FILE:-$PROJECT_ROOT/.env.local}

source_env_file() {
    if [[ -f "$ENV_FILE" ]]; then
        echo "Loading environment from $ENV_FILE"
        # shellcheck disable=SC1090
        source "$ENV_FILE"
    fi
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

start_service_if_needed() {
    local name=$1
    local port=$2
    local mac_cmd=$3
    local linux_cmd=$4

    if nc -z localhost "$port" >/dev/null 2>&1; then
        echo "$name already running on port $port"
        return
    fi

    echo "Attempting to start $name..."

    case "$(uname -s)" in
        Darwin)
            if [[ -n "$mac_cmd" ]]; then
                eval "$mac_cmd"
            fi
            ;;
        Linux)
            if [[ -n "$linux_cmd" ]]; then
                eval "$linux_cmd"
            fi
            ;;
        *)
            echo "Unsupported OS for automatic $name startup" >&2
            ;;
    esac

    sleep 3

    if nc -z localhost "$port" >/dev/null 2>&1; then
        echo "$name started"
    else
        echo "warning: unable to confirm $name startup on port $port" >&2
    fi
}

simple_start() {
    echo "Starting services directly..."

    cd "$PROJECT_ROOT"

    # Create logs directory if it doesn't exist
    mkdir -p logs

    # Start application services in background with log redirection
    echo "Starting API server..."
    CONFIG_FILE=${CONFIG_FILE:-configs/config.yaml} go run ./cmd/api --config "$CONFIG_FILE" >> logs/api.log 2>&1 &
    API_PID=$!

    echo "Starting call worker..."
    CONFIG_FILE=${CONFIG_FILE:-configs/config.yaml} go run ./cmd/callworker --config "$CONFIG_FILE" >> logs/callworker.log 2>&1 &
    CALLWORKER_PID=$!

    echo "Starting status worker..."
    CONFIG_FILE=${CONFIG_FILE:-configs/config.yaml} go run ./cmd/statusworker --config "$CONFIG_FILE" >> logs/statusworker.log 2>&1 &
    STATUSWORKER_PID=$!

    echo "Starting retry worker..."
    CONFIG_FILE=${CONFIG_FILE:-configs/config.yaml} go run ./cmd/retryworker --config "$CONFIG_FILE" >> logs/retryworker.log 2>&1 &
    RETRYWORKER_PID=$!

    echo "Starting scheduler..."
    CONFIG_FILE=${CONFIG_FILE:-configs/config.yaml} go run ./cmd/scheduler --config "$CONFIG_FILE" >> logs/scheduler.log 2>&1 &
    SCHEDULER_PID=$!

    echo ""
    echo "Services started! Press Ctrl+C to stop all services."
    echo "API server running on port (configured in config)"
    echo "Logs are being written to logs/*.log files"
    echo ""

    # Wait for interrupt signal
    trap 'echo "Stopping services..."; kill $API_PID $CALLWORKER_PID $STATUSWORKER_PID $RETRYWORKER_PID $SCHEDULER_PID 2>/dev/null; exit 0' INT TERM

    # Keep running
    wait
}

goreman_start() {
    if command_exists goreman && [[ -f "$PROCFILE" ]]; then
        echo "Starting services with goreman..."
        cd "$PROJECT_ROOT"
        exec goreman -f "$PROCFILE" start
    else
        echo "Goreman not available, using simple starter..."
        simple_start
    fi
}

main() {
    source_env_file

    if ! command_exists nc; then
        echo "error: netcat (nc) is required" >&2
        exit 1
    fi

    echo "========================================"
    echo "Starting Outbound Voice Campaign stack"
    echo "========================================"

    start_service_if_needed "PostgreSQL" 5432 "brew services start postgresql@14" "sudo systemctl start postgresql" || true
    start_service_if_needed "ScyllaDB/Cassandra" 9042 "brew services start cassandra" "sudo systemctl start scylla-server" || true
    start_service_if_needed "Redis" 6379 "brew services start redis" "sudo systemctl start redis-server" || true

    echo "Kafka will be started via Docker when needed (see scripts/init-db.sh)"

    goreman_start
}

main "$@"
