#!/bin/bash

set -euo pipefail

LOG_DIR="logs"
KAFKA_LOG="$HOME/kafka/logs/server.log"
SCYLLA_LOG="/var/log/scylla/server.log"

if [[ ! -d "$LOG_DIR" ]]; then
    echo "Log directory $LOG_DIR not found"
    exit 1
fi

files=()
while IFS= read -r -d '' file; do
    files+=("$file")
done < <(find "$LOG_DIR" -type f -name '*.log' -print0)

if [[ -f "$KAFKA_LOG" ]]; then
    files+=("$KAFKA_LOG")
fi

if [[ -f "$SCYLLA_LOG" ]]; then
    files+=("$SCYLLA_LOG")
fi

if [[ ${#files[@]} -eq 0 ]]; then
    echo "No log files found to tail"
    exit 0
fi

echo "Tailing logs:"
for file in "${files[@]}"; do
    echo "  - $file"
done

exec tail -F "${files[@]}"
