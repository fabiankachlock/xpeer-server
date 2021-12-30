# xpeer-server

The xpeer-server is a pseudo p2p relay server which implements the xpeer-spec (see spec.md) written in golang.

> **XPeer is currently in beta and under heavy development.** Make sure, that your server version is compatible with your client version.

## How to run

1. Install go 1.17 (or higher) [here](https://go.dev/dl/)
2. Run: `go mod download` to install all dependencies
3. Run `go run main.go` to start the server
4. Optional: Run `go build -o xpeer-server main.go` to create an executable which can be run via `./xpeer-server`.