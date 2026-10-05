#!/bin/sh
# Cross-compile vswitch server and Android client for arm64.
# Usage: sh scripts/build-android.sh
set -e
cd "$(dirname "$0")/.."

echo "=== Building vswitch server (linux/arm64) ==="
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/vswitch-arm64 ./cmd/vswitch

echo "=== Building Android client (linux/arm64) ==="
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/vswitch-client-arm64 ./cmd/client

echo "=== Building for host ==="
go build -ldflags="-s -w" -o dist/vswitch ./cmd/vswitch
go build -ldflags="-s -w" -o dist/vswitch-client ./cmd/client

echo ""
echo "Artifacts in dist/:"
ls -lh dist/