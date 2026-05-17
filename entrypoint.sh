#!/bin/sh
set -e

CACHE_DIR=/cache
OUT_DIR=/usr/share/nginx/html

echo "[menuplan] Fetching menus (cache: $CACHE_DIR)..."
fetch_menu -cache-dir "$CACHE_DIR"

echo "[menuplan] Building Hugo site..."
hugo --minify -d "$OUT_DIR"

echo "[menuplan] Starting cron..."
service cron start

echo "[menuplan] Starting nginx..."
exec nginx -g 'daemon off;'
