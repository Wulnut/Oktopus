#!/bin/bash
# One-time initialization for a fresh production server (e.g. GCP VM).
#
# Run this once before the first prod-deploy.sh. It:
#   1. Adds the current user to the docker group (avoids needing sudo for docker)
#   2. Creates a swap file (recommended for Docker builds; optional on 8GB+ RAM)
#   3. Creates the deployment directory
#
# Usage:
#   sudo ./prod-init.sh [deploy_dir] [swap_size_gb]
#   sudo ./prod-init.sh ~/oktopus 4

set -euo pipefail

DEPLOY_DIR="${1:-$HOME/oktopus}"
SWAP_GB="${2:-4}"
SWAP_FILE="/swapfile"

log() { printf '==> %s\n' "$*"; }

# ---------------------------------------------------------------------------
# Must run as root (or with sudo)
# ---------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: Run with sudo: sudo $0 [deploy_dir] [swap_size_gb]" >&2
    exit 1
fi

REAL_USER="${SUDO_USER:-$(whoami)}"

# ---------------------------------------------------------------------------
# 1. Add user to docker group
# ---------------------------------------------------------------------------
if ! id -nG "$REAL_USER" | grep -qw docker; then
    log "Adding ${REAL_USER} to docker group"
    usermod -aG docker "$REAL_USER"
    echo "    NOTE: Log out and back in (or run 'newgrp docker') for this to take effect."
else
    log "${REAL_USER} already in docker group"
fi

# ---------------------------------------------------------------------------
# 2. Create swap file
# ---------------------------------------------------------------------------
if swapon --show | grep -q "$SWAP_FILE"; then
    log "Swap already active at ${SWAP_FILE}"
else
    SWAP_BYTES="${SWAP_GB}G"
    log "Creating ${SWAP_BYTES} swap file at ${SWAP_FILE}"

    if [ ! -f "$SWAP_FILE" ]; then
        fallocate -l "$SWAP_BYTES" "$SWAP_FILE"
    fi

    chmod 600 "$SWAP_FILE"
    mkswap "$SWAP_FILE"
    swapon "$SWAP_FILE"

    # Persist across reboots
    if ! grep -q "$SWAP_FILE" /etc/fstab; then
        echo "${SWAP_FILE} none swap sw 0 0" >> /etc/fstab
        log "Added swap to /etc/fstab"
    fi

    log "Swap activated (${SWAP_BYTES})"
fi

echo ""
log "Swap status:"
free -h
echo ""
swapon --show

# ---------------------------------------------------------------------------
# 3. Create deployment directory
# ---------------------------------------------------------------------------
log "Creating deployment directory: ${DEPLOY_DIR}"
mkdir -p "$DEPLOY_DIR"
chown "$REAL_USER":"$REAL_USER" "$DEPLOY_DIR"

echo ""
log "Init complete."
echo "Next steps:"
echo "  1. Log out and back in (for docker group to take effect)"
echo "  2. Run: prod-deploy.sh oktopus-prod.tar.gz"
