#!/bin/bash
cd "$(dirname "$0")"

# Generate secrets if not present
./generate-secrets.sh

COMPOSE_PROFILES=nats,cwmp,frontend,portainer,registry docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d
COMPOSE_PROFILES=controller,adapter,ws,mqtt,stomp docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d --build
