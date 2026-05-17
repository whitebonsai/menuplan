# ── Stage 1: Build fetch_menu binary ─────────────────────────────────────────
FROM golang:1.24-alpine AS go-builder
WORKDIR /build
COPY fetch_menu.go .
RUN go build -ldflags="-s -w" -o fetch_menu fetch_menu.go

# ── Stage 2: Download Hugo extended ──────────────────────────────────────────
# Run on build machine but download binary for target arch
FROM --platform=$BUILDPLATFORM alpine:3.21 AS hugo-downloader
ARG HUGO_VERSION=0.159.1
ARG TARGETARCH=amd64
RUN apk add --no-cache curl tar && \
    curl -fsSL "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${TARGETARCH}.tar.gz" \
      | tar -xz hugo

# ── Stage 3: Final image ──────────────────────────────────────────────────────
FROM nginx:1.27-alpine

# Binaries
COPY --from=go-builder  /build/fetch_menu   /usr/local/bin/fetch_menu
COPY --from=hugo-downloader /hugo           /usr/local/bin/hugo

# Site source
COPY . /site
WORKDIR /site

# Nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Cron job: rebuild every Monday at 06:00
RUN echo "0 6 * * 1 cd /site && fetch_menu -cache-dir /cache && hugo --minify -d /usr/share/nginx/html 2>&1 | logger -t menuplan" \
      > /etc/crontabs/root

# Entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
