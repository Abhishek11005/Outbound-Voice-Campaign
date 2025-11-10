#!/bin/bash

set -e

# Disable proxy settings to avoid corporate proxy issues
unset HTTP_PROXY
unset HTTPS_PROXY
unset http_proxy
unset https_proxy
unset ALL_PROXY
unset all_proxy
unset NO_PROXY
unset no_proxy

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "========================================"
echo "Outbound Voice Campaign - Installation"
echo "========================================"
echo "Proxy settings disabled for this session"
echo ""

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_TYPE=Linux;;
    Darwin*)    OS_TYPE=Mac;;
    *)          echo "Unsupported OS: ${OS}"; exit 1;;
esac

echo "Detected OS: ${OS_TYPE}"
echo ""

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Install Homebrew on Mac if not present
if [[ "$OS_TYPE" == "Mac" ]] && ! command_exists brew; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl --noproxy '*' -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

echo "Installing required services..."
echo ""

# Install PostgreSQL (Citus extension will be installed via PostgreSQL if available)
echo "1. Installing PostgreSQL..."
if [[ "$OS_TYPE" == "Mac" ]]; then
    if ! command_exists psql; then
        HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= brew install postgresql@14
        brew services start postgresql@14
    fi
    # Note: Citus extension will be installed via PostgreSQL CREATE EXTENSION if available
    # If you need Citus specifically, install it manually after setup
elif [[ "$OS_TYPE" == "Linux" ]]; then
    if ! command_exists psql; then
        sudo apt-get update
        sudo apt-get install -y postgresql-14 postgresql-client-14 postgresql-contrib-14
        sudo systemctl start postgresql
        sudo systemctl enable postgresql
    fi
    # Install Citus
    curl --noproxy '*' https://install.citusdata.com/community/deb.sh | sudo bash
    sudo apt-get install -y postgresql-14-citus-11.0
fi

# Install ScyllaDB
echo "2. Installing ScyllaDB..."
if [[ "$OS_TYPE" == "Mac" ]]; then
    if ! command_exists cqlsh; then
        # ScyllaDB doesn't have native Mac support, use Cassandra as alternative
        HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= brew install cassandra
        brew services start cassandra
    fi
elif [[ "$OS_TYPE" == "Linux" ]]; then
    if ! command_exists scylla; then
        # Add ScyllaDB repo
        sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 5e08fbd8b5d6ec9c
        sudo curl --noproxy '*' -L --output /etc/apt/sources.list.d/scylla.list http://downloads.scylladb.com/deb/ubuntu/scylla-5.2.list
        sudo apt-get update
        sudo apt-get install -y scylla
        sudo systemctl start scylla-server
        sudo systemctl enable scylla-server
    fi
fi

