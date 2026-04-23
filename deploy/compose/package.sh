#!/bin/bash
# Create an offline deployment archive.
# Usage: ./package.sh [output_name]
# Output: oktopus-deploy.tar.gz (or custom name)
#
# The archive contains everything needed to deploy on a VM without
# git or build tools. On the target VM:
#   tar xzf oktopus-deploy.tar.gz
#   cd oktopus-deploy
#   docker load -i images.tar
#   ./run.sh

set -e
cd "$(dirname "$0")"

OUTPUT_NAME="${1:-oktopus-deploy}"
STAGE_DIR="$(pwd)/${OUTPUT_NAME}"

echo "=== Building all images ==="
./build.sh

echo "=== Preparing staging directory ==="
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"

echo "=== Collecting image list ==="
IMAGES=$(COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry \
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml config --images)

echo "Images to package:"
echo "$IMAGES"
IMAGE_COUNT=$(echo "$IMAGES" | wc -l)
echo "Total: $IMAGE_COUNT images"

echo "=== Saving images to tar ==="
docker save -o "$STAGE_DIR/images.tar" $IMAGES

echo "=== Copying deployment files ==="
# Compose and config
cp docker-compose.yaml "$STAGE_DIR/"
cp nginx.conf "$STAGE_DIR/"
cp generate-secrets.sh "$STAGE_DIR/"
cp stop.sh "$STAGE_DIR/"
cp -r nats_config "$STAGE_DIR/"

# Env templates
cp .env.*.example "$STAGE_DIR/"

# Data directories
mkdir -p "$STAGE_DIR/firmwares"
mkdir -p "$STAGE_DIR/mongo_data"
mkdir -p "$STAGE_DIR/nats_data"
mkdir -p "$STAGE_DIR/portainer_data"

# Container upload service config (referenced by compose)
if [ -d container-upload-service ]; then
  cp -r container-upload-service "$STAGE_DIR/"
fi

# Standalone run.sh (no dev overlay needed — images are pre-loaded)
cat > "$STAGE_DIR/run.sh" << 'RUNEOF'
#!/bin/bash
cd "$(dirname "$0")"

# Generate secrets if not present
./generate-secrets.sh

COMPOSE_PROFILES=nats,controller,cwmp,mqtt,stomp,ws,adapter,frontend,portainer,registry \
  docker compose up -d
RUNEOF
chmod +x "$STAGE_DIR/run.sh"

echo "=== Creating archive ==="
tar czf "${OUTPUT_NAME}.tar.gz" -C "$(pwd)" "${OUTPUT_NAME}"
rm -rf "$STAGE_DIR"

ARCHIVE_SIZE=$(du -h "${OUTPUT_NAME}.tar.gz" | cut -f1)
echo ""
echo "=== Done ==="
echo "Archive: $(pwd)/${OUTPUT_NAME}.tar.gz ($ARCHIVE_SIZE)"
echo ""
echo "To deploy on target VM:"
echo "  tar xzf ${OUTPUT_NAME}.tar.gz"
echo "  cd ${OUTPUT_NAME}"
echo "  docker load -i images.tar"
echo "  ./run.sh"
