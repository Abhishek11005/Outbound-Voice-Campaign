#!/bin/bash

# Load Testing Script for Outbound Voice Campaign Service
# Usage: ./load-test.sh [campaigns] [calls_per_campaign] [concurrent_requests] [debug]
#
# This script creates campaigns with complete registered target lists and unique names.
# Only phone numbers from the registered list can be processed by each campaign.
# Campaign names include timestamp and random suffix for uniqueness - no database clearing required.

# set -e  # Removed to allow error handling

CAMPAIGNS=${1:-3}
CALLS_PER_CAMPAIGN=${2:-50}
CONCURRENT_REQUESTS=${3:-10}
DEBUG=${4:-false}

API_BASE="http://localhost:8081/api/v1"
HEALTH_URL="http://localhost:8081/healthz"
TIMESTAMP=$(date +%s)

echo "🚀 Starting Load Test"
echo "Campaigns: $CAMPAIGNS"
echo "Calls per campaign: $CALLS_PER_CAMPAIGN"
echo "Concurrent requests: $CONCURRENT_REQUESTS"
echo "Total calls to create: $((CAMPAIGNS * CALLS_PER_CAMPAIGN))"
[ "$DEBUG" = "true" ] && echo "Debug mode: enabled"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to check if service is healthy
check_health() {
    echo "🏥 Checking service health..."
    local response=$(curl -s "$HEALTH_URL" 2>&1)
    local curl_exit=$?

    if [ $curl_exit -eq 0 ] && echo "$response" | grep -q "ok"; then
        echo -e "${GREEN}✅ Service is healthy${NC}"
        [ "$DEBUG" = "true" ] && echo "Health response: $response"
        return 0
    else
        echo -e "${RED}❌ Service health check failed${NC}"
        echo "Curl exit code: $curl_exit"
        echo "Response: $response"
        echo ""
        echo "💡 Troubleshooting tips:"
        echo "1. Make sure services are running: make start"
        echo "2. Check service logs: make logs"
        echo "3. Verify API is responding: curl $HEALTH_URL"
        echo "4. Test campaign creation manually: curl -X POST $API_BASE/campaigns -H 'Content-Type: application/json' -d '{\"name\":\"test\",\"time_zone\":\"UTC\"}'"
        exit 1
    fi
}

# Function to create a campaign with registered targets
create_campaign() {
    local campaign_num=$1
    # Generate unique campaign name with timestamp and random suffix for uniqueness
    local random_suffix=$(printf "%04d" $((RANDOM % 10000)))
    local campaign_name="load-test-campaign-$TIMESTAMP-$random_suffix-$campaign_num"

    [ "$DEBUG" = "true" ] && echo "📝 Creating campaign $campaign_num: $campaign_name with $CALLS_PER_CAMPAIGN registered targets"

    # Generate the initial target list for this campaign
    local targets_json=""
    for i in $(seq 1 $CALLS_PER_CAMPAIGN); do
        local phone="+1555$(printf "%06d" $((RANDOM % 1000000)))"
        targets_json="$targets_json{\"phone_number\": \"$phone\"},"
    done
    targets_json="${targets_json%,}"

    # Create JSON payload with registered targets
    local json_payload='{
        "name": "'"$campaign_name"'",
        "description": "Load test campaign '"$campaign_num"'",
        "time_zone": "UTC",
        "max_concurrent_calls": 50,
        "retry_policy": {
            "max_attempts": 3,
            "base_delay": "1s",
            "max_delay": "30s",
            "jitter": 0.2
        },
        "business_hours": [
            {"day_of_week": 0, "start": "00:00", "end": "23:59"},
            {"day_of_week": 1, "start": "00:00", "end": "23:59"},
            {"day_of_week": 2, "start": "00:00", "end": "23:59"},
            {"day_of_week": 3, "start": "00:00", "end": "23:59"},
            {"day_of_week": 4, "start": "00:00", "end": "23:59"},
            {"day_of_week": 5, "start": "00:00", "end": "23:59"},
            {"day_of_week": 6, "start": "00:00", "end": "23:59"}
        ],
        "targets": ['"$targets_json"']
    }'

    [ "$DEBUG" = "true" ] && echo "Request payload: $json_payload"

    local response=$(curl -s -w "\nHTTP_STATUS:%{http_code}\n" -X POST "$API_BASE/campaigns" \
        -H 'Content-Type: application/json' \
        -d "$json_payload")

    local curl_exit=$?
    local http_status=$(echo "$response" | grep "HTTP_STATUS:" | cut -d':' -f2)
    local response_body=$(echo "$response" | sed '/HTTP_STATUS:/d')

    if [ "$DEBUG" = "true" ]; then
        echo "Response: $response_body"
        echo "HTTP Status: $http_status"
        echo "Curl exit: $curl_exit"
    fi

    # Extract campaign ID from response
    local campaign_id=$(echo "$response_body" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

    if [ $curl_exit -ne 0 ] || [ "$http_status" != "201" ] || [ -z "$campaign_id" ]; then
        echo -e "${RED}❌ Failed to create campaign $campaign_num${NC}"
        echo "HTTP Status: $http_status"
        echo "Curl exit code: $curl_exit"
        if [ "$DEBUG" = "true" ]; then
            echo "Response: $response_body"
        fi
        return 1
    fi

    [ "$DEBUG" = "true" ] && echo -e "${GREEN}✅ Created campaign $campaign_num: $campaign_id${NC}"
    echo "$campaign_id"
}

