
.PHONY: build run

build:
	xcaddy build --with github.com/caddy-dns/cloudflare --with github.com/ss098/certmagic-s3=.

run:
	./caddy run --config ./Caddyfile