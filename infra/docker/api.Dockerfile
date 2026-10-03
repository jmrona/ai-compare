# `api` image: builds the frontend and the backend and serves both from a single Go process.
# Build context: the repo root (see infra/docker-compose.yml).

FROM node:22-alpine AS frontend
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY frontend/package.json frontend/
RUN pnpm install --frozen-lockfile --filter frontend...
COPY frontend/ frontend/
# true: the UI runs on in-memory sample data. Set to false once the backend serves the full API.
ARG VITE_USE_MOCKS=true
ENV VITE_USE_MOCKS=${VITE_USE_MOCKS} VITE_API_BASE_URL=/api
RUN pnpm --filter frontend build

FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/spike ./cmd/spike

FROM alpine:3.22
WORKDIR /app
COPY --from=backend /out/server /out/spike /app/
COPY --from=frontend /src/frontend/dist /app/web
# Runs as root on purpose: it needs the mounted Docker socket.
ENTRYPOINT ["/app/server"]
