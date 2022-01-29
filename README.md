# xpeer-server

The xpeer-server is a pseudo p2p relay server which implements the xpeer-spec (see spec.md) written in golang.

> **XPeer is currently in beta and under heavy development.** Make sure, that your server version is compatible with your client version.

[Read the spec](https://github.com/fabiankachlock/xpeer-server/blob/main/spec.md)

## How to run

1. Install go 1.17 (or higher) [here](https://go.dev/dl/)
2. Run: `go mod download` to install all dependencies
3. Run `go run pkg/xpeer/main.go` to start the server
4. Optional: Run `go build -o xpeer-server pkg/xpeer/main.go` to create an executable which can be run via `./xpeer-server`

(You can use the shell scripts in the `./scripts` folder alternatively)

## Clients

Currently only a client implementation for the web in Javascript is available. You can find it [here](https://github.com/fabiankachlock/xpeer-client/).