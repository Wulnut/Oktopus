FROM node:22-alpine AS builder

WORKDIR /app

# Optional build-time stamp (CI / compose). prebuild runs write-version.js.
ARG OKTOPUS_VERSION=
ARG OKTOPUS_GIT_COMMIT=
ENV OKTOPUS_VERSION=$OKTOPUS_VERSION
ENV OKTOPUS_GIT_COMMIT=$OKTOPUS_GIT_COMMIT

COPY . ./
RUN rm .env && rm .env.local || true

RUN npm install 

RUN npm run build

FROM node:22-alpine AS runner

WORKDIR /app

COPY --from=builder /app/.next ./.next
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/package-lock.json ./package-lock.json
COPY --from=builder /app/public ./public

RUN npm install --omit=dev

USER node
CMD [ "npm", "run", "start" ]
