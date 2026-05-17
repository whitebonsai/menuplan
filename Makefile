.PHONY: fetch build dev docker-build docker-up docker-down

fetch:
	./fetch_menu -cache-dir ./cache

build: fetch
	hugo --minify

dev:
	./fetch_menu -cache-dir ./cache && hugo server --bind 0.0.0.0 --port 1313

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down
