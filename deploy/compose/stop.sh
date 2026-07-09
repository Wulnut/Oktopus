#!/bin/bash
cd "$(dirname "$0")"

# Match run.sh / CI: always use project "oktopus" (not directory-default "compose").
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-oktopus}"

COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml down
