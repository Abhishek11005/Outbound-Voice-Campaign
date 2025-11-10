CREATE KEYSPACE IF NOT EXISTS campaign
WITH replication = {
  'class': 'SimpleStrategy',
  'replication_factor': 1
};

USE campaign;

CREATE TABLE IF NOT EXISTS calls_by_campaign (
  campaign_id uuid,
  bucket date,
  call_id uuid,
  phone_number text,
  status text,
  attempt_count int,
  scheduled_at timestamp,
  last_attempt_at timestamp,
  updated_at timestamp,
  created_at timestamp,
  last_error text,
  PRIMARY KEY ((campaign_id), bucket, call_id)
) WITH CLUSTERING ORDER BY (bucket DESC, call_id ASC);

CREATE TABLE IF NOT EXISTS calls_by_status (
  campaign_id uuid,
  status text,
  bucket date,
  call_id uuid,
  phone_number text,
  updated_at timestamp,
  PRIMARY KEY ((campaign_id, status), bucket, call_id)
) WITH CLUSTERING ORDER BY (bucket DESC, call_id ASC);

CREATE TABLE IF NOT EXISTS call_attempts (
  call_id uuid,
  attempt_number int,
  status text,
  error text,
  created_at timestamp,
  duration_ms bigint,
  PRIMARY KEY ((call_id), attempt_number)
) WITH CLUSTERING ORDER BY (attempt_number DESC);

CREATE TABLE IF NOT EXISTS campaign_call_metrics (
  campaign_id uuid,
  bucket date,
  total_calls counter,
  completed_calls counter,
  failed_calls counter,
  retries counter,
  PRIMARY KEY ((campaign_id), bucket)
);
