FROM golang:1.25-alpine AS builder

RUN apk add --no-cache nodejs npm

WORKDIR /build

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /build/backend
RUN go mod download

WORKDIR /build
COPY backend/ ./backend/
COPY frontend/ ./frontend/

WORKDIR /build/frontend
RUN npm install && npm run build

WORKDIR /build/backend
RUN go build -o /out/cf-tunnels main.go

FROM alpine:3.21

ARG TARGETARCH
ARG CLOUDFLARED_VERSION=2026.3.0
RUN apk add --no-cache ca-certificates curl \
	&& curl -fsSL \
		"https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-${TARGETARCH}" \
		-o /usr/local/bin/cloudflared \
	&& chmod +x /usr/local/bin/cloudflared \
	&& cloudflared --version

# Layout:
#   /app/bin/cf-tunnels     — API + static server
#   /app/share/web/         — built SPA (Vite dist)
#   /app/data/                — SQLite (mount a volume here)
#   /app/config/              — optional host-mounted config
WORKDIR /app

RUN mkdir -p /app/bin /app/share/web /app/data /app/config

COPY --from=builder /out/cf-tunnels /app/bin/cf-tunnels
COPY --from=builder /build/frontend/dist /app/share/web

ENV LISTEN_PORT=3000
ENV DATA_DIR=/app/data
ENV WEB_ROOT=/app/share/web

EXPOSE 3000

CMD ["/app/bin/cf-tunnels"]
