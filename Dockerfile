# syntax=docker/dockerfile:1

# --- build stage: compile a static lookout-server binary ---------------------
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache module downloads first.
COPY go.mod go.sum ./
RUN go mod download

# Build the control plane as a static, dependency-free binary.
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/lookout-server ./cmd/lookout-server

# --- runtime stage: minimal image with just the binary ----------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 lookout
WORKDIR /data
COPY --from=build /out/lookout-server /usr/local/bin/lookout-server

# Persist the JSON data files (lookout-*.json) written to the working dir.
VOLUME ["/data"]
USER lookout
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/login >/dev/null 2>&1 || exit 1

ENTRYPOINT ["lookout-server"]
