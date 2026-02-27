# ── Stage 1: Build React frontend ────────────────────────────────────────────
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go backend ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed frontend dist into the binary via the static file server
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /finarch-server ./cmd/server

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /finarch-server /app/finarch-server
COPY --from=frontend-builder /app/frontend/dist /app/frontend/dist
RUN mkdir -p /data
VOLUME ["/data"]
ENV FINARCH_DB=/data/finarch.db
ENV FINARCH_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/app/finarch-server"]
