# Reusing node:18.18.0-alpine instead of alpine:latest to avoid downloading unnecessary packages
FROM node:22-alpine

# Install openssl (bash is already available in node image)
RUN apk add --no-cache openssl

WORKDIR /app

COPY generate_certs.sh /app/generate_certs.sh
RUN chmod +x /app/generate_certs.sh

# Entrypoint script that generates certs and then waits
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]

