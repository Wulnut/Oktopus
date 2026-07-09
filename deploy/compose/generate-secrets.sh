#!/bin/bash
# Generate .env files from .env.*.example templates.
# Called automatically by run.sh and run_debug.sh.
#
# Secrets generated:
#   NATS_USER, NATS_PW  - NATS broker authentication
#   JWT_SECRET          - JWT signing key (shared between controller and firmware-upload)
#
# Template files (.env.*.example) are checked into git with __PLACEHOLDER__ values.
# Generated .env.* files contain real secrets and are gitignored.

set -e

COMPOSE_DIR="$(cd "$(dirname "$0")" && pwd)"

# Generate a random alphanumeric string
rand_secret() {
    head -c 32 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c "$1"
}

# Check if secrets have already been generated
if [ -f "$COMPOSE_DIR/.env.nats" ] && ! grep -q '__NATS_USER__' "$COMPOSE_DIR/.env.nats" 2>/dev/null; then
    # .env.nats exists and has no placeholders -- secrets already generated
    exit 0
fi

echo "Generating secrets for first-time setup..."

NATS_USER="oktopus_$(rand_secret 8)"
NATS_PW="$(rand_secret 24)"
JWT_SECRET="$(rand_secret 32)"
STOMP_SERVICE_KEY="$(rand_secret 24)"

NATS_URL_ENCODED="nats://${NATS_USER}:${NATS_PW}@msg_broker:4222"

# Common TLS config block
TLS_BLOCK='NATS_ENABLE_TLS="true"
CLIENT_CRT=/tmp/nats/config/cert.pem
CLIENT_KEY=/tmp/nats/config/key.pem
SERVER_CA=/tmp/nats/config/rootCA.pem'

# .env.nats
cat > "$COMPOSE_DIR/.env.nats" <<EOF
NATS_NAME=oktopus
NATS_USER=${NATS_USER}
NATS_PW=${NATS_PW}
EOF

# Services that only need NATS_URL + TLS
for svc in acs socketio ws; do
    cat > "$COMPOSE_DIR/.env.${svc}" <<EOF
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF
done

# MQTT broker needs auth enabled
cat > "$COMPOSE_DIR/.env.mqtt" <<EOF
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
AUTH_ENABLE=true
EOF

# Services that need NATS_URL + TLS + extra config
cat > "$COMPOSE_DIR/.env.controller" <<EOF
MONGO_URI=mongodb://mongo_usp:27017
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
FIRMWARE_UPLOAD_URL=http://firmware-upload:8006
SECRET_API_KEY=${JWT_SECRET}
CAMPAIGN_SCHEDULER_ENABLED=true
CAMPAIGN_SCHEDULER_INTERVAL_SEC=60
LOCK_REDIS_ENABLED=true
LOCK_REDIS_URL=redis://redis:6379/0
LOCK_KAFKA_ENABLED=true
LOCK_KAFKA_BROKERS=kafka:9092
LOCK_KAFKA_AUDIT_TOPIC=ont-lock-audit
LOCK_GREENPLUM_ENABLED=false
LOCK_GREENPLUM_DSN=postgres://postgres:postgres@greenplum:5432/ont_lock?sslmode=disable
LOCK_IP_POLL_ENABLED=true
LOCK_IP_POLL_INTERVAL_SEC=60
LOCK_NOTIFY_ENABLED=true
LOCK_NOTIFY_HEALTH_SEC=120
LOCK_RETRY_SCHEDULER_ENABLED=true
LOCK_RETRY_SCHEDULER_INTERVAL_SEC=30
LOCK_COMMAND_TIMEOUT_SEC=30
LOCK_COMMAND_MAX_ATTEMPTS=10
EOF

cat > "$COMPOSE_DIR/.env.adapter" <<EOF
MONGO_URI=mongodb://mongo_usp:27017
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF

cat > "$COMPOSE_DIR/.env.mqtt-adapter" <<EOF
MQTT_URL=tcp://mqtt:1883
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF

cat > "$COMPOSE_DIR/.env.ws-adapter" <<EOF
WS_ADDR=ws
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF

cat > "$COMPOSE_DIR/.env.stomp" <<EOF
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
STOMP_SERVICE_USER=oktopusAdapter
STOMP_SERVICE_KEY=${STOMP_SERVICE_KEY}
EOF

cat > "$COMPOSE_DIR/.env.stomp-adapter" <<EOF
STOMP_SERVER=stomp:61613
STOMP_USER=oktopusAdapter
STOMP_PASSWD=${STOMP_SERVICE_KEY}
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF

cat > "$COMPOSE_DIR/.env.firmware-upload" <<EOF
SERVER_PORT=8006
FIRMWARE_DIR=/app/firmwares
JWT_SECRET=${JWT_SECRET}
EOF

cat > "$COMPOSE_DIR/.env.file-server" <<EOF
DIRECTORY_PATH="/app"
SERVER_PORT=":8004"
EOF

echo "Secrets generated. NATS_USER=${NATS_USER}"
echo "To regenerate, delete the .env.* files and re-run."
