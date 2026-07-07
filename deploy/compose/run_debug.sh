#!/bin/bash
cd "$(dirname "$0")"

# Generate secrets if not present
./generate-secrets.sh

# firmware-upload runs as node (uid 1000); bind mount must be writable by that user
mkdir -p firmwares
chown 1000:1000 firmwares 2>/dev/null || chmod 1777 firmwares

COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml up -d
