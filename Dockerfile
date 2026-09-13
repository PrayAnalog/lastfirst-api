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
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

# ---- runtime ----
FROM alpine:3.20
WORKDIR /app
COPY --from=backend /server ./server
COPY --from=frontend /app/dist ./frontend/dist

ENV ADDR=:8080
ENV STATIC_DIR=/app/frontend/dist
EXPOSE 8080

ENTRYPOINT ["./server"]
