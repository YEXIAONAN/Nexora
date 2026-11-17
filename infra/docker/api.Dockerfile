# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS build

WORKDIR /src
# Override when a different module proxy is required. Checksums in go.sum keep
# module contents verifiable regardless of the proxy used.
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api/ ./
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w \
        -X github.com/YEXIAONAN/Nexora/apps/api/internal/version.Version=${VERSION} \
        -X github.com/YEXIAONAN/Nexora/apps/api/internal/version.Commit=${COMMIT} \
        -X github.com/YEXIAONAN/Nexora/apps/api/internal/version.BuildTime=${BUILD_TIME}" \
      -o /out/nexora-api ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
      -o /out/nexora-migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
      -o /out/nexora-bootstrap ./cmd/bootstrap

FROM alpine:3.22

RUN addgroup -S nexora && adduser -S -G nexora -h /app nexora \
 && apk add --no-cache ca-certificates wget

COPY --from=build /out/nexora-api /out/nexora-migrate /out/nexora-bootstrap /usr/local/bin/

USER nexora
WORKDIR /app
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["nexora-api"]
