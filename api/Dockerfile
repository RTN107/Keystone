# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w" -o /out/keystone ./cmd/keystone

# The final image needs no package manager: the binary is static, embeds the time
# zone database, and takes its CA bundle from the build stage for outbound TLS.
FROM alpine:3.22
RUN adduser -D -u 10001 keystone
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/keystone /usr/local/bin/keystone
USER keystone
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=5 \
  CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/api/health" >/dev/null || exit 1
ENTRYPOINT ["keystone"]
