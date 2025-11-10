#!/usr/bin/env bash
set -euo pipefail

BROKER_HOST=${1:-"localhost"}
BROKER_PORT=${2:-"9092"}

create_topic() {
  local topic=$1
  local partitions=${2:-12}
  local replication=${3:-1}
  echo "Creating topic ${topic}"
  kafka-topics.sh \
    --bootstrap-server "${BROKER_HOST}:${BROKER_PORT}" \
    --create \
    --if-not-exists \
    --topic "${topic}" \
    --partitions "${partitions}" \
    --replication-factor "${replication}"
}

create_topic "campaign.calls.dispatch" 48
create_topic "campaign.calls.status" 48
create_topic "campaign.calls.retry.1" 48
create_topic "campaign.calls.retry.2" 48
create_topic "campaign.calls.retry.3" 48
create_topic "campaign.calls.retry.4" 48
create_topic "campaign.calls.retry.5" 48
create_topic "campaign.calls.deadletter" 12
