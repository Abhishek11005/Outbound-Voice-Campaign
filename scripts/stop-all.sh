#!/bin/bash

set -euo pipefail

PROCFILE=${PROCFILE:-Procfile}

stop_goreman() {
    if ! pgrep -f "goreman" >/dev/null 2>&1; then
        echo "goreman is not running"
        return
    fi

    echo "Stopping goreman managed processes..."
    pkill -INT -f "goreman"

    for _ in {1..10}; do
        if ! pgrep -f "goreman" >/dev/null 2>&1; then
            echo "goreman stopped"
            return
        fi
        sleep 1
    done

    echo "goreman still running; forcing termination"
    pkill -TERM -f "goreman" || true
}

cleanup_orphans() {
    patterns=("kafka-server-start" "zookeeper-server-start" "otelcol-contrib" "jaeger-all-in-one")
    for pattern in "${patterns[@]}"; do
        pkill -TERM -f "$pattern" >/dev/null 2>&1 || true
    done
}

echo "========================================"
echo "Stopping Outbound Voice Campaign stack"
echo "========================================"

stop_goreman
cleanup_orphans

echo "All processes stopped (PostgreSQL/Scylla/Redis may continue if managed externally)."
