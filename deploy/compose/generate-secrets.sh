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
for svc in acs mqtt socketio ws; do
    cat > "$COMPOSE_DIR/.env.${svc}" <<EOF
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
EOF
done

# Services that need NATS_URL + TLS + extra config
cat > "$COMPOSE_DIR/.env.controller" <<EOF
MONGO_URI=mongodb://mongo_usp:27017
NATS_URL=${NATS_URL_ENCODED}
${TLS_BLOCK}
FIRMWARE_UPLOAD_URL=http://firmware-upload:8006
FIRMWARE_BASE_URL=http://localhost/firmwares
SECRET_API_KEY=${JWT_SECRET}
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

cat > "$COMPOSE_DIR/.env.stomp-adapter" <<EOF
STOMP_SERVER=stomp:61613
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