# Function to start a campaign
start_campaign() {
    local campaign_id=$1
    local campaign_num=$2

    echo "▶️  Starting campaign $campaign_num: $campaign_id"

    local response=$(curl -s -w "\nHTTP_STATUS:%{http_code}\n" -X POST "$API_BASE/campaigns/$campaign_id/start" \
        -H 'Content-Type: application/json')

    local curl_exit=$?
    local http_status=$(echo "$response" | grep "HTTP_STATUS:" | cut -d':' -f2)
    local response_body=$(echo "$response" | sed '/HTTP_STATUS:/d')

    if [ "$DEBUG" = "true" ]; then
        echo "  Debug - HTTP Status: $http_status"
        echo "  Debug - Response: $response_body"
    fi

    if [ $curl_exit -eq 0 ] && ([ "$http_status" = "200" ] || [ "$http_status" = "204" ] || [ "$http_status" = "202" ]); then
        echo -e "${GREEN}✅ Started campaign $campaign_num${NC}"
        return 0
    else
        echo -e "${RED}❌ Failed to start campaign $campaign_num${NC}"
        echo "  HTTP Status: $http_status"
        if [ -n "$response_body" ]; then
            echo "  Response: $response_body"
        fi
        return 1
    fi
}

# Function to trigger individual calls for a campaign
trigger_individual_calls() {
    local campaign_id=$1
    local campaign_num=$2
    local calls=$3

    echo "📞 Triggering $calls individual calls for campaign $campaign_num"

    # Get the campaign's registered targets to use for validation
    targets_response=$(curl -s "http://localhost:8081/api/v1/campaigns/$campaign_id" 2>/dev/null)
    if [ $? -ne 0 ] || ! echo "$targets_response" | jq . >/dev/null 2>&1; then
        echo -e "${RED}❌ Failed to get campaign details${NC}"
        return 1
    fi

    # For this demo, we'll use one of the registered targets
    # In real usage, you'd specify which target to call
    local phone="+15551234567"  # This should be a registered target

    for i in $(seq 1 $calls); do
        echo "  Triggering call $i for phone $phone"

        local response=$(curl -s -X POST "$API_BASE/calls" \
            -H 'Content-Type: application/json' \
            -d "{\"campaign_id\": \"$campaign_id\", \"phone_number\": \"$phone\"}")

        if [ $? -eq 0 ]; then
            echo -e "${GREEN}    ✅ Call $i triggered${NC}"
        else
            echo -e "${RED}    ❌ Failed to trigger call $i${NC}"
            echo "    Response: $response"
        fi

        # Small delay to avoid overwhelming
        sleep 0.1
    done
}

