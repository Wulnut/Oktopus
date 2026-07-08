#!/bin/bash
#
# service.sh - Oktopus service lifecycle helper
#
# Wraps docker compose with the correct profiles / overlays and adds
# status, logs and a "doctor" check for the device-list request path.
#
# Usage:
#   ./service.sh start          # full stack, locally built images
#   ./service.sh dev            # full stack + frontend hot-reload
#   ./service.sh prod           # production overlay (low-memory VMs)
#   ./service.sh stop           # stop and remove all containers
#   ./service.sh restart [svc]  # restart everything or a single service
#   ./service.sh status         # container status + health
#   ./service.sh logs [svc]     # tail logs (all or one service)
#   ./service.sh doctor         # diagnose why /device never returns
#
set -euo pipefail
cd "$(dirname "$0")"

# All profiles defined in docker-compose.yaml.
PROFILES="nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry"
export COMPOSE_PROFILES="$PROFILES"

# Compose file sets. The base file works everywhere (local + remote);
# dev/debug overlays add build contexts and hot-reload (local only).
BASE_FILES="-f docker-compose.yaml"
DEV_FILES="-f docker-compose.yaml -f docker-compose.dev.yaml"
DEBUG_FILES="-f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml"
PROD_FILES="-f docker-compose.yaml -f docker-compose.prod.yaml"

# Use production overlay when present (GCP / low-memory VMs).
if [ -f docker-compose.prod.yaml ]; then
    COMPOSE_FILES="$PROD_FILES"
else
    COMPOSE_FILES="$BASE_FILES"
fi

# Colors for status output (disabled when not a TTY).
if [ -t 1 ]; then
    GREEN='\033[0;32m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
else
    GREEN=''; YELLOW=''; RED=''; CYAN=''; NC=''
fi

prepare() {
    # Generate .env.* secrets on first run.
    if ! ls .env.controller .env.adapter .env.nats >/dev/null 2>&1; then
        echo -e "${CYAN}>> Generating secrets (first run)${NC}"
        ./generate-secrets.sh
    fi
    # firmware-upload container runs as uid 1000; the bind mount must be writable.
    mkdir -p firmwares
    chown 1000:1000 firmwares 2>/dev/null || chmod 1777 firmwares
}

cmd_start() {
    prepare
    echo -e "${CYAN}>> Starting Oktopus${NC}"
    docker compose $COMPOSE_FILES up -d --remove-orphans
    echo -e "${GREEN}>> Done. Web UI: http://localhost${NC}"
}

cmd_dev() {
    prepare
    echo -e "${CYAN}>> Starting Oktopus (frontend hot-reload)${NC}"
    docker compose $DEBUG_FILES up -d --remove-orphans
    echo -e "${GREEN}>> Done. Web UI: http://localhost  (frontend auto-reloads on save)${NC}"
}

cmd_prod() {
    prepare
    echo -e "${CYAN}>> Starting Oktopus (production overlay)${NC}"
    docker compose $PROD_FILES up -d --remove-orphans
    echo -e "${GREEN}>> Done. Web UI: http://localhost${NC}"
}

cmd_stop() {
    echo -e "${CYAN}>> Stopping Oktopus${NC}"
    docker compose $COMPOSE_FILES down
    echo -e "${GREEN}>> Stopped.${NC}"
}

cmd_restart() {
    local svc="${1:-}"
    if [ -z "$svc" ]; then
        echo -e "${CYAN}>> Restarting all services${NC}"
        docker compose $COMPOSE_FILES restart
    else
        echo -e "${CYAN}>> Restarting $svc${NC}"
        docker compose $COMPOSE_FILES restart "$svc"
    fi
}

cmd_status() {
    echo -e "${CYAN}>> Container status${NC}"
    docker compose $COMPOSE_FILES ps --format 'table {{.Name}}\t{{.Service}}\t{{.Status}}\t{{.Health}}' 2>/dev/null \
        || docker compose $COMPOSE_FILES ps
}

cmd_logs() {
    local svc="${1:-}"
    if [ -z "$svc" ]; then
        docker compose $COMPOSE_FILES logs -f --tail=100
    else
        docker compose $COMPOSE_FILES logs -f --tail=200 "$svc"
    fi
}

