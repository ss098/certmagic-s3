## CertMagic-S3

CertMagic S3-compatible driver written in Go.

### Guide

Build

    go get -u github.com/caddyserver/xcaddy/cmd/xcaddy

    xcaddy build --output ./caddy --with github.com/ss098/certmagic-s3

Build container

    FROM caddy:builder AS builder
    RUN xcaddy build --with github.com/ss098/certmagic-s3 --with ...

    FROM caddy
    COPY --from=builder /usr/bin/caddy /usr/bin/caddy

Run

    caddy run --config caddy.json

Caddyfile Example

    # Global Config

    {
        storage s3 {
            host "Host"
            bucket "Bucket"
            access_id "Access ID"
            secret_key "Secret Key"
            prefix "ssl"
            insecure false #disables SSL if true
        }
    }

JSON Config Example

    {
      "storage": {
        "module": "s3",
        "host": "Host",
        "bucket": "Bucket",
        "access_id": "Access ID",
        "secret_key": "Secret Key",
        "prefix": "ssl",
        "insecure": false
      }
      "app": {
        ...
      }
    }

From Environment

    S3_HOST
    S3_BUCKET
    S3_ACCESS_ID
    S3_SECRET_KEY
    S3_PREFIX
    S3_INSECURE
