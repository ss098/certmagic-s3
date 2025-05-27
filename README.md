# CertMagic-S3

CertMagic S3-compatible driver written in Go.

## Guide

Build

    go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest

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
            host "s3.example.com"
            bucket "my-cert-bucket"
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
        "host": "s3.example.com",
        "bucket": "my-cert-bucket",
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

AWS IAM Provider Example

Caddyfile Example

    # Global Config

    {
        storage s3 {
            host "s3.example.com"
            bucket "my-cert-bucket"
            use_iam_provider true
            prefix "ssl"
            insecure false #disables SSL if true
        }
    }
