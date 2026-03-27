FROM node:22-alpine

WORKDIR /app

# Install docker CLI
RUN apk add --no-cache docker-cli

# Copy package files
COPY package.json ./
RUN npm install

# Copy service file
COPY container-upload-service.js ./

# Expose port
EXPOSE 8005

# Note: runs as root because it needs Docker socket access (/var/run/docker.sock)
# The service is internal-only (expose, not ports) and requires Docker API access.

# Run service
CMD ["node", "container-upload-service.js"]

