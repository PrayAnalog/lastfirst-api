# Build context must be the project root (the folder containing both
# backend/ and frontend/), e.g.:
#   docker build -f backend/Dockerfile -t <image>:<tag> .

# ---- frontend build ----
FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- backend build ----
# Cross-compiled to linux/amd64 (DOKS node architecture) regardless of the
# machine this is built on, so plain `docker build` always produces an
# image DOKS can pull -- no --platform flag needed at build time.
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /server ./cmd/server

# ---- runtime (pinned to linux/amd64 to match DOKS nodes) ----
FROM --platform=linux/amd64 alpine:3.24
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=backend --chown=65532:65532 /server ./server
COPY --from=frontend --chown=65532:65532 /app/dist ./frontend/dist

ENV ADDR=:8080
ENV STATIC_DIR=/app/frontend/dist
EXPOSE 8080

USER 65532:65532
STOPSIGNAL SIGTERM
ENTRYPOINT ["./server"]