# Function to monitor campaign statistics
monitor_campaigns() {
    local campaign_ids=("$@")

    echo "📊 Monitoring campaign statistics..."
    echo "Press Ctrl+C to stop monitoring"

    while true; do
        echo "=== Campaign Statistics ==="
        for i in "${!campaign_ids[@]}"; do
            local campaign_id="${campaign_ids[$i]}"
            local stats=$(curl -s "$API_BASE/campaigns/$campaign_id/stats" 2>/dev/null)

            if [ $? -eq 0 ] && echo "$stats" | jq . >/dev/null 2>&1; then
                local total=$(echo "$stats" | jq -r '.total_calls // 0')
                local completed=$(echo "$stats" | jq -r '.completed_calls // 0')
                local failed=$(echo "$stats" | jq -r '.failed_calls // 0')
                local in_progress=$(echo "$stats" | jq -r '.in_progress_calls // 0')
                local pending=$(echo "$stats" | jq -r '.pending_calls // 0')
                local retries=$(echo "$stats" | jq -r '.retries_attempted // 0')

                echo "Campaign $((i+1)): Total=$total, Completed=$completed, Failed=$failed, InProgress=$in_progress, Pending=$pending, Retries=$retries"
            else
                echo "Campaign $((i+1)): Error fetching stats (ID: ${campaign_id:0:8}...)"
                if [ "$DEBUG" = "true" ] && [ -n "$stats" ]; then
                    echo "  Debug response: $stats"
                fi
            fi
        done
        echo
        sleep 5
    done
}

# Main execution
check_health

echo "🎯 Load Testing Strategy:"
echo "1. Create $CAMPAIGNS campaigns with registered target lists ($CALLS_PER_CAMPAIGN targets each)"
echo "2. Start campaigns (calls will be scheduled based on business hours)"
echo "3. Optionally trigger individual calls via direct API (campaign-based validation)"
echo "4. Monitor progress (scheduler processes registered targets)"
echo

# Create campaigns
campaign_ids=()
for i in $(seq 1 $CAMPAIGNS); do
    campaign_id=$(create_campaign $i)
    if [ $? -eq 0 ] && [ -n "$campaign_id" ]; then
        campaign_ids+=("$campaign_id")
    else
        echo -e "${YELLOW}⚠️  Skipping campaign $i due to creation failure${NC}"
    fi
    sleep 0.1  # Small delay to avoid overwhelming
done

echo "📋 Successfully created ${#campaign_ids[@]} out of $CAMPAIGNS campaigns"

if [ ${#campaign_ids[@]} -eq 0 ]; then
    echo -e "${RED}❌ No campaigns were created successfully. Exiting.${NC}"
    exit 1
fi

# Start campaigns (they already have their registered targets)
for i in "${!campaign_ids[@]}"; do
    campaign_id="${campaign_ids[$i]}"
    campaign_num=$((i+1))

    if start_campaign "$campaign_id" $campaign_num; then
        echo -e "${GREEN}✅ Campaign $campaign_num started with $CALLS_PER_CAMPAIGN registered targets${NC}"
    else
        echo -e "${YELLOW}⚠️  Campaign $campaign_num failed to start${NC}"
    fi
    sleep 0.2
done

echo
echo "⏳ Campaigns are running! The scheduler will process registered targets based on business hours."
echo "Each campaign has $CALLS_PER_CAMPAIGN registered phone numbers that will be called."
echo "The scheduler runs every 3 seconds (configured for testing)."
echo
echo "To monitor progress:"
echo "1. Check campaign stats: curl http://localhost:8081/api/v1/campaigns/{campaign-id}/stats"
echo "2. View individual calls: curl http://localhost:8081/api/v1/campaigns/{campaign-id}/calls"
echo "3. Monitor target states: psql -h localhost -U campaign -d campaign -c \"SELECT state, count(*) FROM campaign_targets GROUP BY state;\""
echo "4. Monitor logs: make logs"
echo

# Optional: Run monitoring in background
if [ "${4:-}" = "monitor" ]; then
    monitor_campaigns "${campaign_ids[@]}"
else
    echo "To start monitoring, run: $0 $CAMPAIGNS $CALLS_PER_CAMPAIGN $CONCURRENT_REQUESTS monitor"
fi

echo "🎉 Load test setup complete!"
echo "Created $CAMPAIGNS campaigns with $CALLS_PER_CAMPAIGN registered targets each"
echo "Total registered targets: $((CAMPAIGNS * CALLS_PER_CAMPAIGN))"
echo "Campaign IDs: ${campaign_ids[*]}"