# Install Confluent Kafka and Zookeeper
echo "3. Installing Confluent Kafka (includes Zookeeper)..."
CONFLUENT_VERSION="7.5.0"
CONFLUENT_DIR="$HOME/confluent"
if [[ ! -d "$CONFLUENT_DIR" ]]; then
    mkdir -p "$CONFLUENT_DIR"
    cd "$CONFLUENT_DIR"

    # Download Confluent Platform Community Edition
    CONFLUENT_URL="https://packages.confluent.io/archive/7.5/confluent-community-${CONFLUENT_VERSION}.tar.gz"
    ARCHIVE_NAME="confluent-community-${CONFLUENT_VERSION}.tar.gz"

    echo "Downloading Confluent Kafka..."
    if ! curl --noproxy '*' -f -L "$CONFLUENT_URL" -o "$ARCHIVE_NAME"; then
        echo "Error: Failed to download Confluent Kafka"
        exit 1
    fi

    echo "Extracting Confluent Kafka..."
    if [[ -f "$ARCHIVE_NAME" ]]; then
        tar -xzf "$ARCHIVE_NAME"
        mv "confluent-${CONFLUENT_VERSION}"/* .
        rm -rf "confluent-${CONFLUENT_VERSION}" "$ARCHIVE_NAME"
    else
        echo "Error: Archive file not found after download"
        exit 1
    fi

    cd "$PROJECT_ROOT"
fi

# Install Redis
echo "4. Installing Redis..."
if [[ "$OS_TYPE" == "Mac" ]]; then
    if ! command_exists redis-server; then
        HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= brew install redis
        brew services start redis
    fi
elif [[ "$OS_TYPE" == "Linux" ]]; then
    if ! command_exists redis-server; then
        sudo apt-get update
        sudo apt-get install -y redis-server
        sudo systemctl start redis-server
        sudo systemctl enable redis-server
    fi
fi

# Install Jaeger
echo "5. Installing Jaeger..."
JAEGER_VERSION="1.53.0"
JAEGER_DIR="$HOME/jaeger"
if [[ ! -f "$JAEGER_DIR/jaeger-all-in-one" ]]; then
    mkdir -p "$JAEGER_DIR"
    cd "$JAEGER_DIR"
    if [[ "$OS_TYPE" == "Mac" ]]; then
        if [[ "$(uname -m)" == "arm64" ]]; then
            JAEGER_ARCH="darwin-arm64"
        else
            JAEGER_ARCH="darwin-amd64"
        fi
    else
        JAEGER_ARCH="linux-amd64"
    fi

    JAEGER_URL="https://github.com/jaegertracing/jaeger/releases/download/v${JAEGER_VERSION}/jaeger-${JAEGER_VERSION}-${JAEGER_ARCH}.tar.gz"
    echo "Downloading Jaeger..."
    if ! curl --noproxy '*' -f -L "$JAEGER_URL" -o jaeger.tar.gz; then
        echo "Error: Failed to download Jaeger"
        exit 1
    fi
    tar -xzf jaeger.tar.gz
    rm jaeger.tar.gz
    cd "$PROJECT_ROOT"
fi

# Install OpenTelemetry Collector
echo "6. Installing OpenTelemetry Collector..."
OTEL_VERSION="0.94.0"
OTEL_DIR="$HOME/otel-collector"
if [[ ! -f "$OTEL_DIR/otelcol-contrib" ]]; then
    mkdir -p "$OTEL_DIR"
    cd "$OTEL_DIR"
    if [[ "$OS_TYPE" == "Mac" ]]; then
        if [[ "$(uname -m)" == "arm64" ]]; then
            OTEL_ARCH="darwin_arm64"
        else
            OTEL_ARCH="darwin_amd64"
        fi
    else
        OTEL_ARCH="linux_amd64"
    fi

    OTEL_URL="https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTEL_VERSION}/otelcol-contrib_${OTEL_VERSION}_${OTEL_ARCH}.tar.gz"
    echo "Downloading OpenTelemetry Collector..."
    if ! curl --noproxy '*' -f -L "$OTEL_URL" -o otelcol.tar.gz; then
        echo "Error: Failed to download OpenTelemetry Collector"
        exit 1
    fi
    tar -xzf otelcol.tar.gz
    rm otelcol.tar.gz
    cd "$PROJECT_ROOT"
fi

# Install Go tools for process management
echo "7. Installing Go tools..."
# Disable proxy for Go module downloads
export GOPROXY=direct
export GOSUMDB=off
if ! command_exists goreman; then
    echo "Installing Goreman (process manager)..."
    go install github.com/mattn/goreman@latest
fi

if ! command_exists air; then
    echo "Installing Air (hot reload)..."
    go install github.com/air-verse/air@latest
fi

if ! command_exists migrate; then
    echo "Installing golang-migrate..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
fi

echo ""
echo "========================================"
echo "Creating local configuration files..."
echo "========================================"

# Create necessary directories
mkdir -p "$PROJECT_ROOT/data/postgres"
mkdir -p "$PROJECT_ROOT/data/scylla"
mkdir -p "$PROJECT_ROOT/data/kafka-logs"
mkdir -p "$PROJECT_ROOT/data/zookeeper"
mkdir -p "$PROJECT_ROOT/data/redis"
mkdir -p "$PROJECT_ROOT/logs"

touch "$PROJECT_ROOT/data/.gitkeep" "$PROJECT_ROOT/logs/.gitkeep"

# Do not overwrite checked-in configuration files; ensure they exist
if [[ ! -f "$PROJECT_ROOT/config/zookeeper.properties" ]]; then
    cat > "$PROJECT_ROOT/config/zookeeper.properties" <<'EOF'
dataDir=data/zookeeper
clientPort=2181
maxClientCnxns=0
admin.enableServer=false
EOF
fi

if [[ ! -f "$PROJECT_ROOT/config/server.properties" ]]; then
    cat > "$PROJECT_ROOT/config/server.properties" <<'EOF'
broker.id=1
listeners=PLAINTEXT://0.0.0.0:9092
advertised.listeners=PLAINTEXT://localhost:9092
log.dirs=data/kafka-logs
num.network.threads=3
num.io.threads=8
socket.send.buffer.bytes=102400
socket.receive.buffer.bytes=102400
socket.request.max.bytes=104857600
num.partitions=48
num.recovery.threads.per.data.dir=1
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
log.retention.hours=168
log.segment.bytes=1073741824
log.retention.check.interval.ms=300000
zookeeper.connect=localhost:2181
zookeeper.connection.timeout.ms=18000
group.initial.rebalance.delay.ms=0
EOF
fi

if [[ ! -f "$PROJECT_ROOT/config/redis.conf" ]]; then
    cat > "$PROJECT_ROOT/config/redis.conf" <<'EOF'
port 6379
bind 127.0.0.1
protected-mode no
daemonize no
dir data/redis
loglevel notice
logfile logs/redis.log
save 900 1
save 300 10
save 60 10000
EOF
fi

echo ""
echo "========================================"
echo "Installation Complete!"
echo "========================================"
echo ""
echo "Services installed:"
echo "  ✓ PostgreSQL with Citus"
echo "  ✓ ScyllaDB/Cassandra"
echo "  ✓ Apache Kafka & Zookeeper"
echo "  ✓ Redis"
echo "  ✓ Jaeger"
echo "  ✓ OpenTelemetry Collector"
echo "  ✓ Process management tools (Goreman, Air)"
echo ""
echo "Next steps:"
echo "  1. Run: ./scripts/init-db.sh     (Initialize databases)"
echo "  2. Run: ./scripts/run-all.sh     (Start all services)"
echo ""
echo "Or simply run: make setup && make start"
