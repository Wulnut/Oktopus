#!/bin/sh
# Fix ownership of mounted volume, then drop to node user
chown -R node:node /app/firmwares
exec su-exec node "$@"
