#! /bin/sh

echo "Building XPeer Server..."

go build -o xpeer-server pkg/xpeer/main.go

echo "Done!"