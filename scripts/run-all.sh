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

goreman_start() {
    if ! command_exists goreman; then
        echo "error: goreman is not installed. Run scripts/install-all.sh first." >&2
        exit 1
    fi

    if [[ ! -f "$PROCFILE" ]]; then
        echo "error: Procfile not found at $PROCFILE" >&2
        exit 1
    fi

    echo "Starting services with goreman..."
    cd "$PROJECT_ROOT"
    exec goreman -f "$PROCFILE" start
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

    echo "Ensuring Confluent Kafka binaries are available..."
    if [[ ! -x "$HOME/confluent/bin/kafka-server-start" ]]; then
        echo "error: Confluent Kafka binaries not found at $HOME/confluent. Run scripts/install-all.sh first." >&2
        exit 1
    fi

    goreman_start
}

main "$@"
