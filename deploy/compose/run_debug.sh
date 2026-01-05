COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d

