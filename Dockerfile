# ── Stage 1: Build React frontend ────────────────────────────────────────────
FROM node:22.23.2-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go backend ─────────────────────────────────────────────────
FROM golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS go-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed frontend dist into the binary via the static file server
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /finarch-server ./cmd/server

# ── Stage 3: Litestream binary ────────────────────────────────────────────────
FROM litestream/litestream:0.3.13@sha256:027eda2a89a86015b9797d2129d4dd447e8953097b4190e1d5a30b73e76d8d58 AS litestream

# ── Stage 4: Runtime image ────────────────────────────────────────────────────
FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S -g 10001 finarch \
  && adduser -S -D -H -u 10001 -G finarch finarch
WORKDIR /app
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY --from=go-builder --chown=finarch:finarch /finarch-server /app/finarch-server
COPY --from=frontend-builder --chown=finarch:finarch /app/frontend/dist /app/frontend/dist
COPY --chown=finarch:finarch litestream.yml /etc/litestream.yml
RUN mkdir -p /data \
  && chown -R finarch:finarch /data /app /etc/litestream.yml
VOLUME ["/data"]
ENV FINARCH_DB=/data/finarch.db
ENV FINARCH_ADDR=0.0.0.0:8080
EXPOSE 8080
USER finarch
ENTRYPOINT ["/app/finarch-server"]
