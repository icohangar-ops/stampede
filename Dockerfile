# One Dockerfile, one target per service.
#   docker build --target orchestrator -t stampede-orchestrator .
#   docker build --target loadgen      -t stampede-loadgen .
#   docker build --target report       -t stampede-report .
#   docker build --target web          -t stampede-web .
#   docker build --target target       -t stampede-target .

FROM node:22-bookworm AS webbuild
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-bookworm AS gomod
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM gomod AS build-orchestrator
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/orchestrator
FROM gomod AS build-loadgen
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/loadgen
FROM gomod AS build-report
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/report
FROM gomod AS build-target
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/target
FROM gomod AS build-web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/web

FROM debian:bookworm-slim AS runtime-base
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
USER 65532:65532

FROM runtime-base AS orchestrator
COPY --from=build-orchestrator /out/app /app
ENTRYPOINT ["/app"]

FROM runtime-base AS loadgen
COPY --from=build-loadgen /out/app /app
ENTRYPOINT ["/app"]

FROM runtime-base AS report
COPY --from=build-report /out/app /app
ENTRYPOINT ["/app"]

FROM runtime-base AS target
COPY --from=build-target /out/app /app
ENTRYPOINT ["/app"]

FROM runtime-base AS web
COPY --from=build-web /out/app /app
COPY --from=webbuild /src/web/dist /dist
ENV STATIC_DIR=/dist
ENTRYPOINT ["/app"]