# Diagnose the "device list stuck loading" symptom by checking every hop in
# the request chain: nginx -> controller -> NATS -> adapter -> mongo.
cmd_doctor() {
    echo -e "${CYAN}>> Running device-list diagnostics${NC}"
    echo

    local all_ok=1
    local container_down=""

    # 1. Check that every core container is running.
    for c in nginx frontend controller adapter nats mongo_usp socketio; do
        local state
        state=$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null || echo "missing")
        if [ "$state" = "running" ]; then
            echo -e "  ${GREEN}[OK]${NC}  $c is running"
        else
            echo -e "  ${RED}[!!]${NC}  $c is '$state' -- this breaks the device-list path"
            all_ok=0
            container_down="$container_down $c"
        fi
    done
    echo

    # 2. If a container is down we already know the cause; show how to fix it.
    if [ $all_ok -eq 0 ]; then
        echo -e "${RED}Root cause: these containers are not running:$container_down${NC}"
        echo
        echo "The frontend calls GET /api/tenants/<slug>/device with a plain fetch()"
        echo "that has no timeout. When the backend never responds, the spinner"
        echo "never stops. A stopped controller or adapter is the usual reason."
        echo
        echo "Fix:"
        echo "  ./service.sh start          # or: ./service.sh restart <service>"
        echo "  ./service.sh logs controller # check for NATS/Mongo connection errors"
        return 1
    fi

    # 3. Controller reachable? (hits MongoDB only, does not exercise NATS)
    echo -ne "  controller health (admin-exists) ... "
    if docker exec controller wget -q --spider http://localhost:8000/api/auth/admin/exists 2>/dev/null; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        echo -e "  ${YELLOW}controller is up but the health endpoint failed.${NC}"
    fi

    # 4. NATS reachable from controller?
    echo -ne "  NATS connectivity from controller ... "
    if docker exec controller sh -c 'echo > /dev/tcp/msg_broker/4222' 2>/dev/null; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${RED}FAIL${NC}  (controller cannot reach NATS on msg_broker:4222)"
    fi

    # 5. Adapter subscribed to the device-list subject?
    #    controller publishes adapter.usp.v1.<slug>.devices.retrieve; adapter
    #    listens on adapter.usp.v1.*.devices.retrieve. No responder => spinner.
    echo -ne "  adapter NATS subscription test  ... "
    local probe
    probe=$(docker exec nats nats req --no-responders --timeout=3s \
        'adapter.usp.v1.__doctor__.devices.retrieve' '{}' 2>&1 || true)
    if echo "$probe" | grep -qi 'no responders'; then
        echo -e "${RED}NO RESPONDERS${NC}"
        echo -e "  ${YELLOW}adapter is running but NOT subscribed to devices.retrieve.${NC}"
        echo -e "  ${YELLOW}Check adapter logs: ./service.sh logs adapter${NC}"
        echo -e "  ${YELLOW}(look for NATS connect errors or a crash on startup)${NC}"
        all_ok=0
    elif echo "$probe" | grep -qi 'NATS-RP-HEADER\|200 OK\|"Code":200\|routed'; then
        echo -e "${GREEN}OK${NC}"
    else
        # Some reply is coming back; assume subscription exists.
        echo -e "${GREEN}OK${NC} (got a reply)"
    fi

    # 6. Mongo reachable from adapter?
    echo -ne "  adapter -> MongoDB ................. "
    if docker exec adapter sh -c 'echo > /dev/tcp/mongo_usp/27017' 2>/dev/null; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${RED}FAIL${NC}  (adapter cannot reach MongoDB)"
        all_ok=0
    fi
    echo

    if [ $all_ok -eq 1 ]; then
        echo -e "${GREEN}>> All checks passed.${NC}"
        echo "If the device list is still loading, open the browser DevTools"
        echo "Network tab and check the actual HTTP status of the /device request:"
        echo "  - 200 with data    => frontend rendering bug"
        echo "  - 500              => backend error (./service.sh logs controller)"
        echo "  - pending forever  => nginx/controller hung (restart controller)"
    else
        echo -e "${RED}>> One or more checks failed.${NC}"
        return 1
    fi
}

usage() {
    cat <<EOF
Oktopus service helper

Usage: ./service.sh <command> [args]

Commands:
  start              Start full stack (locally built images)
  dev                Start full stack + frontend hot-reload
  prod               Start with production overlay (low-memory VMs)
  stop               Stop and remove all containers
  restart [service]  Restart all services, or a single one
  status             Show container status and health
  logs [service]     Tail logs (all, or one service name)
  doctor             Diagnose "device list stuck loading"
EOF
}

main() {
    local cmd="${1:-}"
    shift || true
    case "$cmd" in
        start)   cmd_start "$@" ;;
        dev)     cmd_dev "$@" ;;
        prod)    cmd_prod "$@" ;;
        stop)    cmd_stop "$@" ;;
        restart) cmd_restart "$@" ;;
        status)  cmd_status "$@" ;;
        logs)    cmd_logs "$@" ;;
        doctor)  cmd_doctor "$@" ;;
        *)       usage; exit 1 ;;
    esac
}

main "$@"
