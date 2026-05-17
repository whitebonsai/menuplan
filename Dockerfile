# ── Stage 1: Build fetch_menu binary ─────────────────────────────────────────
FROM golang:1.24-alpine AS go-builder
WORKDIR /build
COPY fetch_menu.go .
RUN go build -ldflags="-s -w" -o fetch_menu fetch_menu.go

# ── Stage 3: Final image ──────────────────────────────────────────────────────
FROM nginx:1.27

ARG HUGO_VERSION=0.159.1
ARG TARGETARCH=amd64

# Binaries
COPY --from=go-builder /build/fetch_menu /usr/local/bin/fetch_menu

RUN apt-get update && apt-get install -y --no-install-recommends curl tar && \
    curl -fsSL "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-${TARGETARCH}.tar.gz" \
      -o /tmp/hugo.tar.gz && \
    tar -xzf /tmp/hugo.tar.gz -C /tmp/ && \
    mv /tmp/hugo /usr/local/bin/hugo && \
    rm /tmp/hugo.tar.gz && \
    hugo version && \
    apt-get purge -y curl && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*

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
