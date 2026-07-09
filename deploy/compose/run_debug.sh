#!/bin/bash
cd "$(dirname "$0")"

# Pin project name so the network is oktopus_usp_network, not compose_usp_network
# (Compose defaults to the directory name "compose", which collides on 172.16.235.0/24).
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-oktopus}"

# Generate secrets if not present
./generate-secrets.sh

# firmware-upload runs as node (uid 1000); bind mount must be writable by that user
mkdir -p firmwares
chown 1000:1000 firmwares 2>/dev/null || chmod 1777 firmwares

COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml up -d
