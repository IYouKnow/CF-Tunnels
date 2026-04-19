FROM golang:1.25-alpine AS builder

RUN apk add --no-cache nodejs npm

WORKDIR /build

# Single Go module lives in backend/ (root go.mod was a stale duplicate)
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /build/backend
RUN go mod download

WORKDIR /build
COPY backend/ ./backend/
COPY frontend/ ./frontend/

WORKDIR /build/frontend
RUN npm install && npm run build

# Server only — checkdb/cleardb/resetdb are separate mains in the same dir
WORKDIR /build/backend
RUN go build -o /app/backend main.go

FROM alpine:3.21

# cloudflared is not packaged in Alpine apk; use official release (matches common local installs).
ARG TARGETARCH
ARG CLOUDFLARED_VERSION=2026.3.0
RUN apk add --no-cache ca-certificates curl \
	&& curl -fsSL \
		"https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-${TARGETARCH}" \
		-o /usr/local/bin/cloudflared \
	&& chmod +x /usr/local/bin/cloudflared \
	&& cloudflared --version

WORKDIR /app

COPY --from=builder /app/backend /app/
COPY --from=builder /build/frontend/dist /app/frontend/dist

RUN mkdir -p /app/data /app/config

ENV LISTEN_PORT=3000

EXPOSE 3000 8080

CMD ["/app/backend"]
