# ── Stage 1: Build fetch_menu binary ─────────────────────────────────────────
FROM golang:1.24-alpine AS go-builder
WORKDIR /build
COPY fetch_menu.go .
RUN go build -ldflags="-s -w" -o fetch_menu fetch_menu.go

# ── Stage 2: Download Hugo ────────────────────────────────────────────────────
FROM debian:bookworm-slim AS hugo-downloader
ARG HUGO_VERSION=0.159.1
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && \
    curl -fsSL "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin hugo

# ── Stage 3: Final image ──────────────────────────────────────────────────────
FROM nginx:1.27

COPY --from=go-builder /build/fetch_menu /usr/local/bin/fetch_menu
COPY --from=hugo-downloader /usr/local/bin/hugo /usr/local/bin/hugo

RUN apt-get update && apt-get install -y --no-install-recommends cron && \
    rm -rf /var/lib/apt/lists/*

# Site source
COPY . /site
WORKDIR /site

# Nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Cron job: rebuild every Monday at 06:00
RUN echo "0 6 * * 1 root cd /site && fetch_menu -cache-dir /cache && hugo --minify -d /usr/share/nginx/html 2>&1 | logger -t menuplan" \
      > /etc/cron.d/menuplan && chmod 644 /etc/cron.d/menuplan

# Entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
