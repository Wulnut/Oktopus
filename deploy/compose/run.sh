#!/bin/bash
cd "$(dirname "$0")"

# Generate secrets if not present
./generate-secrets.sh

COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry docker compose up -d
